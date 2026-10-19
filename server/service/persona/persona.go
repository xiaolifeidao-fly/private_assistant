// Package persona 维护人设卡：根据某人的全部记忆，归纳出「她是什么样的人」。
//
// 人设卡给两个地方用：分身用它来模拟她的视角，助理用它来理解她。
// 卡里只放稳定的特质（性格、说话方式、价值观、雷区、关系格局）；具体的事、最近的状态和情绪
// 留在记忆里，对话时按需检索。所以新记忆进来不会自动重写卡：先让模型判断这批记忆有没有
// 改变她的人设，确实变了才重写。
// 人会变，所以卡按版本保存，每次生成都写明和上一版相比的变化；
// 手工修订和回滚也都是新的版本，历史不丢。
package persona

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

	"gorm.io/gorm"
)

const (
	taskCard = "persona.card"
	taskGate = "persona.gate"

	StatusActive  = "active"
	StatusHistory = "history"

	// 少于这么多条记忆时不生成，免得卡片全是猜的
	minMemories = 3
)

var ErrNotFound = errors.New("人设卡不存在")

type Service struct {
	db       *gorm.DB
	family   FamilyPort
	memory   MemoryPort
	llm      LLMPort
	settings SettingPort
}

func New(database *gorm.DB, familyService FamilyPort, memoryService MemoryPort, gateway LLMPort, settingService SettingPort) *Service {
	return &Service{db: database, family: familyService, memory: memoryService, llm: gateway, settings: settingService}
}

// settleDelay 是某人记忆最近一次变化之后至少等多久才自动重写人设卡（管理端「对话规则 → 自动学习」）。
func (s *Service) settleDelay(ctx context.Context) time.Duration {
	return time.Duration(s.settings.Learning(ctx).PersonaSettleMinutes) * time.Minute
}

type Card struct {
	CardKey      string     `json:"cardKey"`
	PersonKey    string     `json:"personKey"`
	Version      int        `json:"version"`
	Content      string     `json:"content"`
	Changes      string     `json:"changes"`
	MemoryCount  int        `json:"memoryCount"`
	MemoryCutoff *time.Time `json:"memoryCutoff"`
	Status       string     `json:"status"`
	CreatedBy    string     `json:"createdBy"`
	CreatedAt    time.Time  `json:"createdAt"`
}

// Active 返回某人当前生效的人设卡。
func (s *Service) Active(ctx context.Context, familyKey, personKey string) (Card, bool, error) {
	var row cardRow
	err := s.db.WithContext(ctx).Where("family_key = ? AND person_key = ? AND status = ?", familyKey, personKey, StatusActive).
		Order("version DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Card{}, false, nil
	}
	if err != nil {
		return Card{}, false, err
	}
	return row.toDTO(), true, nil
}

func (s *Service) History(ctx context.Context, familyKey, personKey string) ([]Card, error) {
	var rows []cardRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND person_key = ?", familyKey, personKey).
		Order("version DESC").Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Card, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

// SaveManual 手工修订：以给定内容存为新版本并生效。
func (s *Service) SaveManual(ctx context.Context, familyKey, personKey, content, changes string) (Card, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return Card{}, errors.New("人设卡内容不能为空")
	}
	if _, err := s.family.GetPerson(ctx, familyKey, personKey); err != nil {
		return Card{}, err
	}
	var cutoff *time.Time
	var count int
	if current, ok, _ := s.Active(ctx, familyKey, personKey); ok {
		cutoff, count = current.MemoryCutoff, current.MemoryCount
	}
	return s.saveVersion(ctx, familyKey, personKey, content, defaultString(changes, "手工修订"), count, cutoff, "manual")
}

// Activate 把某个历史版本复制为新版本并生效（回滚）。
func (s *Service) Activate(ctx context.Context, familyKey, cardKey string) (Card, error) {
	var row cardRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND card_key = ?", familyKey, cardKey).First(&row).Error; err != nil {
		return Card{}, ErrNotFound
	}
	if row.Status == StatusActive {
		return row.toDTO(), nil
	}
	return s.saveVersion(ctx, familyKey, row.PersonKey, row.Content, fmt.Sprintf("回滚到 v%d", row.Version), row.MemoryCount, row.MemoryCutoff, "rollback")
}

// cardOutput 是生成人设卡时模型的结构化输出。
type cardOutput struct {
	Content     string `json:"content"`
	Changes     string `json:"changes"`
	Significant bool   `json:"significant"`
}

func cardSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"content":     map[string]any{"type": "string", "description": "人设卡正文"},
			"changes":     map[string]any{"type": "string", "description": "和上一版相比的主要变化；第一版写「首次生成」"},
			"significant": map[string]any{"type": "boolean", "description": "她的人设本身有没有变化（性格、说话方式、价值观、雷区、关系格局）；只是多了一件事、措辞调整都不算"},
		},
		"required": []string{"content", "changes", "significant"},
	}
}

