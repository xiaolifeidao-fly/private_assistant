// Package insight 是每周的整理（设计文档 01 · 3.6、06 · M5）：
//
//   - 周洞察：每周日晚把这一周的新记忆和没办完的事交给模型，总结「她们这周在意什么」，
//     存成 insight 类记忆（之后检索、人设卡都能用到），并推送到手机；
//   - 找重复：按向量相似度找出疑似重复的记忆，提给我决定合并（保留一条、另一条归档）还是忽略。
//
// 每个任务每周只跑一次：pa_job_run 以（任务, 周, 家庭）为主键，插入成功的进程才执行。
package insight

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"common/keys"
	"service/family"
	"service/llm"
	"service/memory"
	"service/reminder"
	"service/setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	JobWeekly = "weekly_insight"
	JobMerge  = "merge_similar"

	taskWeekly      = "insight.weekly"
	mergeThreshold  = 0.92
	mergeLimit      = 50
	schedulerTick   = 15 * time.Minute
	weeklyHour      = 20 // 周日晚上 8 点以后
	weekMemoryLimit = 200
)

type MemoryPort interface {
	Since(ctx context.Context, familyKey string, since time.Time, limit int) ([]memory.Memory, error)
	SimilarPairs(ctx context.Context, familyKey string, threshold float64, limit int) ([]memory.SimilarPair, error)
	Get(ctx context.Context, familyKey, memoryKey string) (memory.Memory, error)
	SetStatus(ctx context.Context, familyKey, memoryKey, status string) error
	Create(ctx context.Context, familyKey string, input memory.Input) (memory.Memory, error)
	List(ctx context.Context, familyKey string, filter memory.ListFilter) ([]memory.Memory, int64, error)
}

type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
}

type LLMPort interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

type ReminderPort interface {
	CreateManual(ctx context.Context, familyKey string, input reminder.ManualInput) (reminder.Reminder, error)
}

type SettingPort interface {
	Learning(ctx context.Context) setting.Learning
}

type Service struct {
	db        *gorm.DB
	memory    MemoryPort
	family    FamilyPort
	llm       LLMPort
	reminders ReminderPort
	settings  SettingPort
}

func New(database *gorm.DB, memoryService MemoryPort, familyService FamilyPort, gateway LLMPort, reminderService ReminderPort, settingService SettingPort) *Service {
	return &Service{db: database, memory: memoryService, family: familyService, llm: gateway, reminders: reminderService, settings: settingService}
}

// RunScheduler 每 15 分钟看一次是否到了每周任务的时间。
func (s *Service) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(schedulerTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		now := time.Now()
		if now.Weekday() != time.Sunday || now.Hour() < weeklyHour {
			continue
		}
		var families []string
		if err := s.db.WithContext(ctx).Table("pa_family").Pluck("family_key", &families).Error; err != nil {
			continue
		}
		learning := s.settings.Learning(ctx)
		for _, familyKey := range families {
			jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			if learning.MergeSimilar {
				if _, err := s.FindDuplicates(jobCtx, familyKey, false); err != nil && !errors.Is(err, errAlreadyRan) {
					log.Printf("找重复记忆: %v", err)
				}
			}
			if learning.WeeklyInsight {
				if _, err := s.WeeklyInsight(jobCtx, familyKey, false); err != nil && !errors.Is(err, errAlreadyRan) {
					log.Printf("周洞察: %v", err)
				}
			}
			cancel()
		}
	}
}

var errAlreadyRan = errors.New("这周已经跑过了")

func weekPeriod(at time.Time) string {
	year, week := at.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

// claim 抢占这一周的执行权。force=true（管理端手动触发）时用带时间的周期，不和定时任务冲突。
func (s *Service) claim(ctx context.Context, job, familyKey string, force bool) (string, error) {
	period := weekPeriod(time.Now())
	if force {
		period += "-manual-" + time.Now().Format("0102-1504")
	}
	result := s.db.WithContext(ctx).Clauses(clause.Insert{Modifier: "IGNORE"}).Create(&jobRunRow{Job: job, Period: period, FamilyKey: familyKey, CreatedAt: time.Now()})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", errAlreadyRan
	}
	return period, nil
}

func (s *Service) finish(ctx context.Context, job, period, familyKey, summary string) {
	s.db.WithContext(ctx).Model(&jobRunRow{}).Where("job = ? AND period = ? AND family_key = ?", job, period, familyKey).Update("result", summary)
}

