package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"service/bot"
	"service/family"
	"service/llm"
	"service/memory"
	"service/reminder"
)

// toolScope 是一次工具调用的上下文：哪个家庭、哪个机器人（决定记忆可见范围）。
type toolScope struct {
	familyKey       string
	bot             bot.Bot
	conversationKey string
	allowedTools    []string // 会话冻结的工具列表
}

func (scope toolScope) visibleTo() string {
	if scope.bot.MemoryScope == bot.ScopePerson {
		return scope.bot.PersonKey
	}
	return ""
}

type toolSpec struct {
	definition llm.Tool
	run        func(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error)
}

var memoryTypeNames = func() []string {
	names := make([]string, 0, len(memory.Types))
	for _, info := range memory.Types {
		names = append(names, info.Type)
	}
	return names
}()

func memoryTypeGuide() string {
	var parts []string
	for _, info := range memory.Types {
		if info.Type == "insight" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s（%s）", info.Type, info.Label, info.Hint))
	}
	return strings.Join(parts, "；")
}

func toolSpecs() map[string]toolSpec {
	return map[string]toolSpec{
		bot.ToolSearchMemory: {
			definition: llm.Tool{
				Name:        bot.ToolSearchMemory,
				Description: "检索家庭记忆（原话、性格、习惯、事件、承诺、规矩……）。回答「以前说过/做过什么」或需要依据时先用它。",
				Properties: map[string]any{
					"query":           map[string]any{"type": "string", "description": "要查什么，用自然语言描述"},
					"person":          map[string]any{"type": "string", "description": "只看关于某人的记忆，填人物 key 或称呼，可选"},
					"types":           map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": memoryTypeNames}, "description": "限定记忆类型，可选"},
					"include_history": map[string]any{"type": "boolean", "description": "是否包含已过时的旧记忆，问「她以前怎么想」时设为 true"},
				},
				Required: []string{"query"},
			},
			run: runSearchMemory,
		},
		bot.ToolGetPerson: {
			definition: llm.Tool{
				Name:        bot.ToolGetPerson,
				Description: "查看某个人的完整档案：基本信息、和家人的关系、关于他/她的重要记忆。",
				Properties:  map[string]any{"person": map[string]any{"type": "string", "description": "人物 key 或称呼，如 wife、老婆、大舅"}},
				Required:    []string{"person"},
			},
			run: runGetPerson,
		},
		bot.ToolGetRelations: {
			definition: llm.Tool{
				Name:        bot.ToolGetRelations,
				Description: "查看某人的社会关系：称呼、亲疏、关系要点和雷区。处理亲戚关系、回老家前用它。",
				Properties:  map[string]any{"person": map[string]any{"type": "string", "description": "人物 key 或称呼"}},
				Required:    []string{"person"},
			},
			run: runGetRelations,
		},
		bot.ToolGetPlace: {
			definition: llm.Tool{
				Name:        bot.ToolGetPlace,
				Description: "查看某个家/老家的信息：住着谁、规矩习俗、去之前要准备什么。",
				Properties:  map[string]any{"place": map[string]any{"type": "string", "description": "地点 key 或名称，如 home_wife_parents、岳父母老家"}},
				Required:    []string{"place"},
			},
			run: runGetPlace,
		},
		bot.ToolOpenItems: {
			definition: llm.Tool{
				Name:        bot.ToolOpenItems,
				Description: "列出还没完成的事：我答应过的承诺、家人提过还没落实的诉求。",
				Properties:  map[string]any{},
			},
			run: runOpenItems,
		},
		bot.ToolRemember: {
			definition: llm.Tool{
				Name: bot.ToolRemember,
				Description: "记下一条值得长期记住的信息。记之前先用 search_memory 查重。" +
					"如果新信息和已有记忆冲突（她变了），填 supersedes 指向旧记忆，系统会让我确认后再替代。类型：" + memoryTypeGuide(),
				Properties: map[string]any{
					"type":           map[string]any{"type": "string", "enum": memoryTypeNames},
					"title":          map[string]any{"type": "string", "description": "一句话摘要，60 字以内"},
					"content":        map[string]any{"type": "string", "description": "完整内容；原话类尽量保留原文"},
					"subject_person": map[string]any{"type": "string", "description": "主要关于谁（人物 key 或称呼）；全家通用的留空"},
					"persons":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "还涉及哪些人"},
					"place":          map[string]any{"type": "string", "description": "相关地点 key，规矩习俗类必填"},
					"importance":     map[string]any{"type": "integer", "minimum": 1, "maximum": 5, "description": "1-5，雷区、重要诉求、承诺一般 4-5"},
					"occurred_at":    map[string]any{"type": "string", "description": "事情发生/说这话的时间，YYYY-MM-DD 或 YYYY-MM-DD HH:MM；把「昨天」「上周三」换算成具体日期"},
					"attrs":          map[string]any{"type": "object", "description": "类型相关的结构化字段，如 commitment 的 {\"promisedTo\":\"wife\",\"dueAt\":\"2026-10-20\",\"status\":\"open\"}"},
					"confidence":     map[string]any{"type": "number", "description": "0-1，我明确说了的=1，你推断的给低一些"},
					"supersedes":     map[string]any{"type": "string", "description": "要替代的旧记忆编号 mem_xxx，可选"},
				},
				Required: []string{"type", "title", "content"},
			},
			run: runRemember,
		},
		bot.ToolAskPersona: {
			definition: llm.Tool{
				Name:        bot.ToolAskPersona,
				Description: "预演：把一句我打算说的话交给家人的分身，看她听了第一反应是什么、哪里会不舒服。分身是基于记忆的模拟，不是本人。",
				Properties: map[string]any{
					"persona": map[string]any{"type": "string", "enum": []string{"wife", "mom"}, "description": "分身机器人标识：wife（老婆）、mom（妈妈）"},
					"message": map[string]any{"type": "string", "description": "我打算对她说的原话"},
				},
				Required: []string{"persona", "message"},
			},
			run: runAskPersona,
		},
		bot.ToolListUpcoming: {
			definition: llm.Tool{
				Name: bot.ToolListUpcoming, Description: "查看近期生日、承诺到期、节日、行程和手工提醒。",
				Properties: map[string]any{"days": map[string]any{"type": "integer", "minimum": 1, "maximum": 366, "description": "查未来多少天，默认 30"}},
			},
			run: runListUpcoming,
		},
		bot.ToolCreateReminder: {
			definition: llm.Tool{
				Name: bot.ToolCreateReminder, Description: "建立一条定时提醒。只在我明确要求提醒，或我确认你的提醒建议后调用。",
				Properties: map[string]any{
					"title":     map[string]any{"type": "string", "description": "提醒标题"},
					"body":      map[string]any{"type": "string", "description": "要提醒的具体内容"},
					"remind_at": map[string]any{"type": "string", "description": "提醒时间，YYYY-MM-DD HH:MM"},
					"event_at":  map[string]any{"type": "string", "description": "事情发生时间，YYYY-MM-DD HH:MM，可选"},
				}, Required: []string{"title", "remind_at"},
			},
			run: runCreateReminder,
		},
		bot.ToolLoadSkill: {
			definition: llm.Tool{
				Name:        bot.ToolLoadSkill,
				Description: "读一本做事手册的正文：遇到这类事该先查什么、怎么想、怎么说、别做什么。系统提示词「做事手册」一节列了有哪些，参数填手册的 key。",
				Properties:  map[string]any{"name": map[string]any{"type": "string", "description": "手册 key，如 mother_wife"}},
				Required:    []string{"name"},
			},
			run: runLoadSkill,
		},
		bot.ToolNow: {
			definition: llm.Tool{Name: bot.ToolNow, Description: "获取当前日期、时间和星期。", Properties: map[string]any{}},
			run: func(context.Context, *Service, toolScope, json.RawMessage) (string, error) {
				return formatNow(time.Now()) + "，" + reminder.LunarToday(time.Now()), nil
			},
		},
	}
}

