package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepairJSONAsksAgainOnInvalidOutput(t *testing.T) {
	calls := 0
	var second string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		data, _ := io.ReadAll(r.Body)
		if calls == 2 {
			second = string(data)
		}
		content := `{"name":"卡片","description":"上面写着"TEST"的水印"}`
		if calls == 2 {
			content = `{"name":"卡片","description":"上面写着「TEST」的水印"}`
		}
		encoded, _ := json.Marshal(content)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s},\"finish_reason\":\"stop\"}]}\n\n", encoded)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	gateway := &Gateway{}
	route := Route{Kind: KindOpenAI, BaseURL: server.URL + "/v1", APIKey: "k", Model: "MiniMax-M3", MaxTokens: 100}
	request := Request{
		System:     []SystemPart{{Text: "sys"}},
		Messages:   []Message{{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "这是什么"}}}},
		JSONSchema: map[string]any{"type": "object"},
	}
	first, err := gateway.call(context.Background(), route, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := gateway.repairJSON(context.Background(), route, request, first)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !json.Valid([]byte(JSONText(fixed.Message.Text()))) {
		t.Fatalf("应重试一次并得到合法 JSON: calls=%d text=%s", calls, fixed.Message.Text())
	}
	if !strings.Contains(second, "不是合法的 JSON") {
		t.Fatal("重试时要把解析错误告诉模型")
	}
	if !nativeJSON(Route{Kind: KindAnthropic, Model: "claude-haiku-4-5"}) || nativeJSON(Route{Kind: KindAnthropic, Model: "MiniMax-M3"}) {
		t.Fatal("只有 Claude 走原生结构化输出")
	}
}
