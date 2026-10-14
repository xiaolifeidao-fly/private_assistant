package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"service/chat"
	"service/llm"
	"service/wire"
)

type evalCase struct {
	Name   string   `json:"name"`
	Bot    string   `json:"bot"`
	Turns  []string `json:"turns"`
	Expect expect   `json:"expect"`
	File   string   `json:"-"`
}

// expect 里的断言都是可选的。另外每个用例都自动检查：没有失败的消息、没有无出处的引用。
type expect struct {
	Tools          []string `json:"tools"`          // 必须调用过
	NoTools        []string `json:"noTools"`        // 不能调用
	Skills         []string `json:"skills"`         // 必须读过的手册
	Cites          []string `json:"cites"`          // 回答里必须引用的记忆（@ref）
	MustContainAny []string `json:"mustContainAny"` // 最后一个回答至少包含其中一个
	MustNotContain []string `json:"mustNotContain"` // 任何回答都不能包含
	Notice         *bool    `json:"notice"`         // 是否应该补危机提示卡
	Rubric         []string `json:"rubric"`         // 模型打分项
}

type rubricScore struct {
	Rubric string `json:"rubric"`
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

type result struct {
	Label      string
	Case       evalCase
	Transcript string
	Tools      []string
	Skills     []string
	Failures   []string
	Scores     []rubricScore
	Tokens     int
}

func (r result) passed() bool { return len(r.Failures) == 0 }

func loadCases(dir, filter string) ([]evalCase, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var cases []evalCase
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var items []evalCase
		if err := json.Unmarshal(content, &items); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, item := range items {
			if item.Bot == "" {
				item.Bot = "assistant"
			}
			item.File = filepath.Base(path)
			if filter == "" || strings.Contains(item.Name, filter) {
				cases = append(cases, item)
			}
		}
	}
	return cases, nil
}

type runner struct {
	services *wire.Services
	refs     map[string]string
	judge    bool
}

func (r *runner) run(ctx context.Context, item evalCase) result {
	outcome := result{Case: item}
	fail := func(format string, args ...any) { outcome.Failures = append(outcome.Failures, fmt.Sprintf(format, args...)) }

	owner := chat.Owner{FamilyKey: wire.DefaultFamily, AccountKey: "eval"}
	conversation, err := r.services.Chat.CreateConversation(ctx, owner, item.Bot)
	if err != nil {
		fail("建会话失败: %v", err)
		return outcome
	}
	for _, turn := range item.Turns {
		if err := r.services.Chat.Send(ctx, owner, conversation.ConversationKey, chat.SendInput{Text: turn}, func(chat.Event) {}); err != nil {
			fail("发送「%s」失败: %v", oneLine(turn, 30), err)
			return outcome
		}
	}
	messages, err := r.services.Chat.Messages(ctx, owner, conversation.ConversationKey)
	if err != nil {
		fail("读消息失败: %v", err)
		return outcome
	}

	var answers []string // 每轮最后一条有正文的回答
	var unknown []string
	noticed := false
	var transcript strings.Builder
	for index, message := range messages {
		switch message.Role {
		case llm.RoleUser:
			transcript.WriteString("【我】" + message.Content + "\n")
		case chat.RoleNotice:
			noticed = true
			transcript.WriteString("【系统提示卡】" + message.Content + "\n")
		case llm.RoleAssistant:
			if message.Status == "error" {
				fail("回答失败: %s", message.Error)
				continue
			}
			var blocks []llm.Block
			_ = json.Unmarshal(message.Blocks, &blocks)
			for _, block := range blocks {
				if block.Type != llm.BlockToolUse {
					continue
				}
				outcome.Tools = appendUnique(outcome.Tools, block.ToolName)
				transcript.WriteString(fmt.Sprintf("【助理调用 %s】%s\n", block.ToolName, oneLine(string(block.Input), 160)))
				if block.ToolName == "load_skill" {
					var input struct {
						Name string `json:"name"`
					}
					_ = json.Unmarshal(block.Input, &input)
					outcome.Skills = appendUnique(outcome.Skills, input.Name)
				}
			}
			var usage llm.Usage
			if json.Unmarshal(message.Usage, &usage) == nil {
				outcome.Tokens += usage.InputTokens + usage.CacheReadTokens + usage.CacheWriteTokens + usage.OutputTokens
			}
			var meta struct {
				Citations *struct {
					Unknown []string `json:"unknown"`
				} `json:"citations"`
			}
			if json.Unmarshal(message.Meta, &meta) == nil && meta.Citations != nil {
				unknown = append(unknown, meta.Citations.Unknown...)
			}
			if message.Content != "" {
				transcript.WriteString("【助理】" + message.Content + "\n")
				if index+1 >= len(messages) || messages[index+1].TurnKey != message.TurnKey || messages[index+1].Role == chat.RoleNotice {
					answers = append(answers, message.Content)
				}
			}
		}
	}
	outcome.Transcript = transcript.String()
	if len(answers) == 0 {
		fail("没有得到任何回答")
		return outcome
	}
	last := answers[len(answers)-1]
	all := strings.Join(answers, "\n")

	if len(unknown) > 0 {
		fail("引用了没有出处的记忆: %s", strings.Join(unknown, ", "))
	}
	for _, tool := range item.Expect.Tools {
		if !slices.Contains(outcome.Tools, tool) {
			fail("没有调用 %s", tool)
		}
	}
	for _, tool := range item.Expect.NoTools {
		if slices.Contains(outcome.Tools, tool) {
			fail("不该调用 %s", tool)
		}
	}
	for _, name := range item.Expect.Skills {
		if !slices.Contains(outcome.Skills, name) {
			fail("没有读手册 %s", name)
		}
	}
	for _, ref := range item.Expect.Cites {
		key, ok := r.refs[strings.TrimPrefix(ref, "@")]
		if !ok {
			fail("用例写错了：fixture 里没有 %s", ref)
		} else if !strings.Contains(all, key) {
			fail("没有引用 %s（%s）", ref, key)
		}
	}
	if len(item.Expect.MustContainAny) > 0 && !containsAny(last, item.Expect.MustContainAny) {
		fail("回答里没有出现任何一个：%s", strings.Join(item.Expect.MustContainAny, " / "))
	}
	for _, word := range item.Expect.MustNotContain {
		if strings.Contains(all, word) {
			fail("回答里出现了「%s」", word)
		}
	}
	if item.Expect.Notice != nil && *item.Expect.Notice != noticed {
		if noticed {
			fail("不该补危机提示卡")
		} else {
			fail("应该给出求助建议（回答里没有，也没有补提示卡）")
		}
	}
	if r.judge && len(item.Expect.Rubric) > 0 {
		scores, err := r.score(ctx, item.Expect.Rubric, outcome.Transcript)
		if err != nil {
			fail("打分失败: %v", err)
		}
		outcome.Scores = scores
		for _, score := range scores {
			if !score.Pass {
				fail("rubric 未通过：%s —— %s", score.Rubric, score.Reason)
			}
		}
	}
	return outcome
}

