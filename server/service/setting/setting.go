// Package setting 是管理端可改的对话规则：危机词、上下文预算、自动学习的节奏。
//
// 每个分组在 pa_setting 里存一行 JSON。读的时候和出厂值合并（库里没写的字段用出厂值），
// 缓存 30 秒 —— 管理端改完很快生效，又不至于每轮对话都查库。
package setting

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	GroupSafety   = "safety"
	GroupBudget   = "budget"
	GroupLearning = "learning"

	cacheTTL = 30 * time.Second
)

// Safety 是危机兜底的规则（设计文档 05 · 2.2、06 · 1）。
type Safety struct {
	// CrisisWords 命中任意一个就算「可能的危机」。
	CrisisWords []string `json:"crisisWords"`
	// ExcludePhrases 先从原话里去掉这些短语再匹配，用来消掉误报，比如「想死你了」。
	ExcludePhrases []string `json:"excludePhrases"`
	// HelpWords 回答里出现任意一个，就算给了求助建议。
	HelpWords []string `json:"helpWords"`
	// Notice 模型漏了求助建议时补的提示卡。
	Notice string `json:"notice"`
}

// Budget 是上下文预算（设计文档 05 · 2.3、2.4）。
type Budget struct {
	TurnContextHits   int      `json:"turnContextHits"`   // 每轮最多检索几条记忆
	TurnContextChars  int      `json:"turnContextChars"`  // 每轮检索结果总字数
	ToolOutputChars   int      `json:"toolOutputChars"`   // 单个工具结果字数
	SegmentTokens     int      `json:"segmentTokens"`     // 上一轮输入超过它就压缩成摘要、开新分段
	SmallTalkWords    []string `json:"smallTalkWords"`    // 只由这些词组成的短句算寒暄，不检索
	SmallTalkMaxRunes int      `json:"smallTalkMaxRunes"` // 超过这么长就不算寒暄
}

// Learning 是「越用越准」的节奏（设计文档 06 · 2）。
type Learning struct {
	ChatExtract          bool `json:"chatExtract"`          // 对话空闲后自动从我说的话里抽取记忆
	ChatIdleMinutes      int  `json:"chatIdleMinutes"`      // 对话空闲多久后抽取
	PersonaSettleMinutes int  `json:"personaSettleMinutes"` // 记忆最后一次变化后等多久再刷新人设卡
	WeeklyInsight        bool `json:"weeklyInsight"`        // 每周日晚生成「这周她在意什么」
	MergeSimilar         bool `json:"mergeSimilar"`         // 每周找出疑似重复的记忆，提给我确认合并
}

func DefaultSafety() Safety {
	return Safety{
		CrisisWords: []string{
			"离婚", "分居", "动手", "打了我", "打了她", "打我", "家暴",
			"不想活", "活着没意思", "活不下去", "自杀", "轻生", "想死", "伤害自己", "自残", "抑郁",
		},
		ExcludePhrases: []string{"想死你了", "想死我了", "笑死", "热死", "累死", "饿死", "困死", "气死我了"},
		HelpWords:      []string{"12356", "热线", "咨询", "专业", "心理", "医生", "医院", "110", "120"},
		Notice: "如果有人身安全风险，请马上打 110 / 120。\n" +
			"情绪很难熬的时候，可以打全国心理援助热线 12356。\n" +
			"夫妻矛盾走到冷战、提离婚这一步，建议找婚姻家庭咨询师一起谈谈（当地妇联、社区或医院心理科都能介绍）。",
	}
}

func DefaultBudget() Budget {
	return Budget{
		TurnContextHits: 12, TurnContextChars: 4000, ToolOutputChars: 6000, SegmentTokens: 60000,
		SmallTalkWords:    []string{"好的", "好滴", "好吧", "好", "嗯嗯", "嗯", "哦", "噢", "谢谢你", "谢谢", "多谢", "谢了", "收到", "知道了", "明白了", "明白", "懂了", "行", "可以", "ok", "okay", "👌", "👍"},
		SmallTalkMaxRunes: 12,
	}
}

func DefaultLearning() Learning {
	return Learning{ChatExtract: true, ChatIdleMinutes: 15, PersonaSettleMinutes: 20, WeeklyInsight: true, MergeSimilar: true}
}

// Defaults 按分组返回出厂值，管理端「恢复默认」用。
func Defaults(group string) (any, bool) {
	switch group {
	case GroupSafety:
		return DefaultSafety(), true
	case GroupBudget:
		return DefaultBudget(), true
	case GroupLearning:
		return DefaultLearning(), true
	}
	return nil, false
}

type Service struct {
	db    *gorm.DB
	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	value json.RawMessage
	at    time.Time
}

