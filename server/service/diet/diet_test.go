package diet

import (
	"strings"
	"testing"
	"time"

	"service/identity"
)

func TestGuessMeal(t *testing.T) {
	cases := map[string]string{"07:10": "breakfast", "10:20": "morning_snack", "12:30": "lunch", "15:00": "afternoon_snack", "18:40": "dinner", "21:00": "night_snack", "02:00": "breakfast"}
	for clock, want := range cases {
		at, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-18 "+clock, shanghai)
		if got := GuessMeal(at); got != want {
			t.Errorf("%s: got %s want %s", clock, got, want)
		}
	}
}

func TestCanPlan(t *testing.T) {
	for _, role := range []string{identity.RoleAdmin, identity.RoleMember, identity.RoleSpouse} {
		if !(Actor{Role: role}).CanPlan() {
			t.Errorf("%s 应该能改计划", role)
		}
	}
	if (Actor{Role: identity.RoleElder}).CanPlan() {
		t.Error("奶奶的账号不能改计划")
	}
}

func TestAgeOf(t *testing.T) {
	today := time.Date(2026, 10, 18, 0, 0, 0, 0, time.Local)
	if got := ageOf("2023-04-20", today); got != "3 岁 5 个月" {
		t.Errorf("got %q", got)
	}
	if ageOf("", today) != "" {
		t.Error("没生日应为空")
	}
}

func TestReply(t *testing.T) {
	reply := replyFor([]Record{{Meal: "lunch", Foods: "米饭、青菜", Amount: "all"}}, false)
	if !strings.Contains(reply, "午饭：米饭、青菜，吃完了") {
		t.Errorf("got %q", reply)
	}
}

func TestSchemasAreStrict(t *testing.T) {
	for _, schema := range []map[string]any{extractSchema(), planSchema()} {
		if schema["additionalProperties"] != false || len(schema["required"].([]string)) != len(schema["properties"].(map[string]any)) {
			t.Fatal("schema 不满足结构化输出要求")
		}
		for _, value := range schema["properties"].(map[string]any) {
			items, ok := value.(map[string]any)["items"].(map[string]any)
			if ok && len(items["required"].([]string)) != len(items["properties"].(map[string]any)) {
				t.Fatal("item schema 不满足结构化输出要求")
			}
		}
	}
}

func TestWeekStartOf(t *testing.T) {
	for date, want := range map[string]string{"2026-10-18": "2026-10-12", "2026-10-12": "2026-10-12", "2026-10-14": "2026-10-12"} {
		day, _ := parseDate(date)
		if got := weekStartOf(day).Format(dateLayout); got != want {
			t.Errorf("%s: got %s want %s", date, got, want)
		}
	}
}

func TestFormatOverview(t *testing.T) {
	eaten, _ := time.ParseInLocation("2006-01-02 15:04", "2026-10-12 12:10", time.Local)
	text := formatOverview(Overview{ChildLabel: "女儿", From: "2026-10-12", Days: 2,
		Plans:   []Plan{{Date: "2026-10-12", Meal: "lunch", Foods: "米饭、鱼", Note: "少盐"}, {Date: "2026-10-12", Meal: "dinner", Foods: "面条"}},
		Records: []Record{{EatenAt: eaten, Meal: "lunch", Foods: "米饭", Amount: "half", Note: "鱼没吃"}}})
	for _, want := range []string{"午饭　计划：米饭、鱼（要求：少盐）；实际：12:10 米饭，吃了一半（鱼没吃）", "晚饭　计划：面条；实际：没记", "2026-10-13（周二）：没有计划，也没有记录"} {
		if !strings.Contains(text, want) {
			t.Errorf("缺少 %q:\n%s", want, text)
		}
	}
}
