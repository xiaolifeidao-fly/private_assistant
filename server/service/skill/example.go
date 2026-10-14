package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"common/keys"
	"service/llm"
)

// 手册示例（设计文档 06 · M5 话术库）：原设计里的 Script 表并进手册 —— 每本手册下挂几条
// 「什么情况 → 好的说法 / 别这么说 / 为什么」。审过的示例随手册正文一起给模型（few-shot）。
// 点踩并写了「哪里不对」的回答，会自动整理成一条待审示例。

const (
	ExamplePending  = "pending"
	ExampleApproved = "approved"
	ExampleRejected = "rejected"

	taskExample = "skill.example"
	// 每本手册最多带这么多条示例，新的优先
	examplesPerSkill = 5
)

type Example struct {
	ExampleKey       string    `json:"exampleKey"`
	SkillKey         string    `json:"skillKey"`
	Situation        string    `json:"situation"`
	GoodResponse     string    `json:"goodResponse"`
	BadResponse      string    `json:"badResponse"`
	Why              string    `json:"why"`
	Status           string    `json:"status"`
	Source           string    `json:"source"`
	SourceMessageKey string    `json:"sourceMessageKey"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// ListExamples 列出示例。skillKey、status 为空表示不限。
func (s *Service) ListExamples(ctx context.Context, familyKey, skillKey, status string) ([]Example, error) {
	query := s.db.WithContext(ctx).Where("family_key = ?", familyKey)
	if skillKey != "" {
		query = query.Where("skill_key = ?", skillKey)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var rows []exampleRow
	if err := query.Order("id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Example, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, nil
}

// SaveExample 新建（ExampleKey 为空）或修改示例。手工新建的直接生效。
func (s *Service) SaveExample(ctx context.Context, familyKey string, input Example) (Example, error) {
	input.Situation, input.GoodResponse = strings.TrimSpace(input.Situation), strings.TrimSpace(input.GoodResponse)
	if input.Situation == "" || input.GoodResponse == "" {
		return Example{}, errors.New("情况和好的说法不能为空")
	}
	if _, err := s.Get(ctx, familyKey, input.SkillKey); err != nil {
		return Example{}, err
	}
	if input.Status == "" {
		input.Status = ExampleApproved
	}
	if !validExampleStatus(input.Status) {
		return Example{}, errors.New("状态只能是 pending / approved / rejected")
	}
	now := time.Now()
	if input.ExampleKey == "" {
		row := exampleRow{
			ExampleKey: keys.New("exm"), FamilyKey: familyKey, SkillKey: input.SkillKey, Situation: input.Situation,
			GoodResponse: input.GoodResponse, BadResponse: input.BadResponse, Why: input.Why, Status: input.Status,
			Source: defaultSource(input.Source), SourceMessageKey: nullable(input.SourceMessageKey), CreatedAt: now, UpdatedAt: now,
		}
		if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
			return Example{}, err
		}
		return row.toDTO(), nil
	}
	result := s.db.WithContext(ctx).Model(&exampleRow{}).Where("family_key = ? AND example_key = ?", familyKey, input.ExampleKey).
		Updates(map[string]any{
			"skill_key": input.SkillKey, "situation": input.Situation, "good_response": input.GoodResponse,
			"bad_response": input.BadResponse, "why": input.Why, "status": input.Status, "updated_at": now,
		})
	if result.Error != nil {
		return Example{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Example{}, errors.New("示例不存在")
	}
	var row exampleRow
	if err := s.db.WithContext(ctx).Where("example_key = ?", input.ExampleKey).First(&row).Error; err != nil {
		return Example{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) SetExampleStatus(ctx context.Context, familyKey, exampleKey, status string) error {
	if !validExampleStatus(status) {
		return errors.New("状态只能是 pending / approved / rejected")
	}
	return s.db.WithContext(ctx).Model(&exampleRow{}).Where("family_key = ? AND example_key = ?", familyKey, exampleKey).
		Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error
}

// CountPendingExamples 是待审示例数，管理端侧边栏提示用。
func (s *Service) CountPendingExamples(ctx context.Context, familyKey string) int64 {
	var count int64
	s.db.WithContext(ctx).Model(&exampleRow{}).Where("family_key = ? AND status = ?", familyKey, ExamplePending).Count(&count)
	return count
}

// renderExamples 把审过的示例拼在手册正文后面。
func (s *Service) renderExamples(ctx context.Context, familyKey, skillKey string) string {
	var rows []exampleRow
	s.db.WithContext(ctx).Where("family_key = ? AND skill_key = ? AND status = ?", familyKey, skillKey, ExampleApproved).
		Order("id DESC").Limit(examplesPerSkill).Find(&rows)
	if len(rows) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("\n\n## 我们家的示例\n以下是以前真实发生过、我确认过的处理方式，同类情况照着它的思路来。\n")
	for index, row := range rows {
		fmt.Fprintf(&builder, "\n### 示例 %d\n- 情况：%s\n- 好的说法：%s\n", index+1, row.Situation, row.GoodResponse)
		if strings.TrimSpace(row.BadResponse) != "" {
			builder.WriteString("- 别这么说：" + row.BadResponse + "\n")
		}
		if strings.TrimSpace(row.Why) != "" {
			builder.WriteString("- 为什么：" + row.Why + "\n")
		}
	}
	return builder.String()
}

// Render 返回 load_skill 给模型的完整内容：正文 + 审过的示例。
func (s *Service) Render(ctx context.Context, familyKey string, item Skill) string {
	return fmt.Sprintf("# %s（v%d）\n\n%s%s", item.Name, item.Version, item.Body, s.renderExamples(ctx, familyKey, item.SkillKey))
}

// FeedbackDraft 是一条被点踩的回答，用来整理示例。
type FeedbackDraft struct {
	MessageKey string
	BotKey     string
	Question   string   // 这一轮我说的话
	Answer     string   // 被点踩的回答
	Note       string   // 我写的「哪里不对」
	Loaded     []string // 这一轮读过的手册
}

const examplePrompt = `我在用一个家庭关系助理，它的一条回答被我点了踩，并写了哪里不对。
请把这次的教训整理成一条「手册示例」，以后同类情况给助理参考：

- skill：从给出的手册列表里选最相关的一本（填 key）；
- situation：一两句话概括是什么情况（去掉无关细节，保留关键的人和事）；
- goodResponse：按我的反馈，应该怎么回答或怎么说，写成可以直接用的说法；
- badResponse：这次回答里不该说的、或者应该避免的说法（简短）；
- why：背后的原则，一句话。

只根据我的反馈来改，不要加我没提到的判断。`

// DraftFromFeedback 把点踩 + 反馈整理成一条待审示例。没写反馈的不整理。
func (s *Service) DraftFromFeedback(ctx context.Context, familyKey string, draft FeedbackDraft) (Example, error) {
	if strings.TrimSpace(draft.Note) == "" {
		return Example{}, errors.New("没有写哪里不对，无法整理示例")
	}
	if s.llm == nil {
		return Example{}, errors.New("示例整理没有装配")
	}
	skills, err := s.ForBot(ctx, familyKey, draft.BotKey)
	if err != nil {
		return Example{}, err
	}
	if len(skills) == 0 {
		return Example{}, errors.New("这个机器人没有可用的手册")
	}
	var skillKeys []string
	var builder strings.Builder
	builder.WriteString("<skills>\n")
	for _, item := range skills {
		skillKeys = append(skillKeys, item.SkillKey)
		fmt.Fprintf(&builder, "- %s：%s —— %s\n", item.SkillKey, item.Name, item.WhenToUse)
	}
	builder.WriteString("</skills>\n")
	if len(draft.Loaded) > 0 {
		builder.WriteString("这一轮助理读过的手册：" + strings.Join(draft.Loaded, "、") + "\n")
	}
	fmt.Fprintf(&builder, "\n<question>%s</question>\n<answer>%s</answer>\n<feedback>%s</feedback>", draft.Question, oneLine(draft.Answer, 3000), draft.Note)

	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskExample,
		System:     []llm.SystemPart{{Text: examplePrompt}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: exampleSchema(skillKeys),
		RefKey:     draft.MessageKey,
	}, nil)
	if err != nil {
		return Example{}, err
	}
	text := response.Message.Text()
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	var parsed struct {
		Skill        string `json:"skill"`
		Situation    string `json:"situation"`
		GoodResponse string `json:"goodResponse"`
		BadResponse  string `json:"badResponse"`
		Why          string `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return Example{}, fmt.Errorf("解析示例失败: %w", err)
	}
	skillKey := parsed.Skill
	if !contains(skillKeys, skillKey) {
		skillKey = skillKeys[0]
		if len(draft.Loaded) > 0 && contains(skillKeys, draft.Loaded[0]) {
			skillKey = draft.Loaded[0]
		}
	}
	return s.SaveExample(ctx, familyKey, Example{
		SkillKey: skillKey, Situation: parsed.Situation, GoodResponse: parsed.GoodResponse, BadResponse: parsed.BadResponse,
		Why: parsed.Why, Status: ExamplePending, Source: "feedback", SourceMessageKey: draft.MessageKey,
	})
}