const judgePrompt = `你是家庭关系助理的评测员。下面是一段对话记录（【我】是用户，【助理】是被评测的助理，【助理调用】是它调用的工具），以及若干评分项。

逐条判断助理的回答是否满足评分项：满足给 pass=true，不满足或只满足一半给 false；reason 用一句中文说明依据，引用回答里的原话。
只依据对话记录判断，不要因为回答写得长或语气好就放宽。`

func (r *runner) score(ctx context.Context, rubric []string, transcript string) ([]rubricScore, error) {
	var builder strings.Builder
	builder.WriteString("<transcript>\n" + transcript + "</transcript>\n\n评分项：\n")
	for index, item := range rubric {
		fmt.Fprintf(&builder, "%d. %s\n", index+1, item)
	}
	response, err := r.services.LLM.Chat(ctx, llm.Request{
		Task:       "eval.judge",
		System:     []llm.SystemPart{{Text: judgePrompt}},
		Messages:   []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: builder.String()}}}},
		JSONSchema: judgeSchema(),
		RefKey:     "eval",
	}, nil)
	if err != nil {
		return nil, err
	}
	text := response.Message.Text()
	if start := strings.Index(text, "{"); start > 0 {
		text = text[start:]
	}
	var parsed struct {
		Items []rubricScore `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("解析打分结果: %w", err)
	}
	if len(parsed.Items) != len(rubric) {
		return parsed.Items, fmt.Errorf("打分项数量不对：要 %d 项，给了 %d 项", len(rubric), len(parsed.Items))
	}
	for index := range parsed.Items {
		parsed.Items[index].Rubric = rubric[index]
	}
	return parsed.Items, nil
}

func judgeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"rubric": map[string]any{"type": "string"},
						"pass":   map[string]any{"type": "boolean"},
						"reason": map[string]any{"type": "string"},
					},
					"required":             []string{"rubric", "pass", "reason"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"items"},
		"additionalProperties": false,
	}
}

func appendUnique(list []string, value string) []string {
	if value == "" || slices.Contains(list, value) {
		return list
	}
	return append(list, value)
}

func containsAny(text string, words []string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func oneLine(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