// Generate 根据最新记忆生成人设卡。force=false 时，没有新记忆就不生成。
// 返回的 bool 表示是否产生了新版本。
func (s *Service) Generate(ctx context.Context, familyKey, personKey string, force bool) (Card, bool, error) {
	person, err := s.family.GetPerson(ctx, familyKey, personKey)
	if err != nil {
		return Card{}, false, err
	}
	count, latest, err := s.memory.LatestChange(ctx, familyKey, personKey)
	if err != nil {
		return Card{}, false, err
	}
	current, hasCurrent, err := s.Active(ctx, familyKey, personKey)
	if err != nil {
		return Card{}, false, err
	}
	if !force {
		if count < minMemories {
			return current, false, nil
		}
		if hasCurrent && latest != nil && current.MemoryCutoff != nil && !latest.After(*current.MemoryCutoff) {
			return current, false, nil
		}
		// 刚录完一批记忆时先等等，免得一边录一边反复生成
		if latest != nil && time.Since(*latest) < s.settleDelay(ctx) {
			return current, false, nil
		}
	}
	if count == 0 {
		return current, false, errors.New("还没有关于这个人的记忆，先记几条再生成")
	}
	// 已经有卡时，先只拿这批新记忆问一句「人设变了没有」；大多数新记忆只是具体的事，留在记忆里就够了
	if hasCurrent && !force && current.MemoryCutoff != nil {
		changed, err := s.personaChanged(ctx, familyKey, person, current)
		if err != nil {
			return Card{}, false, err
		}
		if !changed {
			s.moveCutoff(ctx, current.CardKey, latest, count)
			return current, false, nil
		}
	}

	active, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Person: personKey, Statuses: []string{memory.StatusActive}, Size: 200})
	if err != nil {
		return Card{}, false, err
	}
	old, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{Person: personKey, Statuses: []string{memory.StatusSuperseded}, Size: 40})
	if err != nil {
		return Card{}, false, err
	}
	var input strings.Builder
	input.WriteString(s.background(ctx, familyKey, person, active, old))
	if hasCurrent {
		fmt.Fprintf(&input, "\n## 上一版人设卡（v%d，%s）\n%s\n", current.Version, current.CreatedAt.Format("2006-01-02"), current.Content)
	}

	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskCard,
		System:     []llm.SystemPart{{Text: systemPrompt(person), Cache: true}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: input.String()}}}},
		JSONSchema: cardSchema(),
		RefKey:     personKey,
	}, nil)
	if err != nil {
		return Card{}, false, err
	}
	if response.StopReason == llm.StopRefusal || response.StopReason == llm.StopMaxToken {
		return Card{}, false, fmt.Errorf("人设卡生成没有完成（%s）", response.StopReason)
	}
	var output cardOutput
	if err := json.Unmarshal([]byte(extractJSON(response.Message.Text())), &output); err != nil {
		return Card{}, false, fmt.Errorf("解析人设卡失败: %w", err)
	}
	if strings.TrimSpace(output.Content) == "" {
		return Card{}, false, errors.New("模型返回了空的人设卡")
	}
	// 没有实质变化：不新增版本，只把参考截止时间往后挪，下次不再重复生成
	if hasCurrent && !output.Significant && !force {
		s.moveCutoff(ctx, current.CardKey, latest, count)
		return current, false, nil
	}
	card, err := s.saveVersion(ctx, familyKey, personKey, output.Content, output.Changes, int(count), latest, "auto")
	return card, err == nil, err
}

// moveCutoff 只把人设卡参考的记忆截止时间往后挪，不新增版本。
func (s *Service) moveCutoff(ctx context.Context, cardKey string, latest *time.Time, count int64) {
	s.db.WithContext(ctx).Model(&cardRow{}).Where("card_key = ?", cardKey).
		Updates(map[string]any{"memory_cutoff": latest, "memory_count": count})
}

// 一次最多拿这么多条新记忆去判断；更多时只看最新的，足够看出人设有没有变
const gateMaxMemories = 60

type gateOutput struct {
	Changed bool   `json:"changed"`
	Reason  string `json:"reason"`
}

func gateSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"changed": map[string]any{"type": "boolean", "description": "这批新记忆是否改变了人设卡里的内容（新的稳定特质、旧特质不再成立、关系格局变了）"},
			"reason":  map[string]any{"type": "string", "description": "一句话说明判断依据"},
		},
		"required": []string{"changed", "reason"},
	}
}

