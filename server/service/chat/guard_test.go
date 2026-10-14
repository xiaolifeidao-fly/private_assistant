package chat

import (
	"encoding/json"
	"strings"
	"testing"

	"service/llm"
	"service/setting"
)

func TestIsSmallTalk(t *testing.T) {
	budget := setting.DefaultBudget()
	for _, text := range []string{"好的", "好的谢谢！", "嗯嗯", "OK", "收到~", "知道了。"} {
		if !isSmallTalk(text, budget) {
			t.Errorf("%q 应算寒暄", text)
		}
	}
	for _, text := range []string{"好的，那我妈那边怎么说", "可以吗", "她又生气了", "谢谢你帮我想这么多，我还有一个问题想问"} {
		if isSmallTalk(text, budget) {
			t.Errorf("%q 不应算寒暄", text)
		}
	}
}

func TestCrisisSignals(t *testing.T) {
	safety := setting.DefaultSafety()
	got := crisisSignals("她昨晚说要离婚，我真的活不下去了", safety)
	if strings.Join(got, ",") != "离婚,活不下去" {
		t.Fatalf("危机词识别错误: %v", got)
	}
	for _, text := range []string{"今天带孩子去公园了", "女儿说想死你了爸爸", "今天热死了"} {
		if hits := crisisSignals(text, safety); len(hits) != 0 {
			t.Fatalf("%q 不应命中: %v", text, hits)
		}
	}
}

func TestCheckCitationsFlagsUnknownKeys(t *testing.T) {
	seen := seenMemoryKeys("常驻记忆 [mem_aa11 | 原话 | 老婆 | 2026-09-20 | 重要度4]", []llm.Message{
		{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockToolResult, Text: "[mem_bb22 | 习惯 …]"}}},
		{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockToolUse, Input: json.RawMessage(`{"supersedes":"mem_cc33"}`)}}},
	})
	result := checkCitations("她说过受不了 [mem_aa11]，也在意 [mem_bb22、mem_cc33]，还有 [mem_zz99]", seen)
	if result == nil || strings.Join(result.Cited, ",") != "mem_aa11,mem_bb22,mem_cc33,mem_zz99" {
		t.Fatalf("引用识别错误: %#v", result)
	}
	if strings.Join(result.Unknown, ",") != "mem_zz99" {
		t.Fatalf("未知引用识别错误: %v", result.Unknown)
	}
	if checkCitations("没有引用", seen) != nil {
		t.Fatal("没有引用时应返回 nil")
	}
}

func TestClipToolOutput(t *testing.T) {
	short := "一行\n两行"
	if clipToolOutput(short, 6000) != short {
		t.Fatal("短结果不应截断")
	}
	long := strings.Repeat("这是一条很长的记忆内容。\n", 1000)
	clipped := clipToolOutput(long, 6000)
	if len([]rune(clipped)) > 6100 || !strings.Contains(clipped, "已截断") {
		t.Fatalf("截断不对，长度 %d", len([]rune(clipped)))
	}
}

func TestRenderTranscriptSkipsTurnContext(t *testing.T) {
	text := renderTranscript([]llm.Message{
		{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "<turn_context>\n当前时间…</turn_context>"}, {Type: llm.BlockText, Text: "她又生气了"}}},
		{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockToolUse, ToolName: "load_skill", Input: json.RawMessage(`{"name":"wife_repair"}`)}}},
		{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockToolResult, ToolName: "load_skill", Text: "# 让老婆感到被在乎（v1）"}}},
		{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "先别解释。"}}},
	})
	if strings.Contains(text, "当前时间") || !strings.Contains(text, "【我】她又生气了") || !strings.Contains(text, "wife_repair") || !strings.Contains(text, "【助理】先别解释") {
		t.Fatalf("转写不对:\n%s", text)
	}
}

func TestTurnMetaEmptyIsNull(t *testing.T) {
	if (turnMeta{}).json() != nil {
		t.Fatal("空 meta 应存 NULL")
	}
}

func TestRenderForLearningKeepsOnlyMyWordsForPersona(t *testing.T) {
	rows := []messageRow{
		{Role: llm.RoleUser, Content: "她昨天说想换工作"},
		{Role: llm.RoleAssistant, Content: "（模拟）我最近确实很累"},
	}
	text, mine := renderForLearning(rows, true)
	if strings.Contains(text, "模拟") || !strings.Contains(text, "【我】她昨天说想换工作") || mine != 8 {
		t.Fatalf("分身对话只应保留我的话: %q %d", text, mine)
	}
	text, _ = renderForLearning(rows, false)
	if !strings.Contains(text, "【助理】") {
		t.Fatalf("助理对话应保留助理的话作上下文: %q", text)
	}
}

func TestRenderForLearningKeepsNewestWhenTooLong(t *testing.T) {
	var rows []messageRow
	for index := 0; index < 3000; index++ {
		rows = append(rows, messageRow{Role: llm.RoleUser, Content: strings.Repeat("字", 20)})
	}
	rows = append(rows, messageRow{Role: llm.RoleUser, Content: "最后一句"})
	text, _ := renderForLearning(rows, false)
	if len([]rune(text)) > learnMaxRunes+3000 || !strings.HasSuffix(text, "最后一句") {
		t.Fatalf("太长时应保留最新的，长度 %d", len([]rune(text)))
	}
}

func TestLostCitations(t *testing.T) {
	history := []llm.Message{
		{Role: llm.RoleUser, Blocks: []llm.Block{{Type: llm.BlockText, Text: "参考 [mem_u1]"}}},
		{Role: llm.RoleAssistant, Blocks: []llm.Block{{Type: llm.BlockText, Text: "她说过 [mem_a1]，也在意 [mem_a2]"}}},
	}
	lost := lostCitations(history, "## 已确认的事实\n- 她受不了当众被说 [mem_a1]")
	if strings.Join(lost, ",") != "mem_a2" {
		t.Fatalf("漏记统计错误: %v", lost)
	}
}
