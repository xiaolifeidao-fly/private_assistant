package chat

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"common/middleware/db"
	"service/setting"
)

// 上下文预算（设计文档 05 · 2.3）。具体数字在管理端「对话规则」里配置（service/setting）。

// turnMeta 记在 pa_message.meta 里，管理端用来排查「这一轮发生了什么」。
type turnMeta struct {
	Retrieval *retrievalMeta `json:"retrieval,omitempty"`
	Safety    *safetyMeta    `json:"safety,omitempty"`
	Segment   *segmentMeta   `json:"segment,omitempty"`
	Citations *citationMeta  `json:"citations,omitempty"`
	// PersonaUpdates 这一轮提示了谁的人设卡更新（手机端在这句话下面显示「她的人设变了」）
	PersonaUpdates []personaUpdate `json:"personaUpdates,omitempty"`
	// Attachment 我这句话带的照片（原始记录）和当场整理出的记忆
	Attachment *attachmentMeta `json:"attachment,omitempty"`
	// Photos 回答里引用到的记忆所附的照片，手机端在回答下面显示原图
	Photos []photoMeta `json:"photos,omitempty"`
}

type personaUpdate struct {
	Person  string `json:"person"`
	From    int    `json:"from"`
	To      int    `json:"to"`
	Changes string `json:"changes"`
}

func (meta turnMeta) json() db.JSON {
	if meta.Retrieval == nil && meta.Safety == nil && meta.Segment == nil && meta.Citations == nil && len(meta.PersonaUpdates) == 0 &&
		meta.Attachment == nil && len(meta.Photos) == 0 {
		return nil
	}
	return db.MustJSON(meta)
}

type retrievalMeta struct {
	Skipped bool `json:"skipped"`
	Hits    int  `json:"hits"`
	Deduped int  `json:"deduped"`
	Clipped int  `json:"clipped"`
}

type segmentMeta struct {
	Rolled       bool     `json:"rolled"`
	From         int      `json:"from"`
	To           int      `json:"to"`
	Tokens       int      `json:"tokens"`       // 触发分段时上一轮的输入 token
	SummaryChars int      `json:"summaryChars"` // 摘要字数
	Lost         []string `json:"lost,omitempty"`
}

// isSmallTalk 判断是不是寒暄（好的、谢谢、嗯……）。寒暄不检索记忆，免得塞进一堆无关参考。
// 长的词先替换，免得「好的」被「好」拆开。
func isSmallTalk(text string, budget setting.Budget) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || len([]rune(text)) > budget.SmallTalkMaxRunes {
		return false
	}
	words := append([]string(nil), budget.SmallTalkWords...)
	sort.SliceStable(words, func(i, j int) bool { return len(words[i]) > len(words[j]) })
	for _, word := range words {
		text = strings.ReplaceAll(text, strings.ToLower(word), "")
	}
	return strings.Trim(text, " ,，.。!！~～、?？啊呀啦了") == ""
}

var memoryKeyPattern = regexp.MustCompile(`mem_[0-9A-Za-z]+`)

// memoryKeys 找出文本里出现的全部记忆编号（去重，保持顺序）。
func memoryKeys(text string) []string {
	seen := map[string]bool{}
	var result []string
	for _, key := range memoryKeyPattern.FindAllString(text, -1) {
		if !seen[key] {
			seen[key] = true
			result = append(result, key)
		}
	}
	return result
}

// clipToolOutput 给工具结果设上限。按行截断，告诉模型怎么缩小范围。
func clipToolOutput(output string, limit int) string {
	runes := []rune(output)
	if len(runes) <= limit {
		return output
	}
	cut := string(runes[:limit])
	if index := strings.LastIndex(cut, "\n"); index > 0 {
		cut = cut[:index]
	}
	return cut + fmt.Sprintf("\n……（结果太长，已截断，原文共 %d 字。需要更多请缩小范围再查，比如指定人或类型。）", len(runes))
}