func exampleSchema(skillKeys []string) map[string]any {
	text := map[string]any{"type": "string"}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"skill": map[string]any{"type": "string", "enum": skillKeys}, "situation": text,
			"goodResponse": text, "badResponse": text, "why": text,
		},
		"required":             []string{"skill", "situation", "goodResponse", "badResponse", "why"},
		"additionalProperties": false,
	}
}

func validExampleStatus(status string) bool {
	return status == ExamplePending || status == ExampleApproved || status == ExampleRejected
}

func defaultSource(source string) string {
	if source == "" {
		return "manual"
	}
	return source
}

func nullable(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

type exampleRow struct {
	ID               int64
	ExampleKey       string
	FamilyKey        string
	SkillKey         string
	Situation        string
	GoodResponse     string
	BadResponse      string
	Why              string
	Status           string
	Source           string
	SourceMessageKey *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (exampleRow) TableName() string { return "pa_skill_example" }

func (row exampleRow) toDTO() Example {
	item := Example{
		ExampleKey: row.ExampleKey, SkillKey: row.SkillKey, Situation: row.Situation, GoodResponse: row.GoodResponse,
		BadResponse: row.BadResponse, Why: row.Why, Status: row.Status, Source: row.Source, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.SourceMessageKey != nil {
		item.SourceMessageKey = *row.SourceMessageKey
	}
	return item
}
