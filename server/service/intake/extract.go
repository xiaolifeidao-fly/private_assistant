package intake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"service/family"
	"service/llm"
	"service/memory"
)

// 抽取分两步（设计文档 3.4）：
//  1. Extract：从原始记录里抽出候选记忆（结构化输出，保证 JSON 合法）。
//  2. Reconcile：每条候选和检索到的相似旧记忆一起交给模型，决定新增 / 补充 / 替代 / 重复。
//     没有相似旧记忆的候选直接新增，不花这一次调用。
const taskExtract = "memory.extract"

// candidate 是模型抽出的一条候选记忆。所有字段都必填（结构化输出的要求），没有的填空串。
type candidate struct {
	Type          string   `json:"type"`
	SubjectPerson string   `json:"subject_person"`
	Persons       []string `json:"persons"`
	Place         string   `json:"place"`
	Title         string   `json:"title"`
	Content       string   `json:"content"`
	Evidence      string   `json:"evidence"`
	Importance    int      `json:"importance"`
	Confidence    float64  `json:"confidence"`
	OccurredAt    string   `json:"occurred_at"`
	Speaker       string   `json:"speaker"`
	PromisedTo    string   `json:"promised_to"`
	DueAt         string   `json:"due_at"`
	Polarity      string   `json:"polarity"`
	Location      string   `json:"location"`
	Usage         string   `json:"usage"`
	Photos        []int    `json:"photos"`
}

type extraction struct {
	Summary        string      `json:"summary"`
	UnknownPersons []string    `json:"unknown_persons"`
	Items          []candidate `json:"items"`
}

