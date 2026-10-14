package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"service/llm"
)

// 会话分段（设计文档 05 · 2.4）：上一轮输入超过预算时，把当前分段压缩成交接摘要，
// 之后只回放新分段。旧分段原样留在库里，系统提示词不变 —— 不改写任何已发出的内容。

const summaryTask = "chat.summary"

const summaryPrompt = `你在给一段家庭助理对话写交接摘要。接手的是同一个助理，它看不到这段对话，只能看到你的摘要，然后继续和我聊。

按下面四节写，每节用短句列点，没有内容的节写「无」：
## 已确认的事实
对话里确认过的情况、家人说过的话，句末保留记忆编号（如 [mem_xxx]）。
## 我做过的决定
我说了要怎么做、选了哪种说法。
## 还没办完的
没回答完的问题、答应了还没做的事、等我回复的确认。
## 读过的手册和工具结论
读过哪些做事手册（load_skill 的名字），分身预演得出的关键结论。

只写对话里有的，不要补充、不要建议。用中文，控制在 1500 字以内。`

// lastInputTokens 返回当前分段最后一次模型调用的输入 token（含缓存读写）。
func (s *Service) lastInputTokens(ctx context.Context, conversation conversationRow) int {
	var row messageRow
	err := s.db.WithContext(ctx).Where("conversation_key = ? AND segment = ? AND role = ? AND status = 'done'",
		conversation.ConversationKey, conversation.Segment, llm.RoleAssistant).Order("id DESC").First(&row).Error
	if err != nil || len(row.Usage) == 0 {
		return 0
	}
	var usage llm.Usage
	if json.Unmarshal(row.Usage, &usage) != nil {
		return 0
	}
	return usage.InputTokens + usage.CacheReadTokens + usage.CacheWriteTokens
}

// rollSegment 把当前分段压缩成摘要并开新分段，返回摘要和摘要漏掉的记忆。失败时不分段。
//
// 「漏掉」= 助理在这一段的回答里引用过、摘要里却没有的记忆编号。分段阈值合不合适，
// 就看这个数：经常漏关键记忆，说明阈值太小（分得太勤）或摘要太短。
func (s *Service) rollSegment(ctx context.Context, conversation *conversationRow) (string, []string, error) {
	history, err := s.history(ctx, conversation.ConversationKey, conversation.Segment)
	if err != nil {
		return "", nil, err
	}
	transcript := renderTranscript(history)
	if transcript == "" {
		return "", nil, errors.New("当前分段没有内容")
	}
	response, err := s.llm.Chat(ctx, llm.Request{
		Task:     summaryTask,
		System:   []llm.SystemPart{{Text: summaryPrompt}},
		Messages: []llm.Message{{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: transcript}}}},
		RefKey:   conversation.ConversationKey,
	}, nil)
	if err != nil {
		return "", nil, err
	}
	summary := strings.TrimSpace(response.Message.Text())
	if summary == "" {
		return "", nil, errors.New("摘要为空")
	}
	next := conversation.Segment + 1
	if err := s.db.WithContext(ctx).Model(&conversationRow{}).Where("id = ?", conversation.ID).Update("segment", next).Error; err != nil {
		return "", nil, err
	}
	conversation.Segment = next
	return summary, lostCitations(history, summary), nil
}

// lostCitations 是助理回答里引用过、摘要里没有的记忆编号。
func lostCitations(history []llm.Message, summary string) []string {
	kept := map[string]bool{}
	for _, key := range memoryKeys(summary) {
		kept[key] = true
	}
	seen := map[string]bool{}
	var lost []string
	for _, message := range history {
		if message.Role != llm.RoleAssistant {
			continue
		}
		for _, key := range memoryKeys(message.Text()) {
			if !kept[key] && !seen[key] {
				seen[key] = true
				lost = append(lost, key)
			}
		}
	}
	return lost
}

// renderTranscript 把历史转成给摘要模型读的纯文本。工具结果只留开头，够判断查到了什么。
func renderTranscript(history []llm.Message) string {
	var builder strings.Builder
	for _, message := range history {
		for _, block := range message.Blocks {
			switch block.Type {
			case llm.BlockText:
				text := strings.TrimSpace(block.Text)
				if text == "" {
					continue
				}
				if message.Role == llm.RoleUser && strings.HasPrefix(text, "<turn_context>") {
					continue // 每轮自动检索的参考，下一段会重新检索
				}
				if strings.HasPrefix(text, "<previous_summary>") {
					builder.WriteString("【更早的摘要】" + text + "\n")
				} else if message.Role == llm.RoleUser {
					builder.WriteString("【我】" + text + "\n")
				} else {
					builder.WriteString("【助理】" + text + "\n")
				}
			case llm.BlockToolUse:
				fmt.Fprintf(&builder, "【助理调用 %s】%s\n", block.ToolName, oneLine(string(block.Input), 200))
			case llm.BlockToolResult:
				fmt.Fprintf(&builder, "【%s 结果】%s\n", block.ToolName, oneLine(block.Text, 400))
			}
		}
	}
	return builder.String()
}

func previousSummaryBlock(summary string) string {
	return "<previous_summary>\n这段对话太长，前面的内容已压缩成下面的摘要。需要细节时用工具重新查，读过的手册需要时重新读。\n\n" +
		summary + "\n</previous_summary>"
}