// Run 是一次每周任务的执行记录。
type Run struct {
	Job       string    `json:"job"`
	Period    string    `json:"period"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Service) Runs(ctx context.Context, familyKey string, limit int) ([]Run, error) {
	var rows []jobRunRow
	if err := s.db.WithContext(ctx).Where("family_key = ?", familyKey).Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Run, 0, len(rows))
	for _, row := range rows {
		run := Run{Job: row.Job, Period: row.Period, CreatedAt: row.CreatedAt}
		if row.Result != nil {
			run.Result = *row.Result
		}
		result = append(result, run)
	}
	return result, nil
}

// ---------------------------------------------------------------- 周洞察

const weeklyPrompt = `你在帮我做家庭关系的每周回顾。下面是这一周新记下的事，以及还没办完的承诺和诉求。

请输出：
- items：每个值得关注的家人一条（没有新信息的人不写），person 填人物 key，title 一句话，content 写她这周最在意的 1-3 件事、情绪和关系上的变化、我接下来最该做的一件事；每个判断句末标出依据的记忆编号（如 [mem_xxx]）。
- summary：给我手机推送用的一段话，80 字以内，点出这周最重要的一两件事。

只根据给出的记忆，不要编；不站队、不评判谁对谁错。`

// WeeklyInsight 生成本周洞察，返回新建的洞察记忆。force=true 时忽略「每周一次」。
func (s *Service) WeeklyInsight(ctx context.Context, familyKey string, force bool) ([]memory.Memory, error) {
	period, err := s.claim(ctx, JobWeekly, familyKey, force)
	if err != nil {
		return nil, err
	}
	created, summary, err := s.weeklyInsight(ctx, familyKey)
	if err != nil {
		s.finish(ctx, JobWeekly, period, familyKey, "失败："+err.Error())
		return nil, err
	}
	s.finish(ctx, JobWeekly, period, familyKey, summary)
	return created, nil
}

func (s *Service) weeklyInsight(ctx context.Context, familyKey string) ([]memory.Memory, string, error) {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return nil, "", err
	}
	names := map[string]string{}
	var personKeys []string
	for _, person := range persons {
		names[person.PersonKey] = person.RoleLabel
		personKeys = append(personKeys, person.PersonKey)
	}
	week, err := s.memory.Since(ctx, familyKey, time.Now().AddDate(0, 0, -7), weekMemoryLimit)
	if err != nil {
		return nil, "", err
	}
	if len(week) == 0 {
		return nil, "这周没有新记忆，跳过", nil
	}
	open, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Types: []string{"commitment", "need"}, Statuses: []string{memory.StatusActive}, Size: 50})
	if err != nil {
		return nil, "", err
	}
	var builder strings.Builder
	builder.WriteString("家人：")
	for _, person := range persons {
		fmt.Fprintf(&builder, "%s=%s；", person.PersonKey, person.RoleLabel)
	}
	builder.WriteString("\n\n<this_week>\n")
	for _, item := range week {
		builder.WriteString(formatMemory(item, names) + "\n")
	}
	builder.WriteString("</this_week>\n\n<open_items>\n")
	for _, item := range open {
		var attrs struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(item.Attrs, &attrs)
		if attrs.Status == "" || attrs.Status == "open" {
			builder.WriteString(formatMemory(item, names) + "\n")
		}
	}
	builder.WriteString("</open_items>")

	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskWeekly,
		System:     []llm.SystemPart{{Text: weeklyPrompt}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: weeklySchema(personKeys),
		RefKey:     "insight",
	}, nil)
	if err != nil {
		return nil, "", err
	}
	text := response.Message.Text()
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	var parsed struct {
		Items []struct {
			Person  string `json:"person"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"items"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, "", fmt.Errorf("解析周洞察失败: %w", err)
	}
	now := time.Now()
	var created []memory.Memory
	for _, item := range parsed.Items {
		if strings.TrimSpace(item.Content) == "" {
			continue
		}
		subject := item.Person
		if _, ok := names[subject]; !ok {
			subject = ""
		}
		saved, err := s.memory.Create(ctx, familyKey, memory.Input{
			Type: "insight", SubjectPerson: subject, Title: item.Title, Content: item.Content, Importance: 3,
			Confidence: 0.8, Status: memory.StatusActive, OccurredAt: &now, CreatedBy: "insight",
			Attrs: json.RawMessage(fmt.Sprintf(`{"period":%q}`, weekPeriod(now))),
		})
		if err != nil {
			log.Printf("保存周洞察失败: %v", err)
			continue
		}
		created = append(created, saved)
	}
	summary := strings.TrimSpace(parsed.Summary)
	if summary != "" {
		if _, err := s.reminders.CreateManual(ctx, familyKey, reminder.ManualInput{
			Title: "本周回顾", Body: summary, RemindAt: now.Add(time.Minute), TargetURL: "/today",
		}); err != nil {
			log.Printf("推送周洞察失败: %v", err)
		}
	}
	return created, summary, nil
}

func weeklySchema(personKeys []string) map[string]any {
	text := map[string]any{"type": "string"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{"type": "array", "items": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"person": map[string]any{"type": "string", "enum": personKeys}, "title": text, "content": text},
				"required":             []string{"person", "title", "content"},
				"additionalProperties": false,
			}},
			"summary": text,
		},
		"required":             []string{"items", "summary"},
		"additionalProperties": false,
	}
}