func decodeInput(input json.RawMessage, target any) error {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	if err := json.Unmarshal(input, target); err != nil {
		return fmt.Errorf("INVALID_JSON: %s", string(input))
	}
	return nil
}

func runSearchMemory(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Query          string   `json:"query"`
		Person         string   `json:"person"`
		Types          []string `json:"types"`
		IncludeHistory bool     `json:"include_history"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Query) == "" {
		return "", errors.New("query 不能为空")
	}
	dir, err := s.loadDirectory(ctx, scope.familyKey)
	if err != nil {
		return "", err
	}
	query := memory.SearchQuery{Text: args.Query, Types: args.Types, IncludeHistory: args.IncludeHistory, Limit: 15, VisibleTo: scope.visibleTo()}
	if args.Person != "" {
		person, err := s.family.ResolvePerson(ctx, scope.familyKey, args.Person)
		if err != nil {
			return "", fmt.Errorf("认不出「%s」是谁", args.Person)
		}
		query.Persons = []string{person.PersonKey}
	} else {
		query.Persons = dir.mentioned(args.Query)
	}
	hits, err := s.memory.Search(ctx, scope.familyKey, query)
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "没有找到相关记忆。", nil
	}
	var lines []string
	var used []string
	for _, hit := range hits {
		lines = append(lines, formatMemory(dir, hit.Memory))
		used = append(used, hit.Memory.MemoryKey)
	}
	s.memory.MarkUsed(ctx, used)
	return strings.Join(lines, "\n"), nil
}

func runGetPerson(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Person string `json:"person"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	person, err := s.family.ResolvePerson(ctx, scope.familyKey, args.Person)
	if err != nil {
		return "", fmt.Errorf("认不出「%s」是谁", args.Person)
	}
	if visible := scope.visibleTo(); visible != "" && person.PersonKey != visible {
		return "", errors.New("分身只能查看自己的档案")
	}
	dir, err := s.loadDirectory(ctx, scope.familyKey)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s（key=%s）\n", dir.personName(person.PersonKey), person.PersonKey)
	if len(person.Aliases) > 0 {
		builder.WriteString("别名：" + strings.Join(person.Aliases, "、") + "\n")
	}
	if birthday := birthdayText(person); birthday != "" {
		builder.WriteString("生日：" + birthday + "\n")
	}
	if place, ok := dir.places[person.HomePlaceKey]; ok {
		builder.WriteString("住在：" + place.Name + "\n")
	}
	if person.Profile != "" {
		builder.WriteString("概况：" + person.Profile + "\n")
	}
	if card, ok, cardErr := s.personas.Active(ctx, scope.familyKey, person.PersonKey); cardErr == nil && ok {
		fmt.Fprintf(&builder, "人设卡（v%d，%s 更新）：\n%s\n", card.Version, card.CreatedAt.Format("2006-01-02"), card.Content)
	}
	relations, _ := s.family.ListRelations(ctx, scope.familyKey, person.PersonKey)
	if len(relations) > 0 {
		builder.WriteString("关系：\n")
		for _, relation := range relations {
			builder.WriteString("- " + formatRelation(dir, relation) + "\n")
		}
	}
	core, _ := s.memory.Core(ctx, scope.familyKey, memory.CoreFilter{Persons: []string{person.PersonKey}, MinImportance: 3, Limit: 30, VisibleTo: scope.visibleTo()})
	if len(core) > 0 {
		builder.WriteString("重要记忆：\n")
		for _, item := range core {
			builder.WriteString(formatMemory(dir, item) + "\n")
		}
	}
	return builder.String(), nil
}

