package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"common/middleware/db"
	"common/viewer"
	"service/bot"
	"service/family"
	"service/memory"
	"service/setting"
)

// 上下文分两部分（设计文档 3.5）：
//   - 系统提示词：机器人提示词 + 家庭档案 + 常驻记忆。会话开始时生成一次并冻结，
//     之后整个会话原样复用 —— 命中提示词缓存，也不破坏 Claude 的 thinking 块。
//   - 每轮参考：当前时间 + 针对这一问检索到的记忆，作为这一轮用户消息的第一个文本块
//     追加进历史，之后同样不再改动。

type directory struct {
	persons map[string]family.Person
	places  map[string]family.Place
	ordered []family.Person
}

func (s *Service) loadDirectory(ctx context.Context, familyKey string) (directory, error) {
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return directory{}, err
	}
	places, err := s.family.ListPlaces(ctx, familyKey)
	if err != nil {
		return directory{}, err
	}
	dir := directory{persons: map[string]family.Person{}, places: map[string]family.Place{}, ordered: persons}
	for _, person := range persons {
		dir.persons[person.PersonKey] = person
	}
	for _, place := range places {
		dir.places[place.PlaceKey] = place
	}
	return dir, nil
}

func (dir directory) personName(key string) string {
	if person, ok := dir.persons[key]; ok {
		if person.Name != person.RoleLabel && person.Name != "" {
			return fmt.Sprintf("%s(%s)", person.RoleLabel, person.Name)
		}
		return person.RoleLabel
	}
	return key
}

// mentioned 找出文本里提到的人（按名字、身份、别名匹配）。
func (dir directory) mentioned(text string) []string {
	var result []string
	for _, person := range dir.ordered {
		names := append([]string{person.Name, person.RoleLabel}, person.Aliases...)
		for _, name := range names {
			if len([]rune(name)) >= 1 && name != "我" && strings.Contains(text, name) {
				result = append(result, person.PersonKey)
				break
			}
		}
	}
	return result
}