func formatMemory(item memory.Memory, names map[string]string) string {
	about := "全家"
	if name, ok := names[item.SubjectPerson]; ok {
		about = name
	}
	when := item.CreatedAt
	if item.OccurredAt != nil {
		when = *item.OccurredAt
	}
	line := fmt.Sprintf("[%s | %s | %s | %s] %s", item.MemoryKey, item.TypeLabel, about, when.Format("01-02"), item.Title)
	content := strings.Join(strings.Fields(item.Content), " ")
	if runes := []rune(content); len(runes) > 300 {
		content = string(runes[:300]) + "…"
	}
	if content != "" && content != item.Title {
		line += " —— " + content
	}
	return line
}

// ---------------------------------------------------------------- 找重复

// Merge 是一条重复记忆的合并建议。
type Merge struct {
	MergeKey   string         `json:"mergeKey"`
	A          *memory.Memory `json:"a"`
	B          *memory.Memory `json:"b"`
	Similarity float64        `json:"similarity"`
	Status     string         `json:"status"`
	KeptKey    string         `json:"keptKey"`
	CreatedAt  time.Time      `json:"createdAt"`
}

// FindDuplicates 找出疑似重复的记忆，新的写进待处理列表，返回新增条数。
func (s *Service) FindDuplicates(ctx context.Context, familyKey string, force bool) (int, error) {
	period, err := s.claim(ctx, JobMerge, familyKey, force)
	if err != nil {
		return 0, err
	}
	pairs, err := s.memory.SimilarPairs(ctx, familyKey, mergeThreshold, mergeLimit)
	if err != nil {
		s.finish(ctx, JobMerge, period, familyKey, "失败："+err.Error())
		return 0, err
	}
	added := 0
	for _, pair := range pairs {
		row := mergeRow{MergeKey: keys.New("mrg"), FamilyKey: familyKey, AKey: pair.A, BKey: pair.B, Similarity: pair.Similarity, Status: "pending", CreatedAt: time.Now()}
		result := s.db.WithContext(ctx).Clauses(clause.Insert{Modifier: "IGNORE"}).Create(&row)
		if result.Error == nil && result.RowsAffected > 0 {
			added++
		}
	}
	s.finish(ctx, JobMerge, period, familyKey, fmt.Sprintf("找到 %d 对，新增 %d 对待处理", len(pairs), added))
	return added, nil
}

func (s *Service) ListMerges(ctx context.Context, familyKey, status string) ([]Merge, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var rows []mergeRow
	if err := query.Order("similarity DESC, id DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Merge, 0, len(rows))
	for _, row := range rows {
		item := Merge{MergeKey: row.MergeKey, Similarity: row.Similarity, Status: row.Status, CreatedAt: row.CreatedAt}
		if row.KeptKey != nil {
			item.KeptKey = *row.KeptKey
		}
		if a, err := s.memory.Get(ctx, familyKey, row.AKey); err == nil {
			item.A = &a
		}
		if b, err := s.memory.Get(ctx, familyKey, row.BKey); err == nil {
			item.B = &b
		}
		result = append(result, item)
	}
	return result, nil
}

// DecideMerge 处理一条建议：keep 是要保留的记忆编号（另一条归档）；keep 为空表示不是重复，忽略。
func (s *Service) DecideMerge(ctx context.Context, familyKey, mergeKey, keep string) error {
	var row mergeRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND merge_key = ?", familyKey, mergeKey).First(&row).Error; err != nil {
		return errors.New("建议不存在")
	}
	if row.Status != "pending" {
		return errors.New("这条已经处理过了")
	}
	now := time.Now()
	updates := map[string]any{"status": "dismissed", "decided_at": now}
	if keep != "" {
		if keep != row.AKey && keep != row.BKey {
			return errors.New("只能保留这两条中的一条")
		}
		drop := row.AKey
		if keep == row.AKey {
			drop = row.BKey
		}
		if err := s.memory.SetStatus(ctx, familyKey, drop, memory.StatusArchived); err != nil {
			return err
		}
		updates["status"], updates["kept_key"] = "merged", keep
	}
	return s.db.WithContext(ctx).Model(&mergeRow{}).Where("id = ?", row.ID).Updates(updates).Error
}

type mergeRow struct {
	ID         int64
	MergeKey   string
	FamilyKey  string
	AKey       string
	BKey       string
	Similarity float64
	Status     string
	KeptKey    *string
	CreatedAt  time.Time
	DecidedAt  *time.Time
}

func (mergeRow) TableName() string { return "pa_memory_merge" }

type jobRunRow struct {
	Job       string `gorm:"primaryKey"`
	Period    string `gorm:"primaryKey"`
	FamilyKey string `gorm:"primaryKey"`
	Result    *string
	CreatedAt time.Time
}

func (jobRunRow) TableName() string { return "pa_job_run" }
