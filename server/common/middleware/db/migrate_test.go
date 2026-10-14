package db

import (
	"os"
	"strings"
	"testing"
)

func TestSplitStatementsOnInitMigration(t *testing.T) {
	content, err := os.ReadFile("../../../migrations/sql/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitStatements(string(content))
	creates, inserts := 0, 0
	for _, statement := range statements {
		if strings.TrimSpace(statement) == "" {
			t.Fatal("出现空语句")
		}
		if strings.HasSuffix(statement, ";") {
			t.Fatalf("语句末尾不应带分号: %s", statement[len(statement)-40:])
		}
		switch {
		case strings.HasPrefix(statement, "CREATE TABLE"):
			creates++
		case strings.HasPrefix(statement, "INSERT"):
			inserts++
		default:
			t.Fatalf("意外的语句开头: %.60s", statement)
		}
	}
	if creates != 17 || inserts != 5 {
		t.Fatalf("期望 17 个建表 + 5 个插入，实际 %d + %d", creates, inserts)
	}
}

func TestSplitStatementsKeepsSemicolonInsideText(t *testing.T) {
	statements := splitStatements("-- 注释\nINSERT INTO t VALUES ('a;b');\nSELECT 1;")
	if len(statements) != 2 || statements[0] != "INSERT INTO t VALUES ('a;b')" {
		t.Fatalf("分句错误: %#v", statements)
	}
}

func TestSplitStatementsOnReminderMigration(t *testing.T) {
	content, err := os.ReadFile("../../../migrations/sql/0005_reminder.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitStatements(string(content))
	if len(statements) != 6 {
		t.Fatalf("提醒迁移应有 4 个建表 + 2 个升级语句，实际 %d", len(statements))
	}
	for index, statement := range statements {
		if index < 4 && !strings.HasPrefix(statement, "CREATE TABLE IF NOT EXISTS") {
			t.Fatalf("第 %d 条不是可重入建表: %.80s", index+1, statement)
		}
		if index >= 4 && !strings.HasPrefix(statement, "UPDATE pa_bot") {
			t.Fatalf("第 %d 条不是机器人升级: %.80s", index+1, statement)
		}
	}
}

func TestSplitStatementsOnSkillMigration(t *testing.T) {
	content, err := os.ReadFile("../../../migrations/sql/0006_skill_harness.sql")
	if err != nil {
		t.Fatal(err)
	}
	statements := splitStatements(string(content))
	prefixes := []string{"CREATE TABLE IF NOT EXISTS pa_skill", "ALTER TABLE pa_conversation", "ALTER TABLE pa_message", "UPDATE pa_bot", "INSERT IGNORE INTO pa_llm_route"}
	if len(statements) != len(prefixes) {
		t.Fatalf("应有 %d 条语句，实际 %d", len(prefixes), len(statements))
	}
	for index, prefix := range prefixes {
		if !strings.HasPrefix(statements[index], prefix) {
			t.Fatalf("第 %d 条应以 %q 开头: %.80s", index+1, prefix, statements[index])
		}
	}
}

func TestSplitStatementsOnLaterMigrations(t *testing.T) {
	for name, want := range map[string]int{"0007_setting_learning.sql": 3, "0008_m5.sql": 5} {
		content, err := os.ReadFile("../../../migrations/sql/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(splitStatements(string(content))); got != want {
			t.Fatalf("%s 应有 %d 条语句，实际 %d", name, want, got)
		}
	}
}