type decision struct {
	Index   int    `json:"index"`
	Action  string `json:"action"` // add / update / supersede / duplicate
	Target  string `json:"target"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Reason  string `json:"reason"`
}

func stringField(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func extractionSchema() map[string]any {
	types := make([]string, 0, len(memory.Types))
	for _, info := range memory.Types {
		if info.Type != "insight" {
			types = append(types, info.Type)
		}
	}
	item := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"type":           map[string]any{"type": "string", "enum": types},
			"subject_person": stringField("主要关于谁：人物 key；认不出或全家通用填空串"),
			"persons":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "还涉及的人物 key"},
			"place":          stringField("相关地点 key，没有填空串"),
			"title":          stringField("一句话摘要，40 字以内"),
			"content":        stringField("完整内容；原话类必须保留原文"),
			"evidence":       stringField("依据：原文中对应的片段"),
			"importance":     map[string]any{"type": "integer", "description": "1-5"},
			"confidence":     map[string]any{"type": "number", "description": "0-1，原文明说=0.9 以上，推断=0.7 以下"},
			"occurred_at":    stringField("发生时间 YYYY-MM-DD 或 YYYY-MM-DD HH:MM；不知道填空串"),
			"speaker":        stringField("原话的说话人 key，非原话填空串"),
			"promised_to":    stringField("承诺对象 key，非承诺填空串"),
			"due_at":         stringField("承诺/诉求的期限 YYYY-MM-DD，没有填空串"),
			"polarity":       map[string]any{"type": "string", "enum": []string{"", "like", "dislike"}},
			"location":       stringField("物品的存放位置，从大到小写全：如「家里 / 主卧 / 衣柜左边第二层」；非物品填空串"),
			"usage":          stringField("物品是干什么用的；非物品填空串"),
			"photos":         map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "这条记忆对应的照片序号（从 0 开始，按图片顺序）；没有填空数组"},
		},
		"required": []string{"type", "subject_person", "persons", "place", "title", "content", "evidence", "importance",
			"confidence", "occurred_at", "speaker", "promised_to", "due_at", "polarity", "location", "usage", "photos"},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"summary":         stringField("这条记录讲了什么，一句话"),
			"unknown_persons": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "提到了但家庭成员里没有的人，写原文里的叫法"},
			"items":           map[string]any{"type": "array", "items": item},
		},
		"required": []string{"summary", "unknown_persons", "items"},
	}
}

func reconcileSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"decisions": map[string]any{"type": "array", "items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"index":   map[string]any{"type": "integer"},
					"action":  map[string]any{"type": "string", "enum": []string{"add", "update", "supersede", "duplicate"}},
					"target":  stringField("update/supersede/duplicate 时填旧记忆编号，add 填空串"),
					"title":   stringField("update 时填合并后的摘要，其余填空串"),
					"content": stringField("update 时填合并后的完整内容，其余填空串"),
					"reason":  stringField("一句话理由"),
				},
				"required": []string{"index", "action", "target", "title", "content", "reason"},
			}},
		},
		"required": []string{"decisions"},
	}
}

func (s *Service) extractSystem(ctx context.Context, familyKey string) (string, map[string]family.Person, error) {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return "", nil, err
	}
	places, err := s.family.ListPlaces(ctx, familyKey)
	if err != nil {
		return "", nil, err
	}
	byKey := map[string]family.Person{}
	var builder strings.Builder
	builder.WriteString(`你是家庭记忆整理员。下面会给你一段我（这个家里的丈夫/爸爸）记下的原始内容：可能是随手记的话、粘贴的微信聊天、聊天截图。
你的任务是从中抽出值得长期记住的信息，供以后提醒我、帮我处理家庭关系用。

要抽的：家人说过的重要的话（尤其是老婆的不满、期待、在意的事）、诉求、我答应的事、发生的事件和各方反应、性格习惯喜好的表现或变化、雷区、老家的规矩和人情往来、孩子的饮食作息健康成长。
不要抽的：寒暄、无关的闲聊、纯事务性的沟通（"到了吗""好的"），以及只在当下有意义、过几天就没用的信息。

规则：
- 只根据原文，不要编造。推断出来的内容 confidence 给 0.7 以下。
- 原话类（quote）的 content 必须保留原文，不要改写；title 用一句话概括她在说什么。
- 人用下面的 key 表示。聊天截图里的昵称、备注名要结合「这条记录关于谁」和上下文判断是谁；我自己的消息通常在右侧/绿色气泡。认不出的人放进 unknown_persons，相关记忆的 subject_person 填空串，并在 content 里写清原文的叫法。
- 把「昨天」「下周三」「月底」换算成具体日期，以记录的发生时间为准。
- 同一件事不要拆成多条重复记忆；一段吵架的经过记一条 event，其中最重要的一两句原话再各记一条 quote。
- importance：雷区、强烈的不满、我的承诺、健康问题一般 4-5；日常喜好 2-3。
- 什么都不值得记就返回空的 items。

照片和物品：
- 来源是「随手录」时，多半是我拍了一样东西、口述它是什么、放在哪，用来以后找东西。这时记成 item（物品），一样东西一条。
- item 的 title 写成「东西 → 放在哪」，如「护照和户口本 → 书房书桌右边抽屉」；content 把照片里看到的写详细：名称、品牌型号、颜色、外观、数量、上面的字、包装，再加上我说的用途和位置。以后我会用各种叫法来找（「那个蓝色的盒子」「充电器」），描述越具体越好找。
- location 从大到小写全；照片里能看出位置（柜子、房间）就结合我说的一起写，看不出也没说就填「未说明」，并把 confidence 给 0.7 以下。
- photos 填这样东西出现在哪几张照片里；一张照片里有好几样东西可以各记一条、都引用这张照片。
- 照片不是物品而是事情（票据、聊天截图、家人的合影、孩子的作品），按内容记成对应类型，photos 同样填上，以后能把原图翻出来。
- 物品默认 subject_person 填空串（全家的东西）；明确是某人的（女儿的水杯、妈妈的药）才填那个人。

`)
	builder.WriteString("## 家庭成员\n")
	for _, person := range persons {
		byKey[person.PersonKey] = person
		line := fmt.Sprintf("- key=%s %s", person.PersonKey, person.RoleLabel)
		if person.Name != "" && person.Name != person.RoleLabel {
			line += "（" + person.Name + "）"
		}
		if len(person.Aliases) > 0 {
			line += "，也叫：" + strings.Join(person.Aliases, "、")
		}
		builder.WriteString(line + "\n")
	}
	builder.WriteString("\n## 地点\n")
	for _, place := range places {
		builder.WriteString(fmt.Sprintf("- key=%s %s\n", place.PlaceKey, place.Name))
	}
	builder.WriteString("\n## 记忆类型\n")
	for _, info := range memory.Types {
		if info.Type != "insight" {
			builder.WriteString(fmt.Sprintf("- %s：%s。%s\n", info.Type, info.Label, info.Hint))
		}
	}
	return builder.String(), byKey, nil
}

func noteHeader(row noteRow, persons map[string]family.Person) string {
	var builder strings.Builder
	builder.WriteString("## 这条记录\n")
	builder.WriteString("录入时间：" + row.CreatedAt.Format("2006-01-02 15:04") + "\n")
	if row.OccurredAt != nil {
		builder.WriteString("发生时间：" + row.OccurredAt.Format("2006-01-02 15:04") + "\n")
	}
	if len(row.AboutPersons) > 0 {
		var names []string
		for _, key := range row.AboutPersons {
			if person, ok := persons[key]; ok {
				names = append(names, fmt.Sprintf("%s(key=%s)", person.RoleLabel, key))
			}
		}
		if len(names) > 0 {
			builder.WriteString("关于谁：" + strings.Join(names, "、") + "\n")
		}
	}
	builder.WriteString("来源：" + channelLabel(row.Channel) + "\n")
	if len(row.attachments()) > 0 {
		kind := "截图"
		if row.Channel == "capture" {
			kind = "照片"
		}
		builder.WriteString(fmt.Sprintf("附了 %d 张%s（见上方图片，按顺序，序号从 0 开始）。\n", len(row.attachments()), kind))
	}
	if row.RawText != "" {
		builder.WriteString("\n内容：\n" + row.RawText + "\n")
	}
	return builder.String()
}

func channelLabel(channel string) string {
	switch channel {
	case "quick_note":
		return "速记"
	case "paste":
		return "粘贴的文字/聊天记录"
	case "screenshot":
		return "聊天截图"
	case "manager":
		return "管理端录入"
	case "chat":
		return "和助理的对话（只抽取我陈述的事实）"
	case "capture":
		return "随手录（照片 + 我口述/打字的说明，常用来记东西放在哪）"
	case "import":
		return "批量导入的聊天记录"
	}
	return channel
}

// process 对一条原始记录跑完整个抽取流程，返回结果摘要。
func (s *Service) process(ctx context.Context, row noteRow) (Result, error) {
	system, persons, err := s.extractSystem(ctx, row.FamilyKey)
	if err != nil {
		return Result{}, err
	}
	images, err := s.loadImages(row)
	if err != nil {
		return Result{}, err
	}
	blocks := append(images, llm.Block{Type: llm.BlockText, Text: noteHeader(row, persons)})
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskExtract,
		System:     []llm.SystemPart{{Text: system, Cache: true}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: blocks}},
		JSONSchema: extractionSchema(),
		RefKey:     row.NoteKey,
	}, nil)
	if err != nil {
		return Result{}, err
	}
	if response.StopReason == llm.StopRefusal {
		return Result{}, errors.New("模型拒绝处理这条内容")
	}
	if response.StopReason == llm.StopMaxToken {
		return Result{}, errors.New("内容太多，输出被截断了；请拆成几条分别提交，或在管理端调大 memory.extract 的 max_tokens")
	}
	var extracted extraction
	if err := json.Unmarshal([]byte(extractJSON(response.Message.Text())), &extracted); err != nil {
		return Result{}, fmt.Errorf("解析抽取结果失败: %w", err)
	}

	result := Result{Summary: extracted.Summary, UnknownPersons: nonEmpty(extracted.UnknownPersons)}
	items := sanitize(extracted.Items, persons, row)
	if len(items) == 0 {
		return result, nil
	}
	decisions, err := s.reconcile(ctx, row, items)
	if err != nil {
		return result, err
	}
	for index, item := range items {
		if err := s.apply(ctx, row, item, decisions[index], &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// sanitize 校验并清洗模型输出：认不出的 key 置空、数值钳到合法范围、日期解析。
func sanitize(items []candidate, persons map[string]family.Person, row noteRow) []candidate {
	known := func(key string) string {
		if _, ok := persons[key]; ok {
			return key
		}
		return ""
	}
	var result []candidate
	for _, item := range items {
		if strings.TrimSpace(item.Content) == "" && strings.TrimSpace(item.Title) == "" {
			continue
		}
		item.SubjectPerson = known(item.SubjectPerson)
		item.Speaker = known(item.Speaker)
		item.PromisedTo = known(item.PromisedTo)
		var others []string
		for _, key := range item.Persons {
			if known(key) != "" && key != item.SubjectPerson {
				others = append(others, key)
			}
		}
		item.Persons = others
		if item.Importance < 1 || item.Importance > 5 {
			item.Importance = 3
		}
		if item.Confidence <= 0 || item.Confidence > 1 {
			item.Confidence = 0.7
		}
		imageCount := len(row.attachments())
		var photos []int
		for _, index := range item.Photos {
			if index >= 0 && index < imageCount && !slices.Contains(photos, index) {
				photos = append(photos, index)
			}
		}
		item.Photos = photos
		if item.Type == "item" && imageCount == 1 && len(photos) == 0 {
			item.Photos = []int{0}
		}
		if item.OccurredAt == "" && row.OccurredAt != nil {
			item.OccurredAt = row.OccurredAt.Format("2006-01-02 15:04")
		}
		result = append(result, item)
	}
	return result
}

// reconcile 给每条候选决定动作。没有相似旧记忆的直接 add。
func (s *Service) reconcile(ctx context.Context, row noteRow, items []candidate) ([]decision, error) {
	decisions := make([]decision, len(items))
	var builder strings.Builder
	needModel := false
	for index, item := range items {
		decisions[index] = decision{Index: index, Action: "add"}
		query := memory.SearchQuery{Text: item.Title + "\n" + item.Content, Limit: 5}
		if item.SubjectPerson != "" {
			query.Persons = []string{item.SubjectPerson}
		}
		hits, err := s.memory.Search(ctx, row.FamilyKey, query)
		if err != nil {
			return nil, err
		}
		var similar []memory.Memory
		for _, hit := range hits {
			// 只把真正可能相关的拿去比：同一个人或同一类
			if hit.Memory.Type == item.Type || (item.SubjectPerson != "" && hit.Memory.SubjectPerson == item.SubjectPerson) {
				similar = append(similar, hit.Memory)
			}
		}
		if len(similar) == 0 {
			continue
		}
		needModel = true
		fmt.Fprintf(&builder, "### 候选 %d\n[%s] %s —— %s\n已有的相似记忆：\n", index, item.Type, item.Title, oneLine(item.Content, 300))
		for _, old := range similar {
			fmt.Fprintf(&builder, "- %s [%s | %s] %s —— %s\n", old.MemoryKey, old.Type, dateOf(old), old.Title, oneLine(old.Content, 200))
		}
		builder.WriteString("\n")
	}
	if !needModel {
		return decisions, nil
	}
	response, err := s.llm.Chat(ctx, llm.Request{
		Task: taskExtract,
		System: []llm.SystemPart{{Text: `你在整理家庭记忆库。每条候选记忆都附有几条已有的相似记忆，请判断候选该怎么处理：
- add：是新信息（即使主题相近，只要是另一件事、另一句话、另一个时间点，都算新信息）。
- duplicate：已有记忆已经包含了同样的信息，不用再记。target 填那条旧记忆。
- update：同一件事/同一个事实，候选补充了细节但没有改变含义。target 填旧记忆，title/content 填合并后的完整版本。
- supersede：同一个方面的情况变了（以前喜欢现在不喜欢、习惯改了、规矩变了、承诺的时间改了），新的应该替代旧的。target 填旧记忆。
原话（quote）和事件（event）每一次都是独立的，几乎总是 add 或 duplicate，不要 update 或 supersede。
物品（item）：同一样东西换了地方是 supersede；同一样东西、同一个位置、只是多了描述或照片是 update；不同的东西（哪怕放在一起）是 add。
没有列出相似记忆的候选不用输出。`}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: reconcileSchema(),
		RefKey:     row.NoteKey,
	}, nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Decisions []decision `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(extractJSON(response.Message.Text())), &parsed); err != nil {
		return nil, fmt.Errorf("解析比对结果失败: %w", err)
	}
	for _, item := range parsed.Decisions {
		if item.Index >= 0 && item.Index < len(decisions) {
			switch item.Action {
			case "add", "update", "supersede", "duplicate":
				decisions[item.Index] = item
			}
		}
	}
	return decisions, nil
}

