package reminder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"service/memory"
)

func TestNextLunarAnnual(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, shanghai)
	got, err := nextLunarAnnual("01-01", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format("2006-01-02") != "2026-02-17" {
		t.Fatalf("2026 年春节换算错误: %s", got)
	}
}

func TestNextLunarAnnualRollsToNextYear(t *testing.T) {
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, shanghai)
	got, err := nextLunarAnnual("01-01", from)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format("2006-01-02") != "2027-02-06" {
		t.Fatalf("应跳到下一个春节: %s", got)
	}
}

func TestParseLeapLunarBirthday(t *testing.T) {
	month, day, leap, err := parseMonthDay("L07-15")
	if err != nil || month != 7 || day != 15 || !leap {
		t.Fatalf("闰月解析错误: %d %d %v %v", month, day, leap, err)
	}
}

func TestAttrDueHonorsCompletion(t *testing.T) {
	open := memory.Memory{Attrs: json.RawMessage(`{"dueAt":"2026-10-20","status":"open"}`)}
	if due, ok := attrDue(open); !ok || due.Format("2006-01-02") != "2026-10-20" {
		t.Fatalf("开放承诺的截止日解析失败: %s %v", due, ok)
	}
	done := memory.Memory{Attrs: json.RawMessage(`{"dueAt":"2026-10-20","status":"done"}`)}
	if _, ok := attrDue(done); ok {
		t.Fatal("已完成的承诺不应再派生提醒")
	}
}

func TestPushTopicIsWebPushSafeLength(t *testing.T) {
	topic := pushTopic("rmd_abcdefghijklmnopqrstuvwxyz_0123456789")
	if len(topic) > 32 || strings.Contains(topic, "_") {
		t.Fatalf("不合法的 topic: %q", topic)
	}
}
