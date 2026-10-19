// Package chat 是对话编排层：会话、消息、上下文组装、工具调用循环。
//
// 它处在依赖图的最上层，直接使用 family / memory / bot / llm 的服务；
// 下层领域包之间互不引用。
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"common/keys"
	"common/middleware/db"
	"service/bot"
	"service/llm"
	"service/skill"

	"gorm.io/gorm"
)

const maxToolRounds = 8

var ErrNotFound = errors.New("会话不存在")

type Service struct {
	db        *gorm.DB
	family    FamilyPort
	memory    MemoryPort
	bots      BotPort
	llm       LLMPort
	personas  PersonaPort
	reminders ReminderPort
	skills    SkillPort
	settings  SettingPort
	notes     NotePort
	diet      DietPort
}

func New(database *gorm.DB, familyService FamilyPort, memoryService MemoryPort, botService BotPort, gateway LLMPort, personaService PersonaPort, reminderService ReminderPort, skillService SkillPort, settingService SettingPort, noteService NotePort, dietService DietPort) *Service {
	return &Service{db: database, family: familyService, memory: memoryService, bots: botService, llm: gateway, personas: personaService, reminders: reminderService, skills: skillService, settings: settingService, notes: noteService, diet: dietService}
}

type Conversation struct {
	ConversationKey string     `json:"conversationKey"`
	AccountKey      string     `json:"accountKey"`
	BotKey          string     `json:"botKey"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	MessageCount    int        `json:"messageCount"`
	LastMessageAt   *time.Time `json:"lastMessageAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// Message 是对外的消息视图。tool 消息和 assistant 的工具调用前端可以折叠展示；
// notice 是系统补的固定提示卡，只给人看。
type Message struct {
	MessageKey   string          `json:"messageKey"`
	TurnKey      string          `json:"turnKey"`
	Segment      int             `json:"segment"`
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	Blocks       json.RawMessage `json:"blocks"`
	Model        string          `json:"model"`
	Usage        json.RawMessage `json:"usage"`
	Meta         json.RawMessage `json:"meta"`
	Status       string          `json:"status"`
	Error        string          `json:"error"`
	Feedback     string          `json:"feedback"`
	FeedbackNote string          `json:"feedbackNote"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Owner 标识调用方。AccountKey 为空表示管理端（不按账号过滤）。
type Owner struct {
	FamilyKey  string
	AccountKey string
}

func (s *Service) ListConversations(ctx context.Context, owner Owner, botKey string, page, size int) ([]Conversation, int64, error) {
	query := s.db.WithContext(ctx).Model(&conversationRow{}).Where("family_key = ? AND status = 'active'", owner.FamilyKey)
	if owner.AccountKey != "" {
		query = query.Where("account_key = ?", owner.AccountKey)
	}
	if botKey != "" {
		query = query.Where("bot_key = ?", botKey)
	}
	var total int64
	query.Count(&total)
	if size <= 0 || size > 100 {
		size = 30
	}
	if page <= 0 {
		page = 1
	}
	var rows []conversationRow
	if err := query.Order("COALESCE(last_message_at, created_at) DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	result := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.toDTO())
	}
	return result, total, nil
}

func (s *Service) CreateConversation(ctx context.Context, owner Owner, botKey string) (Conversation, error) {
	item, err := s.bots.Get(ctx, owner.FamilyKey, botKey)
	if err != nil {
		return Conversation{}, err
	}
	if !item.Enabled {
		return Conversation{}, errors.New("这个机器人已停用")
	}
	now := time.Now()
	row := conversationRow{
		ConversationKey: keys.New("conv"), FamilyKey: owner.FamilyKey, AccountKey: owner.AccountKey, BotKey: botKey,
		Title: "新对话", Status: "active", BotVersion: item.Version, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Conversation{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) GetConversation(ctx context.Context, owner Owner, conversationKey string) (Conversation, error) {
	row, err := s.conversation(ctx, owner, conversationKey)
	if err != nil {
		return Conversation{}, err
	}
	return row.toDTO(), nil
}

func (s *Service) ArchiveConversation(ctx context.Context, owner Owner, conversationKey string) error {
	row, err := s.conversation(ctx, owner, conversationKey)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error
}

func (s *Service) RenameConversation(ctx context.Context, owner Owner, conversationKey, title string) error {
	row, err := s.conversation(ctx, owner, conversationKey)
	if err != nil {
		return err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("标题不能为空")
	}
	return s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"title": oneLine(title, 60), "updated_at": time.Now()}).Error
}

func (s *Service) Messages(ctx context.Context, owner Owner, conversationKey string) ([]Message, error) {
	if _, err := s.conversation(ctx, owner, conversationKey); err != nil {
		return nil, err
	}
	var rows []messageRow
	if err := s.db.WithContext(ctx).Where("conversation_key = ?", conversationKey).Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]Message, 0, len(rows))
	for _, row := range rows {
		result = append(result, toMessage(row))
	}
	return result, nil
}

// SystemSnapshot 返回会话冻结的系统提示词，管理端排查「它为什么这么答」用。
func (s *Service) SystemSnapshot(ctx context.Context, owner Owner, conversationKey string) (string, error) {
	row, err := s.conversation(ctx, owner, conversationKey)
	if err != nil {
		return "", err
	}
	return row.SystemSnapshot, nil
}

func (s *Service) Feedback(ctx context.Context, owner Owner, messageKey, feedback, note string) error {
	if feedback != "up" && feedback != "down" && feedback != "" {
		return errors.New("feedback 只能是 up / down")
	}
	var row messageRow
	if err := s.db.WithContext(ctx).Where("message_key = ? AND role = ?", messageKey, llm.RoleAssistant).First(&row).Error; err != nil {
		return errors.New("消息不存在")
	}
	if _, err := s.conversation(ctx, owner, row.ConversationKey); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"feedback": feedback, "feedback_note": note}).Error; err != nil {
		return err
	}
	// 点踩并写了哪里不对：后台整理成一条待审的手册示例（设计文档 06 · M5）
	if feedback == "down" && strings.TrimSpace(note) != "" {
		go s.draftExample(context.WithoutCancel(ctx), owner.FamilyKey, row, note)
	}
	return nil
}

func (s *Service) draftExample(ctx context.Context, familyKey string, answer messageRow, note string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	var conversation conversationRow
	if err := s.db.WithContext(ctx).Where("conversation_key = ?", answer.ConversationKey).First(&conversation).Error; err != nil {
		return
	}
	var turn []messageRow
	s.db.WithContext(ctx).Where("turn_key = ?", answer.TurnKey).Order("id").Find(&turn)
	draft := skill.FeedbackDraft{MessageKey: answer.MessageKey, BotKey: conversation.BotKey, Answer: answer.Content, Note: note}
	for _, row := range turn {
		if row.Role == llm.RoleUser {
			draft.Question = row.Content
		}
		var blocks []llm.Block
		_ = json.Unmarshal(row.Blocks, &blocks)
		for _, block := range blocks {
			if block.Type == llm.BlockToolUse && block.ToolName == bot.ToolLoadSkill {
				var input struct {
					Name string `json:"name"`
				}
				if json.Unmarshal(block.Input, &input) == nil && input.Name != "" {
					draft.Loaded = append(draft.Loaded, input.Name)
				}
			}
		}
	}
	if _, err := s.skills.DraftFromFeedback(ctx, familyKey, draft); err != nil {
		log.Printf("从反馈整理示例失败 %s: %v", answer.MessageKey, err)
	}
}

// Event 是推给前端的流式事件。
type Event struct {
	Type      string   `json:"type"` // user_message / delta / assistant_message / tool_message / notice / error / done
	Text      string   `json:"text,omitempty"`
	Message   *Message `json:"message,omitempty"`
	Error     string   `json:"error,omitempty"`
	MemoryKey []string `json:"memoryKeys,omitempty"`
}

// Send 发一条消息并跑完整个回答（含工具调用）。emit 在调用方 goroutine 里同步调用。
// 每一步都先落库再推送，客户端断开不影响回答写完。
// SendInput 是我发的一条消息。NoteKey 非空表示这条消息带了照片：照片已经作为原始记录
// 存好（POST /notes，channel=capture），这一轮先当场整理成记忆，再让模型基于整理结果回答。
type SendInput struct {
	Text    string `json:"text"`
	NoteKey string `json:"noteKey"`
}

func (s *Service) Send(ctx context.Context, owner Owner, conversationKey string, input SendInput, emit func(Event)) error {
	text := strings.TrimSpace(input.Text)
	if text == "" && input.NoteKey == "" {
		return errors.New("消息不能为空")
	}
	conversation, err := s.conversation(ctx, owner, conversationKey)
	if err != nil {
		return err
	}
	item, err := s.bots.Get(ctx, owner.FamilyKey, conversation.BotKey)
	if err != nil {
		return err
	}
	var attachment *attachmentMeta
	var attachmentText string
	if input.NoteKey != "" {
		emit(Event{Type: "status", Text: "正在看照片、整理成记忆…"})
		attachment, attachmentText, err = s.ingestAttachment(ctx, owner.FamilyKey, item, input.NoteKey)
		if err != nil {
			return err
		}
		if text == "" {
			text = fmt.Sprintf("（发了 %d 张照片）", attachment.Photos)
		}
	}

	// 首条消息时冻结系统提示词，之后整个会话不再改动。
	if conversation.SystemSnapshot == "" {
		system, versions, err := s.buildSystem(ctx, owner.FamilyKey, item)
		if err != nil {
			return err
		}
		conversation.SystemSnapshot = system
		conversation.ToolsSnapshot = db.StringList(item.Tools)
		conversation.BotVersion = item.Version
		conversation.PersonaVersions = db.MustJSON(versions)
		if err := s.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", conversation.ID).
			Updates(map[string]any{"system_snapshot": system, "tools_snapshot": conversation.ToolsSnapshot, "bot_version": item.Version,
				"persona_versions": conversation.PersonaVersions}).Error; err != nil {
			return err
		}
	}
	// 工具列表不冻结：每轮按机器人当前的配置给，管理端加了工具，开着的对话下一句就能用。
	// tools_snapshot 记最近一轮实际给了哪些，排查时对得上。
	if !slices.Equal([]string(conversation.ToolsSnapshot), item.Tools) {
		conversation.ToolsSnapshot = db.StringList(item.Tools)
		if err := s.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", conversation.ID).
			Update("tools_snapshot", conversation.ToolsSnapshot).Error; err != nil {
			return err
		}
	}
	specs := toolSpecs()
	var tools []llm.Tool
	for _, name := range conversation.ToolsSnapshot {
		if spec, ok := specs[name]; ok {
			tools = append(tools, spec.definition)
		}
	}
	scope := toolScope{familyKey: owner.FamilyKey, bot: item, conversationKey: conversationKey, allowedTools: conversation.ToolsSnapshot}

	safety, budget := s.settings.Safety(ctx), s.settings.Budget(ctx)

	// 上一轮已经超出预算：先压缩成摘要、开新分段。失败就继续用全量历史。
	var userMeta turnMeta
	var blocks []llm.Block
	if tokens := s.lastInputTokens(ctx, conversation); tokens > budget.SegmentTokens {
		from := conversation.Segment
		if summary, lost, err := s.rollSegment(ctx, &conversation); err != nil {
			log.Printf("会话 %s 分段失败，继续用全量历史: %v", conversationKey, err)
		} else {
			userMeta.Segment = &segmentMeta{Rolled: true, From: from, To: conversation.Segment, Tokens: tokens,
				SummaryChars: len([]rune(summary)), Lost: lost}
			blocks = append(blocks, llm.Block{Type: llm.BlockText, Text: previousSummaryBlock(summary)})
		}
	}

	signals := crisisSignals(text, safety)
	if len(signals) > 0 {
		userMeta.Safety = &safetyMeta{Signals: signals}
	}
	// 人设卡在对话中途更新了：在这一轮参考里说明变化（系统提示词不动）
	personaText, personaChanged := s.personaUpdates(ctx, owner.FamilyKey, item, &conversation)
	if len(personaChanged) > 0 {
		userMeta.PersonaUpdates = personaChanged
	}
	turnContext, used, retrieval := s.buildTurnContext(ctx, owner.FamilyKey, item, turnInput{text: text, snapshot: conversation.SystemSnapshot, signals: signals, budget: budget, extra: personaText,
		history: s.recentLines(ctx, conversationKey, conversation.Segment)})
	userMeta.Retrieval = &retrieval
	userMeta.Attachment = attachment
	s.memory.MarkUsed(ctx, used)
	turnKey := keys.New("turn")
	segment := conversation.Segment
	blocks = append(blocks, llm.Block{Type: llm.BlockText, Text: turnContext})
	if attachmentText != "" {
		blocks = append(blocks, llm.Block{Type: llm.BlockText, Text: attachmentText})
	}
	blocks = append(blocks, llm.Block{Type: llm.BlockText, Text: text})
	userMessage, err := s.saveMessage(ctx, messageRow{
		ConversationKey: conversationKey, TurnKey: turnKey, Segment: segment, Role: llm.RoleUser, Content: text,
		Blocks: db.MustJSON(blocks), Meta: userMeta.json(),
	})
	if err != nil {
		return err
	}
	emit(Event{Type: "user_message", Message: &userMessage, MemoryKey: used})
	s.touchConversation(ctx, conversation, text)

	task := item.RouteTask
	for round := 0; round < maxToolRounds; round++ {
		history, err := s.history(ctx, conversationKey, segment)
		if err != nil {
			return err
		}
		response, err := s.llm.Chat(ctx, llm.Request{
			Task:     task,
			System:   []llm.SystemPart{{Text: conversation.SystemSnapshot, Cache: true}},
			Messages: history,
			Tools:    tools,
			RefKey:   conversationKey,
		}, func(delta llm.Delta) {
			if delta.Type == "text" {
				emit(Event{Type: "delta", Text: delta.Text})
			}
		})
		if err != nil {
			s.saveFailure(ctx, conversationKey, turnKey, segment, err)
			return err
		}
		if response.StopReason == llm.StopRefusal {
			s.saveFailure(ctx, conversationKey, turnKey, segment, errors.New("模型拒绝回答了这个问题，可以换个说法再试"))
			return errors.New("模型拒绝回答了这个问题，可以换个说法再试")
		}

		uses := response.Message.ToolUses()
		// 输出被截断时工具入参可能不完整，不执行，直接结束。
		if response.StopReason == llm.StopMaxToken && len(uses) > 0 {
			response.Message.Blocks = textOnly(response.Message.Blocks)
			response.Message.Raw = nil
			uses = nil
		}
		final := len(uses) == 0 || response.StopReason != llm.StopToolUse
		var answerMeta turnMeta
		if final {
			answer := response.Message.Text()
			answerMeta.Citations = checkCitations(answer, seenMemoryKeys(conversation.SystemSnapshot, history))
			if answerMeta.Citations != nil {
				answerMeta.Photos = s.citedPhotos(ctx, owner.FamilyKey, answerMeta.Citations.Cited)
			}
			if len(signals) > 0 {
				mentioned := mentionsHelp(answer, safety)
				answerMeta.Safety = &safetyMeta{HelpMentioned: &mentioned, NoticeAdded: !mentioned}
			}
		}
		assistant, err := s.saveMessage(ctx, messageRow{
			ConversationKey: conversationKey, TurnKey: turnKey, Segment: segment, Role: llm.RoleAssistant,
			Content: response.Message.Text(), Blocks: db.MustJSON(response.Message.Blocks),
			Provider: response.Message.Provider, Model: response.Model, Raw: db.JSON(response.Message.Raw),
			Usage: db.MustJSON(response.Usage), Meta: answerMeta.json(),
		})
		if err != nil {
			return err
		}
		emit(Event{Type: "assistant_message", Message: &assistant})
		if final {
			if answerMeta.Safety != nil && answerMeta.Safety.NoticeAdded {
				if notice, err := s.saveMessage(ctx, messageRow{
					ConversationKey: conversationKey, TurnKey: turnKey, Segment: segment, Role: RoleNotice, Content: safety.Notice,
				}); err == nil {
					emit(Event{Type: "notice", Message: &notice})
				}
			}
			s.touchConversation(ctx, conversation, "")
			emit(Event{Type: "done"})
			return nil
		}

		// 同一轮的全部工具结果放进同一条消息，顺序与调用一致。
		results := make([]llm.Block, 0, len(uses))
		var summary []string
		for _, use := range uses {
			output, runErr := s.runTool(ctx, specs, scope, use)
			isError := runErr != nil
			if isError {
				output = runErr.Error()
			}
			results = append(results, llm.Block{Type: llm.BlockToolResult, ToolUseID: use.ToolUseID, ToolName: use.ToolName, Text: clipToolOutput(output, budget.ToolOutputChars), IsError: isError})
			summary = append(summary, use.ToolName)
		}
		toolMessage, err := s.saveMessage(ctx, messageRow{
			ConversationKey: conversationKey, TurnKey: turnKey, Segment: segment, Role: "tool",
			Content: strings.Join(summary, ", "), Blocks: db.MustJSON(results),
		})
		if err != nil {
			return err
		}
		emit(Event{Type: "tool_message", Message: &toolMessage})
	}
	s.saveFailure(ctx, conversationKey, turnKey, segment, errors.New("工具调用轮数超过上限，已停止"))
	emit(Event{Type: "done"})
	return nil
}

// seenMemoryKeys 是模型在这段对话里见过的记忆编号：系统提示词 + 当前分段的全部历史。
func seenMemoryKeys(system string, history []llm.Message) map[string]bool {
	seen := map[string]bool{}
	add := func(text string) {
		for _, key := range memoryKeys(text) {
			seen[key] = true
		}
	}
	add(system)
	for _, message := range history {
		for _, block := range message.Blocks {
			add(block.Text)
			add(string(block.Input))
		}
	}
	return seen
}

func (s *Service) runTool(ctx context.Context, specs map[string]toolSpec, scope toolScope, use llm.Block) (output string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("工具 %s panic: %v", use.ToolName, recovered)
			err = fmt.Errorf("工具执行出错")
		}
	}()
	allowed := false
	for _, name := range scope.allowedTools {
		if name == use.ToolName {
			allowed = true
		}
	}
	spec, ok := specs[use.ToolName]
	if !ok || !allowed {
		return "", fmt.Errorf("没有这个工具：%s", use.ToolName)
	}
	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return spec.run(toolCtx, s, scope, use.Input)
}

// history 把会话当前分段里成功的消息还原成模型历史。
// 只追加、不改写：同一分段每次发出去的历史前缀字节一致。
func (s *Service) history(ctx context.Context, conversationKey string, segment int) ([]llm.Message, error) {
	var rows []messageRow
	if err := s.db.WithContext(ctx).Where("conversation_key = ? AND segment = ? AND status = 'done' AND role <> ?", conversationKey, segment, RoleNotice).
		Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var messages []llm.Message
	for index, row := range rows {
		var blocks []llm.Block
		if err := json.Unmarshal(row.Blocks, &blocks); err != nil {
			continue
		}
		message := llm.Message{Role: row.Role, Blocks: blocks}
		switch row.Role {
		case llm.RoleAssistant:
			message.Provider = row.Provider
			message.Raw = json.RawMessage(row.Raw)
			// 发起了工具调用却没有对应结果（进程中途退出）的 assistant 消息不能回放，否则接口报错。
			if len(message.ToolUses()) > 0 && (index+1 >= len(rows) || rows[index+1].Role != "tool") {
				message.Blocks = textOnly(blocks)
				message.Raw = nil
				if len(message.Blocks) == 0 {
					continue
				}
			}
		case "tool":
			message.Role = llm.RoleUser
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (s *Service) conversation(ctx context.Context, owner Owner, conversationKey string) (conversationRow, error) {
	query := s.db.WithContext(ctx).Where("family_key = ? AND conversation_key = ?", owner.FamilyKey, conversationKey)
	if owner.AccountKey != "" {
		query = query.Where("account_key = ?", owner.AccountKey)
	}
	var row conversationRow
	if err := query.First(&row).Error; err != nil {
		return row, ErrNotFound
	}
	return row, nil
}

func (s *Service) saveMessage(ctx context.Context, row messageRow) (Message, error) {
	row.MessageKey = keys.New("msg")
	if row.Status == "" {
		row.Status = "done"
	}
	row.CreatedAt = time.Now()
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return Message{}, err
	}
	return toMessage(row), nil
}

func (s *Service) saveFailure(ctx context.Context, conversationKey, turnKey string, segment int, cause error) {
	_, _ = s.saveMessage(context.WithoutCancel(ctx), messageRow{
		ConversationKey: conversationKey, TurnKey: turnKey, Segment: segment, Role: llm.RoleAssistant, Content: "",
		Status: "error", Error: oneLine(cause.Error(), 900),
	})
}

func (s *Service) touchConversation(ctx context.Context, conversation conversationRow, firstText string) {
	now := time.Now()
	updates := map[string]any{"last_message_at": now, "updated_at": now, "message_count": gorm.Expr("(SELECT COUNT(*) FROM pa_message WHERE conversation_key = ?)", conversation.ConversationKey)}
	if firstText != "" && conversation.Title == "新对话" {
		updates["title"] = oneLine(firstText, 30)
	}
	s.db.WithContext(context.WithoutCancel(ctx)).Model(&conversationRow{}).Where("id = ?", conversation.ID).Updates(updates)
}

func textOnly(blocks []llm.Block) []llm.Block {
	var result []llm.Block
	for _, block := range blocks {
		if block.Type == llm.BlockText && block.Text != "" {
			result = append(result, block)
		}
	}
	return result
}

func toMessage(row messageRow) Message {
	return Message{
		MessageKey: row.MessageKey, TurnKey: row.TurnKey, Segment: row.Segment, Role: row.Role, Content: row.Content,
		Blocks: json.RawMessage(row.Blocks), Model: row.Model, Usage: json.RawMessage(row.Usage), Meta: json.RawMessage(row.Meta), Status: row.Status,
		Error: row.Error, Feedback: row.Feedback, FeedbackNote: row.FeedbackNote, CreatedAt: row.CreatedAt,
	}
}

// EvalCase 把一条被点踩的回答转成评测用例（设计文档 05 · 3.3）：
// 这轮及之前的用户原话作为 turns，反馈作为打分项。里面是真实对话，进仓库前要改写成虚构内容。
func (s *Service) EvalCase(ctx context.Context, owner Owner, messageKey string) (map[string]any, error) {
	var target messageRow
	if err := s.db.WithContext(ctx).Where("message_key = ? AND role = ?", messageKey, llm.RoleAssistant).First(&target).Error; err != nil {
		return nil, errors.New("消息不存在")
	}
	conversation, err := s.conversation(ctx, owner, target.ConversationKey)
	if err != nil {
		return nil, err
	}
	var users []messageRow
	if err := s.db.WithContext(ctx).Where("conversation_key = ? AND role = ? AND id <= ?", target.ConversationKey, llm.RoleUser, target.ID).
		Order("id").Find(&users).Error; err != nil {
		return nil, err
	}
	turns := make([]string, 0, len(users))
	for _, row := range users {
		turns = append(turns, row.Content)
	}
	rubric := []string{}
	if note := strings.TrimSpace(target.FeedbackNote); note != "" {
		rubric = append(rubric, "避免这次反馈指出的问题："+note)
	}
	return map[string]any{
		"name":  "反馈-" + oneLine(conversation.Title, 20),
		"bot":   conversation.BotKey,
		"turns": turns,
		"expect": map[string]any{
			"rubric": rubric,
		},
		"_badAnswer": target.Content,
		"_note":      "来自真实对话：提交进仓库前把人名、原话改写成 fixture.json 里的虚构内容，并删掉 _ 开头的字段",
	}, nil
}

// SafetyHit 是一次危机词命中，管理端用来判断词表准不准。
type SafetyHit struct {
	MessageKey      string    `json:"messageKey"`
	ConversationKey string    `json:"conversationKey"`
	Content         string    `json:"content"`
	Signals         []string  `json:"signals"`
	HelpMentioned   *bool     `json:"helpMentioned"`
	NoticeAdded     bool      `json:"noticeAdded"`
	CreatedAt       time.Time `json:"createdAt"`
}

// SafetyHits 列出最近命中危机词的用户消息，以及这一轮回答有没有给出求助建议。
func (s *Service) SafetyHits(ctx context.Context, owner Owner, limit int) ([]SafetyHit, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []messageRow
	if err := s.db.WithContext(ctx).Table("pa_message AS m").Select("m.*").
		Joins("JOIN pa_conversation c ON c.conversation_key = m.conversation_key").
		Where("c.family_key = ? AND m.role = ? AND JSON_LENGTH(JSON_EXTRACT(m.meta, '$.safety.signals')) > 0", owner.FamilyKey, llm.RoleUser).
		Order("m.id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SafetyHit, 0, len(rows))
	for _, row := range rows {
		var meta turnMeta
		_ = json.Unmarshal(row.Meta, &meta)
		hit := SafetyHit{MessageKey: row.MessageKey, ConversationKey: row.ConversationKey, Content: row.Content, CreatedAt: row.CreatedAt}
		if meta.Safety != nil {
			hit.Signals = meta.Safety.Signals
		}
		var answers []messageRow
		s.db.WithContext(ctx).Where("turn_key = ? AND role = ? AND meta IS NOT NULL", row.TurnKey, llm.RoleAssistant).Order("id DESC").Limit(1).Find(&answers)
		if len(answers) > 0 {
			var answerMeta turnMeta
			if json.Unmarshal(answers[0].Meta, &answerMeta) == nil && answerMeta.Safety != nil {
				hit.HelpMentioned, hit.NoticeAdded = answerMeta.Safety.HelpMentioned, answerMeta.Safety.NoticeAdded
			}
		}
		result = append(result, hit)
	}
	return result, nil
}

// SegmentRoll 是一次长对话分段，管理端用来判断分段阈值合不合适。
type SegmentRoll struct {
	MessageKey      string    `json:"messageKey"`
	ConversationKey string    `json:"conversationKey"`
	Title           string    `json:"title"`
	To              int       `json:"to"`
	Tokens          int       `json:"tokens"`
	SummaryChars    int       `json:"summaryChars"`
	Lost            []string  `json:"lost"`
	CreatedAt       time.Time `json:"createdAt"`
}

func (s *Service) SegmentRolls(ctx context.Context, owner Owner, limit int) ([]SegmentRoll, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []struct {
		messageRow
		Title string
	}
	if err := s.db.WithContext(ctx).Table("pa_message AS m").Select("m.*, c.title").
		Joins("JOIN pa_conversation c ON c.conversation_key = m.conversation_key").
		Where("c.family_key = ? AND m.role = ? AND JSON_EXTRACT(m.meta, '$.segment.rolled') = true", owner.FamilyKey, llm.RoleUser).
		Order("m.id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SegmentRoll, 0, len(rows))
	for _, row := range rows {
		var meta turnMeta
		if json.Unmarshal(row.Meta, &meta) != nil || meta.Segment == nil {
			continue
		}
		lost := meta.Segment.Lost
		if lost == nil {
			lost = []string{}
		}
		result = append(result, SegmentRoll{
			MessageKey: row.MessageKey, ConversationKey: row.ConversationKey, Title: row.Title, To: meta.Segment.To,
			Tokens: meta.Segment.Tokens, SummaryChars: meta.Segment.SummaryChars, Lost: lost, CreatedAt: row.CreatedAt,
		})
	}
	return result, nil
}
