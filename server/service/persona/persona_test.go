package persona

import (
	"strings"
	"testing"
	"time"

	"service/family"
	"service/memory"
)

func TestSystemPromptSectionsForChild(t *testing.T) {
	if !strings.Contains(systemPrompt(family.Person{RoleLabel: "女儿"}), "饮食禁忌") {
		t.Fatal("孩子应使用育儿相关的小节")
	}
	if !strings.Contains(systemPrompt(family.Person{RoleLabel: "老婆"}), "雷区") {
		t.Fatal("成人应包含雷区小节")
	}
	if strings.Contains(systemPrompt(family.Person{RoleLabel: "老婆"}), "近期状态") {
		t.Fatal("近期状态留在记忆里，不进人设卡")
	}
}

func TestCardSchemaIsStrict(t *testing.T) {
	for _, schema := range []map[string]any{cardSchema(), gateSchema()} {
		if schema["additionalProperties"] != false || len(schema["required"].([]string)) != len(schema["properties"].(map[string]any)) {
			t.Fatal("人设卡 schema 不满足结构化输出要求")
		}
	}
}

func TestFormatMemoryMarksSuperseded(t *testing.T) {
	at := time.Date(2026, 5, 1, 0, 0, 0, 0, time.Local)
	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local)
	line := formatMemory(memory.Memory{TypeLabel: "喜好厌恶", Title: "周末喜欢出去玩", Content: "周末喜欢出去玩", OccurredAt: &at,
		SubjectPerson: "wife", Status: memory.StatusSuperseded, ValidTo: &to}, map[string]string{"wife": "老婆"})
	if !strings.Contains(line, "2026-05-01") || !strings.Contains(line, "关于老婆") || !strings.Contains(line, "2026-09-01 起不再成立") {
		t.Fatalf("格式错误: %s", line)
	}
}
