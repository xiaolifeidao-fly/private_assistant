package chat

import (
	"strings"

	"service/setting"
)

// 回答前后的程序化把关（设计文档 05 · 2.1、2.2）。
// 只记录和补充，不改写已经发出去的正文 —— 历史只追加。

// RoleNotice 是只给人看的固定提示卡，不进模型历史。
const RoleNotice = "notice"

type safetyMeta struct {
	Signals       []string `json:"signals,omitempty"`
	HelpMentioned *bool    `json:"helpMentioned,omitempty"`
	NoticeAdded   bool     `json:"noticeAdded,omitempty"`
}

type citationMeta struct {
	Cited   []string `json:"cited"`
	Unknown []string `json:"unknown,omitempty"`
}

// crisisSignals 找出原话里的危机词。危机信号只是「可能」：命中后提醒模型读 crisis 手册，
// 模型漏了求助建议才补提示卡。先去掉排除短语（「想死你了」），再匹配。
func crisisSignals(text string, safety setting.Safety) []string {
	for _, phrase := range safety.ExcludePhrases {
		text = strings.ReplaceAll(text, phrase, " ")
	}
	var hits []string
	for _, word := range safety.CrisisWords {
		if strings.Contains(text, word) {
			hits = append(hits, word)
		}
	}
	return hits
}

func mentionsHelp(text string, safety setting.Safety) bool {
	for _, word := range safety.HelpWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// safetyContext 是命中危机信号时加进 <turn_context> 的一段。
func safetyContext(signals []string) string {
	return "<safety>系统检测到可能的危机信号（" + strings.Join(signals, "、") +
		"）。先判断是不是真实的危机：是的话按 crisis 手册回答，明确给出求助渠道；只是顺口提到（比如别人家的事）就正常回答。</safety>\n"
}

// checkCitations 比对回答里引用的记忆编号和模型在这段对话里见过的编号。
func checkCitations(answer string, seen map[string]bool) *citationMeta {
	cited := memoryKeys(answer)
	if len(cited) == 0 {
		return nil
	}
	result := &citationMeta{Cited: cited}
	for _, key := range cited {
		if !seen[key] {
			result.Unknown = append(result.Unknown, key)
		}
	}
	return result
}
