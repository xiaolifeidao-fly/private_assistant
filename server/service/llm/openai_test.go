package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIStreamWithToolCalls(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("路径或鉴权错误: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"choices":[{"delta":{"content":"好的，"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"remember","arguments":"{\"type\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"quote\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":30,"completion_tokens":9}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	route := Route{Kind: KindOpenAI, BaseURL: server.URL + "/v1", APIKey: "k", Model: "deepseek-chat", MaxTokens: 100}
	history := []Message{
		{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "记一下"}}},
		{Role: RoleAssistant, Blocks: []Block{{Type: BlockToolUse, ToolUseID: "call_0", ToolName: "now", Input: json.RawMessage(`{}`)}}},
		{Role: RoleUser, Blocks: []Block{{Type: BlockToolResult, ToolUseID: "call_0", Text: "2026-10-07"}}},
	}
	var deltas string
	response, err := chatOpenAI(context.Background(), route, Request{System: []SystemPart{{Text: "sys"}}, Messages: history}, func(d Delta) { deltas += d.Text })
	if err != nil {
		t.Fatal(err)
	}
	if deltas != "好的，" || response.StopReason != StopToolUse {
		t.Fatalf("deltas=%q stop=%s", deltas, response.StopReason)
	}
	uses := response.Message.ToolUses()
	if len(uses) != 1 || uses[0].ToolUseID != "call_1" || string(uses[0].Input) != `{"type":"quote"}` {
		t.Fatalf("工具调用拼接错误: %#v", uses)
	}
	if response.Usage.InputTokens != 30 || response.Usage.OutputTokens != 9 {
		t.Fatalf("用量错误: %#v", response.Usage)
	}
	messages := body["messages"].([]any)
	if len(messages) != 4 || messages[0].(map[string]any)["role"] != "system" || messages[3].(map[string]any)["role"] != "tool" {
		t.Fatalf("消息转换错误: %#v", messages)
	}
}

func TestEmbedNormalizesAndKeepsOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"index":1,"embedding":[0,2]},{"index":0,"embedding":[3,4]}]}`)
	}))
	defer server.Close()
	vectors, err := embedOpenAI(context.Background(), Route{BaseURL: server.URL, Model: "m"}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(vectors[0][0])-0.6) > 1e-6 || math.Abs(float64(vectors[1][1])-1) > 1e-6 {
		t.Fatalf("向量未按 index 归一化: %v", vectors)
	}
}
