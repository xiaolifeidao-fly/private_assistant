package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"service/family"
	"service/llm"
	"service/memory"
)

// 按家里的记忆改写手册（设计文档 06 · 0 #1）：出厂手册是按常识写的，
// 这里把和这本手册相关的真实记忆、家庭档案交给模型，让它把通用说法换成我们家的具体情况。
// 只给建议，不直接保存 —— 管理端看过、改过再存。

const taskRewrite = "skill.rewrite"

type MemoryPort interface {
	Search(ctx context.Context, familyKey string, query memory.SearchQuery) ([]memory.Hit, error)
	Core(ctx context.Context, familyKey string, filter memory.CoreFilter) ([]memory.Memory, error)
}

type FamilyPort interface {
	ListPersons(ctx context.Context, familyKey string) ([]family.Person, error)
	ListPlaces(ctx context.Context, familyKey string) ([]family.Place, error)
}

type LLMPort interface {
	Chat(ctx context.Context, request llm.Request, onDelta func(llm.Delta)) (llm.Response, error)
}

// Suggestion 是一次改写建议。
type Suggestion struct {
	Body     string   `json:"body"`
	Changes  []string `json:"changes"`  // 改了哪些地方、依据哪条记忆
	Gaps     []string `json:"gaps"`     // 记忆里缺、建议我补记的信息
	Memories int      `json:"memories"` // 参考了多少条记忆
}

const rewritePrompt = `你在帮我修订一本「做事手册」。手册告诉家庭助理遇到某类事该先查什么、怎么想、怎么说、别做什么。
现在的手册是按常识写的，不了解我们家。下面给你这本手册、我们家的档案和相关记忆。

请输出修订后的手册正文：
- 保留原来的结构（## 先查 / ## 怎么想 / ## 怎么说 / ## 别做 / ## 收尾，没有的节可以不加）和原有的工具名。
- 把通用的说法换成我们家的具体情况：具体的人、她们真正在意的点、两边老家真实的规矩、以前发生过的类似的事。每条来自记忆的具体内容，句末用括号标出记忆编号，如（mem_xxx）。
- 只用给你的记忆和档案，不要编；记忆里没有的，保留原来的通用写法。
- 立场不变：不站队、不评判谁对谁错。
- 篇幅和原来差不多，最多长一半。

changes 列出改了哪些地方、依据是什么（一条一句）。gaps 列出写这本手册时发现记忆里缺、值得我补记的信息（一条一句，没有就空）。`

func (s *Service) Suggest(ctx context.Context, familyKey, skillKey string) (Suggestion, error) {
	if s.memory == nil || s.llm == nil || s.family == nil {
		return Suggestion{}, errors.New("改写功能没有装配")
	}
	current, err := s.Get(ctx, familyKey, skillKey)
	if err != nil {
		return Suggestion{}, err
	}
	persons, err := s.family.ListPersons(ctx, familyKey)
	if err != nil {
		return Suggestion{}, err
	}
	places, err := s.family.ListPlaces(ctx, familyKey)
	if err != nil {
		return Suggestion{}, err
	}
	names := map[string]string{}
	var builder strings.Builder
	builder.WriteString("<family>\n")
	for _, person := range persons {
		names[person.PersonKey] = person.RoleLabel
		line := fmt.Sprintf("- %s（%s）", person.RoleLabel, person.Name)
		if person.Profile != "" {
			line += "：" + person.Profile
		}
		builder.WriteString(line + "\n")
	}
	for _, place := range places {
		line := "- 地点 " + place.Name
		if place.Customs != "" {
			line += "，规矩：" + place.Customs
		}
		if place.TravelTips != "" {
			line += "，去之前：" + place.TravelTips
		}
		builder.WriteString(line + "\n")
	}
	builder.WriteString("</family>\n\n<memories>\n")

	// 相关记忆：按手册的用途和正文检索，再补上全家的高重要度记忆
	seen := map[string]bool{}
	count := 0
	add := func(item memory.Memory) {
		if seen[item.MemoryKey] || count >= 80 {
			return
		}
		seen[item.MemoryKey] = true
		count++
		about := "全家"
		if item.SubjectPerson != "" {
			about = defaultName(names, item.SubjectPerson)
		}
		when := item.CreatedAt
		if item.OccurredAt != nil {
			when = *item.OccurredAt
		}
		fmt.Fprintf(&builder, "[%s | %s | %s | %s | 重要度%d] %s —— %s\n", item.MemoryKey, item.TypeLabel, about,
			when.Format("2006-01-02"), item.Importance, item.Title, oneLine(item.Content, 300))
	}
	for _, text := range []string{current.Name + "：" + current.WhenToUse, oneLine(current.Body, 600)} {
		hits, err := s.memory.Search(ctx, familyKey, memory.SearchQuery{Text: text, Limit: 30})
		if err != nil {
			return Suggestion{}, err
		}
		for _, hit := range hits {
			add(hit.Memory)
		}
	}
	core, err := s.memory.Core(ctx, familyKey, memory.CoreFilter{MinImportance: 4, Limit: 40})
	if err != nil {
		return Suggestion{}, err
	}
	for _, item := range core {
		add(item)
	}
	if count == 0 {
		return Suggestion{}, errors.New("还没有记忆，先多记一些家里的事再来改写")
	}
	builder.WriteString("</memories>\n\n")
	fmt.Fprintf(&builder, "<skill name=%q when=%q>\n%s\n</skill>", current.Name, current.WhenToUse, current.Body)

	response, err := s.llm.Chat(ctx, llm.Request{
		Task:       taskRewrite,
		System:     []llm.SystemPart{{Text: rewritePrompt}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: rewriteSchema(),
		RefKey:     skillKey,
	}, nil)
	if err != nil {
		return Suggestion{}, err
	}
	if response.StopReason == llm.StopMaxToken {
		return Suggestion{}, errors.New("输出被截断了，到「模型」里调大 skill.rewrite 的 max_tokens 再试")
	}
	text := response.Message.Text()
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	var result Suggestion
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return Suggestion{}, fmt.Errorf("解析改写结果失败: %w", err)
	}
	if strings.TrimSpace(result.Body) == "" {
		return Suggestion{}, errors.New("模型没有给出改写内容")
	}
	result.Memories = count
	return result, nil
}

func rewriteSchema() map[string]any {
	list := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{"body": map[string]any{"type": "string"}, "changes": list, "gaps": list},
		"required":             []string{"body", "changes", "gaps"},
		"additionalProperties": false,
	}
}

func defaultName(names map[string]string, key string) string {
	if name, ok := names[key]; ok {
		return name
	}
	return key
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
