package memory

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestVectorIndexRanksByCosine(t *testing.T) {
	index := newVectorIndex()
	index.families["f"] = &familyVectors{model: "m", vectors: map[string][]float32{
		"near": {1, 0},
		"far":  {0, 1},
		"mid":  {float32(math.Sqrt(0.5)), float32(math.Sqrt(0.5))},
		"bad":  {1, 0, 0}, // 维度不一致的脏数据要跳过
	}}
	hits := index.search("f", []float32{1, 0}, 2, nil)
	if len(hits) != 2 || hits[0].MemoryKey != "near" || hits[1].MemoryKey != "mid" {
		t.Fatalf("排序错误: %#v", hits)
	}
	index.upsert("f", "m", "far", []float32{1, 0})
	if hits := index.search("f", []float32{1, 0}, 1, nil); hits[0].Score < 0.99 {
		t.Fatalf("upsert 没生效: %#v", hits)
	}
	index.upsert("f", "other-model", "x", []float32{1, 0})
	if _, ok := index.families["f"].vectors["x"]; ok {
		t.Fatal("不同模型的向量不能混进来")
	}
}

func TestVectorEncodingRoundTrip(t *testing.T) {
	vector := []float32{0.25, -1.5, 3}
	decoded := decodeVector(encodeVector(vector))
	for i := range vector {
		if decoded[i] != vector[i] {
			t.Fatalf("编解码不一致: %v vs %v", vector, decoded)
		}
	}
}

func TestRecencyDecaysOnlyEphemeralTypes(t *testing.T) {
	now := time.Now()
	old := now.AddDate(0, 0, -90)
	quote := Memory{Type: "quote", OccurredAt: &old, CreatedAt: old}
	if got := recency(quote, now); math.Abs(got-0.5) > 0.01 {
		t.Fatalf("原话 90 天后应衰减到一半，实际 %.3f", got)
	}
	trait := Memory{Type: "trait", CreatedAt: old}
	if recency(trait, now) != 1 {
		t.Fatal("性格特质不应随时间衰减")
	}
}

func TestBuildRowDefaults(t *testing.T) {
	service := &Service{}
	row, err := service.buildRow(context.Background(), "f", Input{Type: "custom", PlaceKey: "home_wife_parents", Content: "初二回娘家，给长辈的红包不少于一千"})
	if err != nil {
		t.Fatal(err)
	}
	if row.Scope != "place" || row.Status != StatusActive || row.Importance != 3 || row.Title == "" {
		t.Fatalf("默认值错误: %#v", row)
	}
	if _, err := service.buildRow(context.Background(), "f", Input{Type: "nope", Content: "x"}); err == nil {
		t.Fatal("未知类型应报错")
	}
	if _, err := service.buildRow(context.Background(), "f", Input{Type: "quote", Content: "x", Attrs: []byte("{bad")}); err == nil {
		t.Fatal("非法 attrs 应报错")
	}
}

func TestRelevantGate(t *testing.T) {
	cases := []struct {
		vector, text, top             float64
		vectorUsed, personMatch, want bool
	}{
		{0.70, 0, 0.72, true, false, true},   // 向量高
		{0.45, 0, 0.75, true, false, false},  // 比最好的低太多
		{0.35, 0, 0.38, true, false, false},  // 整体都不相关（无答案的问题）
		{0.35, 0, 0.60, true, true, true},    // 点名的人对得上，放宽
		{0.33, 0.9, 0.70, true, false, true}, // 全文高度匹配
		{0.10, 0.9, 0.70, true, true, false},
		{0, 0.2, 0, false, false, true}, // 没有向量时看全文
		{0, 0, 0, false, false, false},
	}
	for _, item := range cases {
		if got := relevant(item.vector, item.text, item.top, item.vectorUsed, item.personMatch); got != item.want {
			t.Errorf("relevant(%v) = %v", item, got)
		}
	}
}
