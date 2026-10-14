package chat

import (
	"strings"
	"testing"
	"time"

	"service/family"
	"service/llm"
	"service/memory"
)

func testDirectory() directory {
	persons := []family.Person{
		{PersonKey: "me", Name: "我", RoleLabel: "我", Aliases: []string{"我", "老公"}},
		{PersonKey: "wife", Name: "小敏", RoleLabel: "老婆", Aliases: []string{"她", "媳妇"}},
		{PersonKey: "mom", Name: "妈妈", RoleLabel: "我妈", Aliases: []string{"老妈", "奶奶"}},
	}
	dir := directory{persons: map[string]family.Person{}, places: map[string]family.Place{}, ordered: persons}
	for _, person := range persons {
		dir.persons[person.PersonKey] = person
	}
	return dir
}

func TestMentionedFindsPeopleByAlias(t *testing.T) {
	got := testDirectory().mentioned("我妈今天又说媳妇给孩子穿少了")
	if strings.Join(got, ",") != "wife,mom" {
		t.Fatalf("认人错误: %v", got)
	}
}

func TestFormatMemoryCarriesKeyAndStatus(t *testing.T) {
	at := time.Date(2026, 9, 20, 20, 0, 0, 0, time.Local)
	validTo := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	line := formatMemory(testDirectory(), memory.Memory{
		MemoryKey: "mem_1", TypeLabel: "原话", SubjectPerson: "wife", Importance: 4, Title: "当着孩子面说我受不了",
		Content: "你妈每次当着孩子面说我，我真的受不了", OccurredAt: &at, Status: memory.StatusSuperseded, ValidTo: &validTo,
	})
	for _, want := range []string{"[mem_1 | 原话 | 老婆(小敏) | 2026-09-20 | 重要度4]", "你妈每次", "已过时"} {
		if !strings.Contains(line, want) {
			t.Fatalf("缺少 %q: %s", want, line)
		}
	}
}

func TestTextOnlyDropsToolUse(t *testing.T) {
	blocks := textOnly([]llm.Block{{Type: llm.BlockText, Text: "a"}, {Type: llm.BlockToolUse, ToolName: "x"}, {Type: llm.BlockText}})
	if len(blocks) != 1 || blocks[0].Text != "a" {
		t.Fatalf("%#v", blocks)
	}
}