// buildSystem 生成冻结的系统提示词，同时返回里面放了哪些人的哪一版人设卡。
func (s *Service) buildSystem(ctx context.Context, familyKey string, item bot.Bot) (string, map[string]int, error) {
	versions := map[string]int{}
	dir, err := s.loadDirectory(ctx, familyKey)
	if err != nil {
		return "", nil, err
	}
	var builder strings.Builder
	prompt := item.SystemPrompt
	if person, ok := dir.persons[item.PersonKey]; ok {
		prompt = strings.NewReplacer("{{person_name}}", person.Name, "{{person_role}}", person.RoleLabel).Replace(prompt)
	}
	builder.WriteString(prompt)

	// 做事手册只放目录，正文由 load_skill 读进历史（设计文档 05 · 1）
	if slices.Contains(item.Tools, bot.ToolLoadSkill) {
		skills, err := s.skills.ForBot(ctx, familyKey, item.BotKey)
		if err != nil {
			return "", nil, err
		}
		if len(skills) > 0 {
			builder.WriteString("\n\n## 做事手册\n遇到下面的场景，先用 load_skill 读对应手册再回答（参数填 key）。一个问题可能涉及几本，按需都读。\n")
			for _, entry := range skills {
				fmt.Fprintf(&builder, "- %s：%s —— %s\n", entry.SkillKey, entry.Name, entry.WhenToUse)
			}
		}
	}

	builder.WriteString("\n\n## 家庭成员\n")
	builder.WriteString("（key 是工具参数里用的标识）\n")
	for _, person := range dir.ordered {
		line := fmt.Sprintf("- key=%s %s", person.PersonKey, dir.personName(person.PersonKey))
		if len(person.Aliases) > 0 {
			line += "，也叫：" + strings.Join(person.Aliases, "、")
		}
		if birthday := birthdayText(person); birthday != "" {
			line += "，生日 " + birthday
		}
		if place, ok := dir.places[person.HomePlaceKey]; ok {
			line += "，住在" + place.Name
		}
		if person.Profile != "" {
			line += "。" + oneLine(person.Profile, 120)
		}
		builder.WriteString(line + "\n")
	}

	builder.WriteString("\n## 几个家\n")
	for _, place := range sortedPlaces(dir.places) {
		line := fmt.Sprintf("- key=%s %s", place.PlaceKey, place.Name)
		if place.Address != "" {
			line += "（" + place.Address + "）"
		}
		if place.Customs != "" {
			line += "。规矩：" + oneLine(place.Customs, 200)
		}
		builder.WriteString(line + "\n")
	}

	relationOwner := "me"
	if item.MemoryScope == bot.ScopePerson && item.PersonKey != "" {
		relationOwner = item.PersonKey
	}
	relations, err := s.family.ListRelations(ctx, familyKey, relationOwner)
	if err != nil {
		return "", nil, err
	}
	// 关系要点、人设卡是从我的记录里整理出来的，老婆、奶奶的账号不放进提示词
	private := !viewer.IsSystem(ctx)
	if len(relations) > 0 && !private {
		builder.WriteString("\n## 关系要点\n")
		for _, relation := range relations {
			builder.WriteString("- " + formatRelation(dir, relation) + "\n")
		}
	}

	// 人设卡：分身只看她自己的；助理看所有核心成员的
	for _, personKey := range cardPersons(dir, item) {
		if private {
			break
		}
		person := dir.persons[personKey]
		versions[personKey] = 0
		card, ok, err := s.personas.Active(ctx, familyKey, person.PersonKey)
		if err != nil || !ok {
			continue
		}
		versions[personKey] = card.Version
		fmt.Fprintf(&builder, "\n## %s的人设卡（v%d，%s 更新，基于 %d 条记忆）\n%s\n",
			dir.personName(person.PersonKey), card.Version, card.CreatedAt.Format("2006-01-02"), card.MemoryCount, card.Content)
	}

	filter := memory.CoreFilter{Limit: 40}
	if item.MemoryScope == bot.ScopePerson && item.PersonKey != "" {
		filter.Persons = []string{item.PersonKey}
		filter.VisibleTo = item.PersonKey
		filter.MinImportance = 3
	}
	core, err := s.memory.Core(ctx, familyKey, filter)
	if err != nil {
		return "", nil, err
	}
	builder.WriteString("\n## 常驻记忆\n")
	if len(core) == 0 {
		builder.WriteString("（还没有记忆。）\n")
	}
	for _, item := range core {
		builder.WriteString(formatMemory(dir, item) + "\n")
	}
	builder.WriteString("\n记忆的格式是 [编号 | 类型 | 关于谁 | 时间 | 重要度]。每一轮我的消息前面会附一段 <turn_context>，" +
		"是系统按我的问题自动检索的参考，不是我写的话；它可能不全，需要时用工具再查。" +
		"人设卡在对话中途更新时，<turn_context> 里会附 <persona_update>，以它为准。\n")
	return builder.String(), versions, nil
}

// cardPersons 是这个机器人会看到人设卡的人：分身只看她自己；助理看所有核心成员（我除外）。
func cardPersons(dir directory, item bot.Bot) []string {
	var result []string
	for _, person := range dir.ordered {
		if !person.IsCore || person.PersonKey == "me" {
			continue
		}
		if item.MemoryScope == bot.ScopePerson && person.PersonKey != item.PersonKey {
			continue
		}
		result = append(result, person.PersonKey)
	}
	return result
}