// personaChanged 拿上一版人设卡和上次之后新增、变动的记忆，问模型人设有没有变。
// 比整卡重写便宜得多，大多数新记忆在这一步就被挡下，只留在记忆里供对话检索。
func (s *Service) personaChanged(ctx context.Context, familyKey string, person family.Person, current Card) (bool, error) {
	fresh, _, err := s.memory.List(ctx, familyKey, memory.ListFilter{
		Person: person.PersonKey, Statuses: []string{memory.StatusActive, memory.StatusSuperseded},
		ChangedAfter: current.MemoryCutoff, Size: gateMaxMemories,
	})
	if err != nil {
		return false, err
	}
	if len(fresh) == 0 {
		return false, nil
	}
	var input strings.Builder
	fmt.Fprintf(&input, "## 要判断的人：%s（%s）\n\n## 当前人设卡（v%d）\n%s\n\n## 上次之后新增或变动的记忆\n",
		person.RoleLabel, person.Name, current.Version, current.Content)
	for _, item := range fresh {
		input.WriteString(formatMemory(item, nil) + "\n")
	}
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskGate,
		System:     []llm.SystemPart{{Text: gatePrompt, Cache: true}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: input.String()}}}},
		JSONSchema: gateSchema(),
		RefKey:     person.PersonKey,
	}, nil)
	if err != nil {
		return false, err
	}
	var output gateOutput
	if err := json.Unmarshal([]byte(extractJSON(response.Message.Text())), &output); err != nil {
		return false, fmt.Errorf("解析人设变化判断失败: %w", err)
	}
	if output.Changed {
		log.Printf("%s 的人设可能变了：%s", person.PersonKey, output.Reason)
	}
	return output.Changed, nil
}

const gatePrompt = `你在帮我（这个家的丈夫/爸爸）维护家人的人设卡。人设卡只写一个人稳定的特质：性格、说话方式、价值观和最在意的事、雷区、和家人的关系格局；孩子还包括喜好、饮食禁忌、作息、成长阶段。
具体发生的事、某一天的情绪、最近在忙什么、答应过的事，都留在记忆里，对话时会按需检索，不需要写进人设卡。

给你当前人设卡和上次之后新增或变动的记忆，判断要不要改人设卡。只有下面这些情况才算 changed=true：
- 记忆显示出卡里没有、而且不是一次性的特质（反复出现的习惯、明确表达的价值观、新的雷区）；
- 卡里写的某条特质被新记忆推翻了（以前怎样，现在不再这样）；
- 和家人的关系格局有了持续性的变化；
- 孩子的喜好、禁忌、作息、成长阶段有了明确变化。
单次事件、一时的情绪、和卡里已有描述一致的新例子、措辞上的补充，都算 changed=false。拿不准时选 false。`

// background 拼出写人设卡要参考的材料：档案、关系、当前有效的记忆和已过时的旧记忆。
func (s *Service) background(ctx context.Context, familyKey string, person family.Person, active, old []memory.Memory) string {
	personKey := person.PersonKey
	var input strings.Builder
	persons, _ := s.family.ListPersons(ctx, familyKey)
	names := map[string]string{}
	for _, item := range persons {
		names[item.PersonKey] = item.RoleLabel
	}
	fmt.Fprintf(&input, "## 要写的人：%s（%s）\n", person.RoleLabel, person.Name)
	if person.Profile != "" {
		input.WriteString("档案概况：" + person.Profile + "\n")
	}
	if relations, err := s.family.ListRelations(ctx, familyKey, personKey); err == nil && len(relations) > 0 {
		input.WriteString("关系：\n")
		for _, relation := range relations {
			fmt.Fprintf(&input, "- %s 是 %s 的%s", names[relation.ToPerson], names[relation.FromPerson], relation.Relation)
			if relation.Notes != "" {
				input.WriteString("。" + relation.Notes)
			}
			if relation.Sensitivities != "" {
				input.WriteString("。雷区：" + relation.Sensitivities)
			}
			input.WriteString("\n")
		}
	}
	input.WriteString("\n## 当前有效的记忆（新的在前）\n")
	for _, item := range active {
		input.WriteString(formatMemory(item, names) + "\n")
	}
	if len(old) > 0 {
		input.WriteString("\n## 已过时的旧记忆（用来看出变化）\n")
		for _, item := range old {
			input.WriteString(formatMemory(item, names) + "\n")
		}
	}
	return input.String()
}

func (s *Service) saveVersion(ctx context.Context, familyKey, personKey, content, changes string, count int, cutoff *time.Time, createdBy string) (Card, error) {
	var row cardRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&cardRow{}).Where("family_key = ? AND person_key = ?", familyKey, personKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		if err := tx.Model(&cardRow{}).Where("family_key = ? AND person_key = ? AND status = ?", familyKey, personKey, StatusActive).
			Update("status", StatusHistory).Error; err != nil {
			return err
		}
		row = cardRow{
			CardKey: keys.New("card"), FamilyKey: familyKey, PersonKey: personKey, Version: maxVersion + 1,
			Content: strings.TrimSpace(content), Changes: strings.TrimSpace(changes), MemoryCount: count, MemoryCutoff: cutoff,
			Status: StatusActive, CreatedBy: createdBy, CreatedAt: time.Now(),
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return Card{}, err
	}
	return row.toDTO(), nil
}