func New(database *gorm.DB) *Service { return &Service{db: database, cache: map[string]cached{}} }

func (s *Service) Safety(ctx context.Context) Safety {
	value := DefaultSafety()
	s.load(ctx, GroupSafety, &value)
	return value
}

func (s *Service) Budget(ctx context.Context) Budget {
	value := DefaultBudget()
	s.load(ctx, GroupBudget, &value)
	if value.TurnContextHits <= 0 || value.TurnContextChars <= 0 || value.ToolOutputChars <= 0 || value.SegmentTokens <= 0 {
		return DefaultBudget() // 防止改坏了导致什么都检索不到
	}
	return value
}

func (s *Service) Learning(ctx context.Context) Learning {
	value := DefaultLearning()
	s.load(ctx, GroupLearning, &value)
	if value.ChatIdleMinutes <= 0 {
		value.ChatIdleMinutes = DefaultLearning().ChatIdleMinutes
	}
	if value.PersonaSettleMinutes < 0 {
		value.PersonaSettleMinutes = DefaultLearning().PersonaSettleMinutes
	}
	return value
}

// Get 返回某个分组当前生效的值（已和出厂值合并）。
func (s *Service) Get(ctx context.Context, group string) (any, error) {
	switch group {
	case GroupSafety:
		return s.Safety(ctx), nil
	case GroupBudget:
		return s.Budget(ctx), nil
	case GroupLearning:
		return s.Learning(ctx), nil
	}
	return nil, errors.New("没有这个分组")
}

// Save 校验并保存一个分组。
func (s *Service) Save(ctx context.Context, group string, raw json.RawMessage) error {
	var value any
	switch group {
	case GroupSafety:
		var item Safety
		if err := json.Unmarshal(raw, &item); err != nil {
			return errors.New("格式不对")
		}
		item.CrisisWords, item.ExcludePhrases, item.HelpWords = clean(item.CrisisWords), clean(item.ExcludePhrases), clean(item.HelpWords)
		item.Notice = strings.TrimSpace(item.Notice)
		if len(item.CrisisWords) == 0 || len(item.HelpWords) == 0 || item.Notice == "" {
			return errors.New("危机词、求助词、提示卡都不能为空")
		}
		value = item
	case GroupBudget:
		var item Budget
		if err := json.Unmarshal(raw, &item); err != nil {
			return errors.New("格式不对")
		}
		if item.TurnContextHits < 1 || item.TurnContextHits > 50 || item.TurnContextChars < 500 || item.ToolOutputChars < 1000 || item.SegmentTokens < 10000 {
			return errors.New("预算太小了：检索 1-50 条、检索字数至少 500、工具结果至少 1000 字、分段阈值至少 10000 token")
		}
		item.SmallTalkWords = clean(item.SmallTalkWords)
		value = item
	case GroupLearning:
		var item Learning
		if err := json.Unmarshal(raw, &item); err != nil {
			return errors.New("格式不对")
		}
		if item.ChatIdleMinutes < 1 || item.PersonaSettleMinutes < 0 {
			return errors.New("空闲时间至少 1 分钟，等待时间不能为负")
		}
		value = item
	default:
		return errors.New("没有这个分组")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := settingRow{SettingKey: group, Value: string(data), UpdatedAt: time.Now()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, group)
	s.mu.Unlock()
	return nil
}

// Reset 删掉库里的值，回到出厂值。
func (s *Service) Reset(ctx context.Context, group string) error {
	if _, ok := Defaults(group); !ok {
		return errors.New("没有这个分组")
	}
	if err := s.db.WithContext(ctx).Where("setting_key = ?", group).Delete(&settingRow{}).Error; err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.cache, group)
	s.mu.Unlock()
	return nil
}

// load 把库里的值叠加到 target（出厂值）上。读不到就保持出厂值。
func (s *Service) load(ctx context.Context, group string, target any) {
	s.mu.Lock()
	entry, ok := s.cache[group]
	s.mu.Unlock()
	if !ok || time.Since(entry.at) > cacheTTL {
		var row settingRow
		entry = cached{at: time.Now()}
		if err := s.db.WithContext(ctx).Where("setting_key = ?", group).First(&row).Error; err == nil {
			entry.value = json.RawMessage(row.Value)
		}
		s.mu.Lock()
		s.cache[group] = entry
		s.mu.Unlock()
	}
	if len(entry.value) > 0 {
		_ = json.Unmarshal(entry.value, target)
	}
}

func clean(list []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

type settingRow struct {
	SettingKey string `gorm:"primaryKey"`
	Value      string
	UpdatedAt  time.Time
}

func (settingRow) TableName() string { return "pa_setting" }
