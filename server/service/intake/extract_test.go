package intake

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"service/family"
	"service/memory"
)

// 结构化输出要求：每个 object 都有 additionalProperties:false，且 required 覆盖全部属性。
func checkStrictSchema(t *testing.T, path string, schema map[string]any) {
	t.Helper()
	switch schema["type"] {
	case "object":
		if schema["additionalProperties"] != false {
			t.Fatalf("%s 缺少 additionalProperties:false", path)
		}
		properties := schema["properties"].(map[string]any)
		required := map[string]bool{}
		for _, name := range schema["required"].([]string) {
			required[name] = true
		}
		for name, value := range properties {
			if !required[name] {
				t.Fatalf("%s.%s 不在 required 里", path, name)
			}
			checkStrictSchema(t, path+"."+name, value.(map[string]any))
		}
	case "array":
		checkStrictSchema(t, path+"[]", schema["items"].(map[string]any))
	}
	for _, unsupported := range []string{"minimum", "maximum", "minLength", "maxLength"} {
		if _, ok := schema[unsupported]; ok {
			t.Fatalf("%s 使用了结构化输出不支持的 %s", path, unsupported)
		}
	}
}

func TestSchemasAreStrict(t *testing.T) {
	checkStrictSchema(t, "extraction", extractionSchema())
	checkStrictSchema(t, "reconcile", reconcileSchema())
}

func TestSanitizeCleansModelOutput(t *testing.T) {
	persons := map[string]family.Person{"wife": {PersonKey: "wife"}, "mom": {PersonKey: "mom"}}
	occurred := time.Date(2026, 10, 6, 21, 0, 0, 0, time.Local)
	items := sanitize([]candidate{
		{Type: "quote", SubjectPerson: "wife", Persons: []string{"wife", "mom", "大舅"}, Speaker: "wife", Content: "原话", Importance: 9, Confidence: 0},
		{Type: "event", SubjectPerson: "表姐", Title: "", Content: ""},
		{Type: "event", SubjectPerson: "表姐", Content: "表姐来家里吃饭"},
	}, persons, noteRow{OccurredAt: &occurred})
	if len(items) != 2 {
		t.Fatalf("空内容的候选应被丢弃: %#v", items)
	}
	first := items[0]
	if strings.Join(first.Persons, ",") != "mom" || first.Importance != 3 || first.Confidence != 0.7 || first.OccurredAt != "2026-10-06 21:00" {
		t.Fatalf("清洗错误: %#v", first)
	}
	if items[1].SubjectPerson != "" {
		t.Fatal("认不出的人应置空")
	}
}

func TestAttrsOf(t *testing.T) {
	var attrs map[string]string
	_ = json.Unmarshal(attrsOf(candidate{Type: "commitment", PromisedTo: "wife", DueAt: "2026-10-20", Evidence: "我说国庆后带你去杭州", Content: "带她去杭州"}, "note_a"), &attrs)
	if attrs["status"] != "open" || attrs["promisedTo"] != "wife" || attrs["dueAt"] != "2026-10-20" || attrs["evidence"] == "" {
		t.Fatalf("承诺的扩展字段错误: %#v", attrs)
	}
	if attrsOf(candidate{Type: "trait", Content: "x", Evidence: "x"}, "note_a") != nil {
		t.Fatal("没有扩展信息时应为空")
	}
}

func TestItemAttrsAndMerge(t *testing.T) {
	first := attrsOf(candidate{Type: "item", Location: "书房 / 书桌右边抽屉", Usage: "出国用", Photos: []int{0, 2}}, "note_a")
	var attrs struct {
		Location string            `json:"location"`
		Usage    string            `json:"usage"`
		Photos   []memory.PhotoRef `json:"photos"`
	}
	_ = json.Unmarshal(first, &attrs)
	if attrs.Location != "书房 / 书桌右边抽屉" || attrs.Usage != "出国用" || len(attrs.Photos) != 2 || attrs.Photos[1] != (memory.PhotoRef{Note: "note_a", Index: 2}) {
		t.Fatalf("物品字段错误: %s", first)
	}
	merged := memory.MergeAttrs(first, attrsOf(candidate{Type: "item", Photos: []int{0}}, "note_b"))
	got := memory.Memory{Attrs: merged}.Photos()
	if len(got) != 3 || got[2] != (memory.PhotoRef{Note: "note_b", Index: 0}) {
		t.Fatalf("照片应取并集: %s", merged)
	}
	if !strings.Contains(string(merged), "书桌右边抽屉") {
		t.Fatalf("没给新位置时保留旧位置: %s", merged)
	}
}

func TestSanitizePhotos(t *testing.T) {
	row := noteRow{Attachments: []byte(`[{"path":"a","mediaType":"image/jpeg"}]`)}
	items := sanitize([]candidate{
		{Type: "item", Title: "电钻", Content: "电钻", Photos: []int{0, 0, 5, -1}},
		{Type: "item", Title: "螺丝刀", Content: "螺丝刀"},
	}, map[string]family.Person{}, row)
	if len(items[0].Photos) != 1 || items[0].Photos[0] != 0 {
		t.Fatalf("越界和重复的序号应去掉: %v", items[0].Photos)
	}
	if len(items[1].Photos) != 1 {
		t.Fatal("只有一张照片时物品默认引用它")
	}
}

func TestExtractJSONStripsFences(t *testing.T) {
	if got := extractJSON("```json\n{\"a\":1}\n```"); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}

func TestNoteHeader(t *testing.T) {
	occurred := time.Date(2026, 10, 6, 21, 0, 0, 0, time.Local)
	header := noteHeader(noteRow{
		CreatedAt: occurred.Add(time.Hour), OccurredAt: &occurred, AboutPersons: []string{"wife"}, Channel: "screenshot",
		RawText: "她说……", Attachments: []byte(`[{"path":"a","mediaType":"image/jpeg"}]`),
	}, map[string]family.Person{"wife": {PersonKey: "wife", RoleLabel: "老婆"}})
	for _, want := range []string{"发生时间：2026-10-06 21:00", "老婆(key=wife)", "聊天截图", "附了 1 张截图", "她说……"} {
		if !strings.Contains(header, want) {
			t.Fatalf("缺少 %q:\n%s", want, header)
		}
	}
}