// personaUpdates 对比会话已知的人设卡版本和当前版本，生成 <persona_update> 段落。
// 已知版本存在 pa_conversation.persona_versions：它不进提示词，提示过一次就更新，避免每轮重复。
// 分身拿到新卡全文（她要靠它说话）；助理只拿变化说明，需要全文自己用 get_person 查。
func (s *Service) personaUpdates(ctx context.Context, familyKey string, item bot.Bot, conversation *conversationRow) (string, []personaUpdate) {
	if !viewer.IsSystem(ctx) {
		return "", nil
	}
	known := map[string]int{}
	if len(conversation.PersonaVersions) > 0 {
		_ = json.Unmarshal(conversation.PersonaVersions, &known)
	}
	dir, err := s.loadDirectory(ctx, familyKey)
	if err != nil {
		return "", nil
	}
	// 加这个字段之前就开始的会话：不知道冻结时是哪一版，就以现在为准，不提示
	baseline := len(conversation.PersonaVersions) == 0
	var builder strings.Builder
	var changed []personaUpdate
	for _, personKey := range cardPersons(dir, item) {
		card, ok, err := s.personas.Active(ctx, familyKey, personKey)
		if err != nil || !ok || card.Version <= known[personKey] {
			continue
		}
		if baseline {
			known[personKey] = card.Version
			continue
		}
		from := "无"
		if known[personKey] > 0 {
			from = fmt.Sprintf("v%d", known[personKey])
		}
		fmt.Fprintf(&builder, "<persona_update>\n%s的人设卡在这段对话期间更新了（%s → v%d，%s）。\n变化：%s\n",
			dir.personName(personKey), from, card.Version, card.CreatedAt.Format("01-02 15:04"), defaultText(card.Changes, "（没有写变化说明）"))
		if item.MemoryScope == bot.ScopePerson {
			builder.WriteString("最新人设卡：\n" + card.Content + "\n")
		}
		builder.WriteString("</persona_update>\n")
		changed = append(changed, personaUpdate{Person: personKey, From: known[personKey], To: card.Version, Changes: card.Changes})
		known[personKey] = card.Version
	}
	if len(changed) == 0 && !baseline {
		return "", nil
	}
	if err := s.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", conversation.ID).
		Update("persona_versions", db.MustJSON(known)).Error; err == nil {
		conversation.PersonaVersions = db.MustJSON(known)
	}
	return builder.String(), changed
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// turnInput 是生成每轮参考需要的信息。
type turnInput struct {
	text     string
	snapshot string   // 冻结的系统提示词：里面已有的常驻记忆不再重复
	signals  []string // 危机信号
	budget   setting.Budget
	extra    string   // 其他要附在参考里的段落（人设卡更新）
	history  []string // 这一轮之前最近几句对话（「我：…」「助理：…」），追问时用来理解指代
}

// buildTurnContext 生成这一轮的参考块，返回引用到的记忆编号和检索记录。
func (s *Service) buildTurnContext(ctx context.Context, familyKey string, item bot.Bot, input turnInput) (string, []string, retrievalMeta) {
	now := time.Now()
	var meta retrievalMeta
	var builder strings.Builder
	builder.WriteString("<turn_context>\n")
	builder.WriteString("当前时间：" + formatNow(now) + "\n")
	if len(input.signals) > 0 {
		builder.WriteString(safetyContext(input.signals))
	}
	builder.WriteString(input.extra)
	if isSmallTalk(input.text, input.budget) {
		meta.Skipped = true
		builder.WriteString("</turn_context>")
		return builder.String(), nil, meta
	}

	dir, err := s.loadDirectory(ctx, familyKey)
	if err != nil {
		builder.WriteString("</turn_context>")
		return builder.String(), nil, meta
	}
	found, err := s.retrieve(ctx, familyKey, item, dir, input.text, input.history, input.budget.TurnContextHits)
	hits := found.hits
	inSnapshot := map[string]bool{}
	for _, key := range memoryKeys(input.snapshot) {
		inSnapshot[key] = true
	}
	var used []string
	var lines []string
	size := 0
	if err == nil {
		for _, hit := range hits {
			meta.Hits++
			if inSnapshot[hit.Memory.MemoryKey] {
				meta.Deduped++
				continue
			}
			line := formatMemory(dir, hit.Memory)
			if size+len([]rune(line)) > input.budget.TurnContextChars {
				meta.Clipped++
				continue
			}
			size += len([]rune(line))
			lines = append(lines, line)
			used = append(used, hit.Memory.MemoryKey)
		}
	}
	var passageLines []string
	for _, passage := range found.passages {
		passageLines = append(passageLines, fmt.Sprintf("[%s | %s | %s] %s", passage.NoteKey, memory.ChannelText(passage.Channel),
			passage.OccurredAt.Format("2006-01-02"), oneLine(passage.Text, 300)))
	}
	if len(passageLines) > 0 {
		builder.WriteString("原始记录里可能相关的原文（整理成记忆时可能漏掉了细节；引用时写 [note_xxx]）：\n" + strings.Join(passageLines, "\n") + "\n")
	}
	switch {
	case len(lines) > 0:
		builder.WriteString("可能相关的记忆：\n" + strings.Join(lines, "\n") + "\n")
	case meta.Deduped > 0:
		builder.WriteString("相关的记忆都已在常驻记忆里。\n")
	default:
		builder.WriteString("没有检索到直接相关的记忆。\n")
	}
	builder.WriteString("</turn_context>")
	return builder.String(), used, meta
}

func formatMemory(dir directory, item memory.Memory) string {
	about := "全家"
	if item.SubjectPerson != "" {
		about = dir.personName(item.SubjectPerson)
	} else if item.PlaceKey != "" {
		if place, ok := dir.places[item.PlaceKey]; ok {
			about = place.Name
		}
	}
	when := item.CreatedAt
	if item.OccurredAt != nil {
		when = *item.OccurredAt
	}
	line := fmt.Sprintf("[%s | %s | %s | %s | 重要度%d] %s", item.MemoryKey, item.TypeLabel, about, when.Format("2006-01-02"), item.Importance, item.Title)
	if content := oneLine(item.Content, 300); content != "" && content != item.Title {
		line += " —— " + content
	}
	if attrs := attrsWithoutPhotos(item.Attrs); attrs != "" {
		line += " " + attrs
	}
	if photos := len(item.Photos()); photos > 0 {
		line += fmt.Sprintf("（有 %d 张照片，引用这条记忆时手机上会显示原图）", photos)
	}
	switch item.Status {
	case memory.StatusSuperseded:
		if item.ValidTo != nil {
			line += fmt.Sprintf("（已过时：%s 起不再成立）", item.ValidTo.Format("2006-01-02"))
		} else {
			line += "（已过时）"
		}
	case memory.StatusPending:
		line += "（待我确认）"
	}
	return line
}

func formatRelation(dir directory, relation family.Relation) string {
	line := fmt.Sprintf("%s 是 %s 的%s", dir.personName(relation.ToPerson), dir.personName(relation.FromPerson), relation.Relation)
	if relation.AddressAs != "" {
		line += "，称呼「" + relation.AddressAs + "」"
	}
	if relation.ChildAddressAs != "" {
		line += "，女儿叫「" + relation.ChildAddressAs + "」"
	}
	line += fmt.Sprintf("，亲近度%d", relation.Closeness)
	if relation.Notes != "" {
		line += "。要点：" + oneLine(relation.Notes, 200)
	}
	if relation.Sensitivities != "" {
		line += "。雷区：" + oneLine(relation.Sensitivities, 200)
	}
	return line
}

func birthdayText(person family.Person) string {
	switch {
	case person.BirthdayBy == "lunar" && person.LunarBirthday != "":
		return "农历 " + person.LunarBirthday
	case person.Birthday != "":
		return person.Birthday
	case person.LunarBirthday != "":
		return "农历 " + person.LunarBirthday
	}
	return ""
}

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

func formatNow(now time.Time) string {
	return now.Format("2006-01-02 15:04") + " " + weekdays[now.Weekday()]
}

func sortedPlaces(places map[string]family.Place) []family.Place {
	result := make([]family.Place, 0, len(places))
	for _, place := range places {
		result = append(result, place)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sort < result[j].Sort })
	return result
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