func runGetRelations(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Person string `json:"person"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	person, err := s.family.ResolvePerson(ctx, scope.familyKey, args.Person)
	if err != nil {
		return "", fmt.Errorf("认不出「%s」是谁", args.Person)
	}
	dir, err := s.loadDirectory(ctx, scope.familyKey)
	if err != nil {
		return "", err
	}
	relations, err := s.family.ListRelations(ctx, scope.familyKey, person.PersonKey)
	if err != nil {
		return "", err
	}
	if len(relations) == 0 {
		return "档案里还没有登记 " + dir.personName(person.PersonKey) + " 的关系。", nil
	}
	var lines []string
	for _, relation := range relations {
		lines = append(lines, "- "+formatRelation(dir, relation))
	}
	return strings.Join(lines, "\n"), nil
}

func runGetPlace(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Place string `json:"place"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	dir, err := s.loadDirectory(ctx, scope.familyKey)
	if err != nil {
		return "", err
	}
	var place family.Place
	found := false
	for _, candidate := range sortedPlaces(dir.places) {
		if candidate.PlaceKey == args.Place || candidate.Name == args.Place || strings.Contains(candidate.Name, args.Place) {
			place, found = candidate, true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("没有找到地点「%s」", args.Place)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s（key=%s）\n", place.Name, place.PlaceKey)
	for _, field := range [][2]string{{"地址", place.Address}, {"方言", place.Dialect}, {"介绍", place.Description}, {"规矩习俗", place.Customs}, {"去之前准备", place.TravelTips}} {
		if field[1] != "" {
			builder.WriteString(field[0] + "：" + field[1] + "\n")
		}
	}
	var residents []string
	for _, person := range dir.ordered {
		if person.HomePlaceKey == place.PlaceKey {
			residents = append(residents, dir.personName(person.PersonKey))
		}
	}
	if len(residents) > 0 {
		builder.WriteString("住在这里：" + strings.Join(residents, "、") + "\n")
	}
	customs, _, err := s.memory.List(ctx, scope.familyKey, memory.ListFilter{
		Types: []string{"custom", "taboo"}, Statuses: []string{memory.StatusActive}, Size: 50, VisibleTo: scope.visibleTo(),
	})
	if err == nil {
		var lines []string
		for _, item := range customs {
			if item.PlaceKey == place.PlaceKey {
				lines = append(lines, formatMemory(dir, item))
			}
		}
		if len(lines) > 0 {
			builder.WriteString("相关记忆：\n" + strings.Join(lines, "\n") + "\n")
		}
	}
	return builder.String(), nil
}

func runOpenItems(ctx context.Context, s *Service, scope toolScope, _ json.RawMessage) (string, error) {
	dir, err := s.loadDirectory(ctx, scope.familyKey)
	if err != nil {
		return "", err
	}
	items, _, err := s.memory.List(ctx, scope.familyKey, memory.ListFilter{Types: []string{"commitment", "need"}, Statuses: []string{memory.StatusActive, memory.StatusPending}, Size: 100})
	if err != nil {
		return "", err
	}
	var lines []string
	for _, item := range items {
		var attrs struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(item.Attrs, &attrs)
		if attrs.Status == "" || attrs.Status == "open" {
			lines = append(lines, formatMemory(dir, item))
		}
	}
	if len(lines) == 0 {
		return "没有未完成的承诺或诉求。", nil
	}
	return strings.Join(lines, "\n"), nil
}

func runRemember(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Type          string          `json:"type"`
		Title         string          `json:"title"`
		Content       string          `json:"content"`
		SubjectPerson string          `json:"subject_person"`
		Persons       []string        `json:"persons"`
		Place         string          `json:"place"`
		Importance    int             `json:"importance"`
		OccurredAt    string          `json:"occurred_at"`
		Attrs         json.RawMessage `json:"attrs"`
		Confidence    float64         `json:"confidence"`
		Supersedes    string          `json:"supersedes"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Title) == "" && strings.TrimSpace(args.Content) == "" {
		return "", errors.New("title 和 content 不能都为空")
	}
	resolve := func(mention string) (string, error) {
		if strings.TrimSpace(mention) == "" {
			return "", nil
		}
		person, err := s.family.ResolvePerson(ctx, scope.familyKey, mention)
		if err != nil {
			return "", fmt.Errorf("认不出「%s」是谁，请先让我在家庭档案里添加这个人，或换成已有的称呼", mention)
		}
		return person.PersonKey, nil
	}
	subject, err := resolve(args.SubjectPerson)
	if err != nil {
		return "", err
	}
	var persons []string
	for _, mention := range args.Persons {
		key, err := resolve(mention)
		if err != nil {
			return "", err
		}
		persons = append(persons, key)
	}
	var occurred *time.Time
	if value := strings.TrimSpace(args.OccurredAt); value != "" {
		for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				occurred = &parsed
				break
			}
		}
	}
	confidence := args.Confidence
	if confidence <= 0 {
		confidence = 0.9
	}
	attrs := args.Attrs
	if string(attrs) == "null" {
		attrs = nil
	}
	// 东西挪了地方是日常，我明确说了的直接替代：旧位置留在替代链里，照片和用途沿用旧记忆的。
	if args.Type == "item" && args.Supersedes != "" && confidence >= 0.8 {
		if old, err := s.memory.Get(ctx, scope.familyKey, args.Supersedes); err == nil && old.Status == memory.StatusActive {
			moved, err := s.memory.Supersede(ctx, scope.familyKey, old.MemoryKey, memory.Input{
				Type: args.Type, SubjectPerson: subject, Persons: persons, PlaceKey: args.Place,
				Title: args.Title, Content: args.Content, Attrs: memory.MergeAttrs(old.Attrs, attrs), Importance: max(args.Importance, old.Importance),
				Confidence: confidence, Status: memory.StatusActive, OccurredAt: occurred, CreatedBy: "chat",
			})
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("已更新位置：%s 替代了 %s（旧位置保留在历史里）。", moved.MemoryKey, old.MemoryKey), nil
		}
	}
	// 推断出来的、或要替代旧记忆的，先进待确认，由我拍板。
	status := memory.StatusActive
	if confidence < 0.8 || args.Supersedes != "" {
		status = memory.StatusPending
	}
	created, err := s.memory.Create(ctx, scope.familyKey, memory.Input{
		Type: args.Type, SubjectPerson: subject, Persons: persons, PlaceKey: args.Place,
		Title: args.Title, Content: args.Content, Attrs: attrs, Importance: args.Importance,
		Confidence: confidence, Status: status, OccurredAt: occurred, CreatedBy: "chat",
		SupersedesKey: args.Supersedes,
	})
	if err != nil {
		return "", err
	}
	if status == memory.StatusPending {
		return fmt.Sprintf("已记下 %s，状态：待确认（在 App「今天」页确认后生效）。", created.MemoryKey), nil
	}
	return fmt.Sprintf("已记下 %s。", created.MemoryKey), nil
}

func runAskPersona(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Persona string `json:"persona"`
		Message string `json:"message"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	if strings.TrimSpace(args.Message) == "" {
		return "", errors.New("message 不能为空")
	}
	// 工具列表可以在管理端修改，所以不能只依赖“默认没给分身这个工具”。
	// 服务端再兜底一次，防止分身通过另一个分身绕过记忆可见范围。
	if scope.bot.Kind != bot.KindAssistant {
		return "", errors.New("只有助理可以调用分身预演")
	}
	if args.Persona != "wife" && args.Persona != "mom" {
		return "", errors.New("只能请老婆或妈妈的分身预演")
	}
	target, err := s.bots.Get(ctx, scope.familyKey, args.Persona)
	if err != nil || target.Kind != bot.KindPersona || !target.Enabled {
		return "", fmt.Errorf("没有叫 %s 的分身", args.Persona)
	}
	system, _, err := s.buildSystem(ctx, scope.familyKey, target)
	if err != nil {
		return "", err
	}
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:   target.RouteTask,
		System: []llm.SystemPart{{Text: system}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "我打算这么跟你说：" + args.Message +
			"\n\n请以你的身份给出听到这句话的真实第一反应，然后用括号跳出角色，用一两句话说哪里让你舒服、哪里让你不舒服、怎么说会更好。"}}}},
		RefKey: scope.conversationKey,
	}, nil)
	if err != nil {
		return "", err
	}
	if response.StopReason == llm.StopRefusal {
		return "", errors.New("分身没有回答这次预演")
	}
	return fmt.Sprintf("【%s 的预演结果（模拟）】\n%s", target.Name, response.Message.Text()), nil
}

