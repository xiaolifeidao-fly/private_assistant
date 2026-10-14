package skill

import (
	"strings"
	"testing"
)

func TestDefaultsAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range defaults() {
		if !validKey(item.SkillKey) || seen[item.SkillKey] {
			t.Errorf("手册标识不合法或重复: %s", item.SkillKey)
		}
		seen[item.SkillKey] = true
		if item.Name == "" || item.WhenToUse == "" || strings.TrimSpace(item.Body) == "" || len(item.BotKeys) == 0 {
			t.Errorf("手册 %s 缺内容", item.SkillKey)
		}
	}
	if !seen["crisis"] {
		t.Error("必须有 crisis 手册：危机兜底会让模型读它")
	}
}

func TestAvailableTo(t *testing.T) {
	item := Skill{BotKeys: []string{"assistant"}, Enabled: true}
	if !item.AvailableTo("assistant") || item.AvailableTo("wife") {
		t.Fatal("适用范围判断错误")
	}
	item.Enabled = false
	if item.AvailableTo("assistant") {
		t.Fatal("停用的手册不应开放")
	}
}