// RefreshDue 给每个核心成员（我除外）检查并按需刷新人设卡。
func (s *Service) RefreshDue(ctx context.Context, familyKey string) {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		log.Printf("刷新人设卡: %v", err)
		return
	}
	for _, person := range persons {
		if !person.IsCore || person.PersonKey == "me" {
			continue
		}
		card, changed, err := s.Generate(ctx, familyKey, person.PersonKey, false)
		if err != nil {
			log.Printf("生成 %s 的人设卡失败: %v", person.PersonKey, err)
			continue
		}
		if changed {
			log.Printf("%s 的人设卡更新到 v%d：%s", person.PersonKey, card.Version, card.Changes)
		}
	}
}

// cardSections 是人设卡的小节；孩子的卡关注点不一样。
func cardSections(person family.Person) string {
	sections := `一、一句话画像
二、性格特点
三、说话方式（语气、口头禅；有原话依据时引用一两句）
四、最在意的事和价值观
五、雷区（什么话、什么事会让她不舒服）
六、和我的关系
七、和其他家人的关系（如婆媳之间、带孩子上的分歧）`
	if strings.Contains(person.RoleLabel, "女儿") || strings.Contains(person.RoleLabel, "儿子") {
		sections = `一、一句话画像
二、性格和情绪特点（怎么哄、怎么让她配合）
三、喜好（吃的、玩的）
四、饮食禁忌和健康情况（过敏、不爱吃的）
五、作息和固定安排
六、成长阶段（语言、幼儿园、能力）
七、和每个家人的相处（谁带她、她黏谁）
八、带她的要点`
	}
	return sections
}

func systemPrompt(person family.Person) string {
	return `你在为我（这个家的丈夫/爸爸）写家人的人设卡。人设卡有两个用途：让 AI 分身模拟她的视角和说话方式；让家庭助理在给我出主意时理解她。

写作要求：
- 只写稳定的特质：反复出现、能用来预判她会怎么想怎么说的东西。具体发生的事、某段时间的情绪和压力、最近在忙什么都不写，这些留在记忆里按需检索。
- 只依据下面给出的记忆和档案，不要编造。信息不够的小节写「暂无足够信息」，不要硬凑。
- 记忆之间有冲突时，以时间更新的为准；能看出变化的，在相应小节里点明（例如「以前……，最近……」）。
- 具体，不要空话。「她很在意被尊重」不如「她很在意被当着孩子和婆婆的面否定」。
- 用第三人称，分节写，每节几句话或几个要点，全文不超过 1200 字。
- 不评判对错，不站队。

按下面的小节写（小节标题照写）：
` + cardSections(person) + `

如果给了上一版人设卡：在上一版基础上更新，保留仍然成立的内容。上一版里有些内容是我手工修订或在对话里口述补充的，没有对应的记忆，只要新记忆不和它矛盾就保留；在 changes 里用一两句话写出这次的主要变化；她的特质本身没变（只是多了几件事、换了个说法）就把 significant 设为 false。`
}

func formatMemory(item memory.Memory, names map[string]string) string {
	when := item.CreatedAt
	if item.OccurredAt != nil {
		when = *item.OccurredAt
	}
	line := fmt.Sprintf("- [%s %s] %s", when.Format("2006-01-02"), item.TypeLabel, item.Title)
	if item.Content != item.Title {
		line += "：" + oneLine(item.Content, 240)
	}
	if item.SubjectPerson != "" {
		if name := names[item.SubjectPerson]; name != "" {
			line += "（关于" + name + "）"
		}
	}
	if item.Status == memory.StatusSuperseded && item.ValidTo != nil {
		line += fmt.Sprintf("（%s 起不再成立）", item.ValidTo.Format("2006-01-02"))
	}
	return line
}

func extractJSON(text string) string {
	text = strings.TrimSpace(text)
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	if end := strings.LastIndex(text, "}"); end >= 0 && end < len(text)-1 {
		text = text[:end+1]
	}
	return text
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

// Recent 返回一段时间以来生成的新版本人设卡（不含第一版），App「今天」页展示「最近的变化」用。
func (s *Service) Recent(ctx context.Context, familyKey string, since time.Time, limit int) ([]Card, error) {
	var rows []cardRow
	if err := s.db.WithContext(ctx).Where("family_key = ? AND version > 1 AND created_at >= ?", familyKey, since).
		Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Card, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}