func runListUpcoming(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	if scope.bot.Kind != bot.KindAssistant {
		return "", errors.New("只有助理可以查看全家提醒")
	}
	var args struct {
		Days int `json:"days"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	items, err := s.reminders.Upcoming(ctx, scope.familyKey, args.Days)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "近期没有提醒。", nil
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("- [%s] %s：%s", item.RemindAt.In(time.Local).Format("2006-01-02 15:04"), item.Title, item.Body))
	}
	return strings.Join(lines, "\n"), nil
}

func runCreateReminder(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	if scope.bot.Kind != bot.KindAssistant {
		return "", errors.New("只有助理可以建提醒")
	}
	var args struct {
		Title    string `json:"title"`
		Body     string `json:"body"`
		RemindAt string `json:"remind_at"`
		EventAt  string `json:"event_at"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	parse := func(value string) (*time.Time, error) {
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		for _, layout := range []string{"2006-01-02 15:04", time.RFC3339, "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
				return &parsed, nil
			}
		}
		return nil, fmt.Errorf("无法识别时间 %q", value)
	}
	remindAt, err := parse(args.RemindAt)
	if err != nil {
		return "", err
	}
	if remindAt == nil {
		return "", errors.New("remind_at 不能为空")
	}
	eventAt, err := parse(args.EventAt)
	if err != nil {
		return "", err
	}
	created, err := s.reminders.CreateManual(ctx, scope.familyKey, reminder.ManualInput{
		Title: args.Title, Body: args.Body, RemindAt: *remindAt, EventAt: eventAt, TargetURL: "/today",
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已建立提醒 %s，将在 %s 提醒。", created.ReminderKey, created.RemindAt.Format("2006-01-02 15:04")), nil
}

func runLoadSkill(ctx context.Context, s *Service, scope toolScope, input json.RawMessage) (string, error) {
	var args struct {
		Name string `json:"name"`
	}
	if err := decodeInput(input, &args); err != nil {
		return "", err
	}
	entry, err := s.skills.Get(ctx, scope.familyKey, strings.TrimSpace(args.Name))
	if err != nil || !entry.AvailableTo(scope.bot.BotKey) {
		return "", fmt.Errorf("没有叫 %s 的手册，看系统提示词里「做事手册」的目录", args.Name)
	}
	return s.skills.Render(ctx, scope.familyKey, entry), nil
}