func (s *Service) apply(ctx context.Context, row noteRow, item candidate, choice decision, result *Result) error {
	if choice.Action != "add" && choice.Target == "" {
		choice.Action = "add"
	}
	switch choice.Action {
	case "duplicate":
		if _, err := s.memory.Get(ctx, row.FamilyKey, choice.Target); err != nil {
			choice.Action = "add" // 模型给的编号不存在，按新增处理
			break
		}
		s.memory.MarkUsed(ctx, []string{choice.Target})
		result.Duplicates = append(result.Duplicates, choice.Target)
		return nil
	case "update":
		old, err := s.memory.Get(ctx, row.FamilyKey, choice.Target)
		if err != nil || old.Status != memory.StatusActive || strings.TrimSpace(choice.Content) == "" {
			choice.Action = "add"
			break
		}
		attrs := old.Attrs
		if old.Type == "item" {
			// 补拍的照片、补充的位置说明并进旧记忆
			attrs = memory.MergeAttrs(old.Attrs, attrsOf(item, row.NoteKey))
		}
		updated := memory.Input{
			Type: old.Type, Scope: old.Scope, SubjectPerson: old.SubjectPerson, PlaceKey: old.PlaceKey, Persons: old.Persons,
			Title: defaultString(choice.Title, old.Title), Content: choice.Content, Attrs: attrs, Importance: max(old.Importance, item.Importance),
			Confidence: old.Confidence, Status: old.Status, OccurredAt: old.OccurredAt, ValidFrom: old.ValidFrom, CreatedBy: old.CreatedBy,
			SourceNoteKey: old.SourceNoteKey,
		}
		if _, err := s.memory.Update(ctx, row.FamilyKey, old.MemoryKey, updated); err != nil {
			return err
		}
		result.Updated = append(result.Updated, old.MemoryKey)
		return nil
	case "supersede":
		if _, err := s.memory.Get(ctx, row.FamilyKey, choice.Target); err != nil {
			choice.Target = ""
		}
	}

	input := memory.Input{
		Type: item.Type, SubjectPerson: item.SubjectPerson, Persons: item.Persons, PlaceKey: item.Place,
		Title: item.Title, Content: item.Content, Attrs: attrsOf(item, row.NoteKey), Importance: item.Importance,
		Confidence: item.Confidence, OccurredAt: parseTime(item.OccurredAt), CreatedBy: "extract", SourceNoteKey: row.NoteKey,
	}
	if choice.Action == "supersede" && choice.Target != "" {
		input.SupersedesKey = choice.Target
	}
	// 「她变了」和没把握的都进待确认，由我拍板。东西挪了地方是日常，直接替代，旧位置留在替代链里。
	input.Status = memory.StatusActive
	if item.Type == "item" && input.SupersedesKey != "" && item.Confidence >= s.config.AutoConfirm {
		moved, err := s.memory.Supersede(ctx, row.FamilyKey, input.SupersedesKey, input)
		if err == nil {
			result.Updated = append(result.Updated, moved.MemoryKey)
			return nil
		}
		// 旧记忆已经被替代过等情况：按普通新增处理
	}
	if input.SupersedesKey != "" || item.Confidence < s.config.AutoConfirm {
		input.Status = memory.StatusPending
	}
	created, err := s.memory.Create(ctx, row.FamilyKey, input)
	if err != nil {
		// 单条写失败（比如地点 key 不对）不拖垮整条记录，降级成不带地点再试一次
		if input.PlaceKey == "" {
			return err
		}
		input.PlaceKey = ""
		if created, err = s.memory.Create(ctx, row.FamilyKey, input); err != nil {
			return err
		}
	}
	if created.Status == memory.StatusPending {
		result.Pending = append(result.Pending, created.MemoryKey)
	} else {
		result.Added = append(result.Added, created.MemoryKey)
	}
	return nil
}

