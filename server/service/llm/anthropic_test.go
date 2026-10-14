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

// 模拟 Claude 流式响应：thinking（带签名）+ 文本 + 工具调用。
const toolUseStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":120,"output_tokens":1,"cache_read_input_tokens":100,"cache_creation_input_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"SIG_ABC"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"我查一下"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"search_memory","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"老婆的生日\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":2}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}

event: message_stop
data: {"type":"message_stop"}

`

const textStream = `event: message_start
data: {"type":"message_start","message":{"id":"msg_2","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":200,"output_tokens":1}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"她的生日是农历九月初八。"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":12}}

event: message_stop
data: {"type":"message_stop"}

`

type capture struct {
	bodies  []map[string]any
	headers []http.Header
}

func fakeClaude(t *testing.T, streams ...string) (*httptest.Server, *capture) {
	t.Helper()
	captured := &capture{}
	call := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		}
		captured.bodies = append(captured.bodies, body)
		captured.headers = append(captured.headers, r.Header.Clone())
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, streams[call%len(streams)])
		call++
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func TestAnthropicToolRoundTripPreservesThinking(t *testing.T) {
	server, captured := fakeClaude(t, toolUseStream, textStream)
	route := Route{Task: "chat.assistant", ProviderKey: "anthropic", Kind: KindAnthropic, BaseURL: server.URL, APIKey: "test", Model: "claude-opus-5-5", MaxTokens: 1000, Effort: "high"}
	request := Request{
		System:   []SystemPart{{Text: "你是助理", Cache: true}},
		Messages: []Message{{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "<turn_context>…</turn_context>"}, {Type: BlockText, Text: "老婆生日哪天？"}}}},
		Tools:    []Tool{{Name: "search_memory", Description: "检索", Properties: map[string]any{"query": map[string]any{"type": "string"}}, Required: []string{"query"}}},
	}

	var streamed strings.Builder
	first, err := chatAnthropic(context.Background(), route, request, func(d Delta) { streamed.WriteString(d.Text) })
	if err != nil {
		t.Fatal(err)
	}
	if first.StopReason != StopToolUse || streamed.String() != "我查一下" {
		t.Fatalf("stop=%s streamed=%q", first.StopReason, streamed.String())
	}
	uses := first.Message.ToolUses()
	if len(uses) != 1 || uses[0].ToolName != "search_memory" || uses[0].ToolUseID != "toolu_1" {
		t.Fatalf("工具调用解析错误: %#v", uses)
	}
	var input struct{ Query string }
	if err := json.Unmarshal(uses[0].Input, &input); err != nil || input.Query != "老婆的生日" {
		t.Fatalf("工具入参错误: %s %v", uses[0].Input, err)
	}
	if !strings.Contains(string(first.Message.Raw), "SIG_ABC") {
		t.Fatalf("Raw 里没有保留 thinking 签名: %s", first.Message.Raw)
	}
	if first.Usage.CacheReadTokens != 100 || first.Usage.OutputTokens != 42 {
		t.Fatalf("用量错误: %#v", first.Usage)
	}

	// 第一次请求的形状：系统提示词打缓存标记、工具开流式入参、effort、没有 fallbacks（自定义 base_url）
	body := captured.bodies[0]
	system := body["system"].([]any)[0].(map[string]any)
	if system["cache_control"] == nil {
		t.Fatal("系统提示词没有 cache_control")
	}
	tool := body["tools"].([]any)[0].(map[string]any)
	if tool["eager_input_streaming"] != true {
		t.Fatal("工具没有开启 eager_input_streaming")
	}
	if body["output_config"].(map[string]any)["effort"] != "high" {
		t.Fatal("effort 没有传下去")
	}
	if _, ok := body["fallbacks"]; ok {
		t.Fatal("自定义 base_url 时不应发送 fallbacks")
	}

	// 第二轮：带上 assistant 原生内容 + 工具结果，thinking 块必须原样回传
	request.Messages = append(request.Messages, first.Message, Message{Role: RoleUser, Blocks: []Block{{Type: BlockToolResult, ToolUseID: "toolu_1", Text: "农历九月初八"}}})
	second, err := chatAnthropic(context.Background(), route, request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.Message.Text() != "她的生日是农历九月初八。" || second.StopReason != StopEnd {
		t.Fatalf("第二轮结果错误: %#v", second)
	}
	messages := captured.bodies[1]["messages"].([]any)
	if len(messages) != 3 {
		t.Fatalf("期望 3 条历史，实际 %d", len(messages))
	}
	assistant := messages[1].(map[string]any)["content"].([]any)
	thinking := assistant[0].(map[string]any)
	if thinking["type"] != "thinking" || thinking["signature"] != "SIG_ABC" {
		t.Fatalf("thinking 块没有原样回传: %#v", thinking)
	}
	result := messages[2].(map[string]any)["content"].([]any)[0].(map[string]any)
	if result["type"] != "tool_result" || result["tool_use_id"] != "toolu_1" {
		t.Fatalf("工具结果错误: %#v", result)
	}

	// 同一会话两次请求的前缀必须字节一致（只追加）
	firstPrefix, _ := json.Marshal(captured.bodies[0]["messages"].([]any)[0])
	secondPrefix, _ := json.Marshal(messages[0])
	if string(firstPrefix) != string(secondPrefix) {
		t.Fatal("历史前缀发生了变化")
	}
}

func TestAnthropicRetriesWithoutThinkingOnSignatureError(t *testing.T) {
	call := 0
	var second map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call++
		if call == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"messages.1.content.0: Invalid `+"`signature`"+` in `+"`thinking`"+` block. The block is bound to a different conversation."}}`)
			return
		}
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &second)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, textStream)
	}))
	defer server.Close()

	raw := json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"OLD"},{"type":"text","text":"之前的回答"}]}`)
	request := Request{Messages: []Message{
		{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "问题一"}}},
		{Role: RoleAssistant, Provider: KindAnthropic, Raw: raw, Blocks: []Block{{Type: BlockText, Text: "之前的回答"}}},
		{Role: RoleUser, Blocks: []Block{{Type: BlockText, Text: "问题二"}}},
	}}
	route := Route{Kind: KindAnthropic, BaseURL: server.URL, APIKey: "test", Model: "claude-opus-5-5", MaxTokens: 100}
	if _, err := chatAnthropic(context.Background(), route, request, nil); err != nil {
		t.Fatal(err)
	}
	if call != 2 {
		t.Fatalf("应该重试一次，实际请求 %d 次", call)
	}
	data, _ := json.Marshal(second["messages"])
	if strings.Contains(string(data), "thinking") || !strings.Contains(string(data), "之前的回答") {
		t.Fatalf("重试时应去掉 thinking、保留文本: %s", data)
	}
}

func TestSupportsDefaultFallback(t *testing.T) {
	cases := []struct {
		baseURL, model string
		want           bool
	}{
		{"", "claude-opus-5-5", true},
		{"", "claude-sonnet-5-5", true},
		{"", "claude-haiku-4-5", false},
		{"https://api.anthropic.com", "claude-opus-5-5", true},
		{"https://my-proxy.example.com", "claude-opus-5-5", false},
	}
	for _, c := range cases {
		if got := supportsDefaultFallback(Route{BaseURL: c.baseURL, Model: c.model}); got != c.want {
			t.Errorf("%s %s: got %v", c.baseURL, c.model, got)
		}
	}
}

func TestAnthropicImageAndJSONSchema(t *testing.T) {
	server, captured := fakeClaude(t, textStream)
	route := Route{Kind: KindAnthropic, BaseURL: server.URL, APIKey: "test", Model: "claude-haiku-4-5", MaxTokens: 100}
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"a": map[string]any{"type": "string"}}, "required": []string{"a"}}
	_, err := chatAnthropic(context.Background(), route, Request{
		Messages: []Message{{Role: RoleUser, Blocks: []Block{
			{Type: BlockImage, MediaType: "image/png", Data: "aGVsbG8="},
			{Type: BlockText, Text: "整理一下"},
		}}},
		JSONSchema: schema,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := captured.bodies[0]
	format := body["output_config"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["schema"] == nil {
		t.Fatalf("结构化输出参数错误: %#v", format)
	}
	if _, ok := body["output_config"].(map[string]any)["effort"]; ok {
		t.Fatal("没配 effort 时不应发送（Haiku 不支持）")
	}
	content := body["messages"].([]any)[0].(map[string]any)["content"].([]any)
	image := content[0].(map[string]any)
	source := image["source"].(map[string]any)
	if image["type"] != "image" || source["type"] != "base64" || source["media_type"] != "image/png" {
		t.Fatalf("图片块错误: %#v", image)
	}
}