func attrsOf(item candidate, noteKey string) json.RawMessage {
	attrs := map[string]string{}
	switch item.Type {
	case "quote":
		if item.Speaker != "" {
			attrs["speaker"] = item.Speaker
		}
	case "commitment":
		attrs["status"] = "open"
		if item.PromisedTo != "" {
			attrs["promisedTo"] = item.PromisedTo
		}
		if item.DueAt != "" {
			attrs["dueAt"] = item.DueAt
		}
	case "need":
		attrs["status"] = "open"
		if item.DueAt != "" {
			attrs["dueAt"] = item.DueAt
		}
	case "preference":
		if item.Polarity != "" {
			attrs["polarity"] = item.Polarity
		}
	case "item":
		if item.Location != "" {
			attrs["location"] = item.Location
		}
		if item.Usage != "" {
			attrs["usage"] = item.Usage
		}
	}
	if item.Evidence != "" && item.Evidence != item.Content {
		attrs["evidence"] = oneLine(item.Evidence, 500)
	}
	if len(item.Photos) == 0 && len(attrs) == 0 {
		return nil
	}
	full := map[string]any{}
	for key, value := range attrs {
		full[key] = value
	}
	if len(item.Photos) > 0 {
		refs := make([]memory.PhotoRef, 0, len(item.Photos))
		for _, index := range item.Photos {
			refs = append(refs, memory.PhotoRef{Note: noteKey, Index: index})
		}
		full["photos"] = refs
	}
	data, _ := json.Marshal(full)
	return data
}

func parseTime(value string) *time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02"} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return &parsed
		}
	}
	return nil
}

// extractJSON 容忍模型在 JSON 外面包了 ```json 代码块（OpenAI 兼容模型偶尔会这样）。
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

func nonEmpty(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func dateOf(item memory.Memory) string {
	if item.OccurredAt != nil {
		return item.OccurredAt.Format("2006-01-02")
	}
	return item.CreatedAt.Format("2006-01-02")
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
