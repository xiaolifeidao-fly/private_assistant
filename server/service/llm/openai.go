package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAI 兼容协议（/chat/completions、/embeddings）。国内模型（DeepSeek、通义、豆包）
// 和本地 Ollama 都走这个协议，作为 Claude 之外的通道和向量化通道。

var httpClient = &http.Client{Timeout: 10 * time.Minute}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// jsonOnlyInstruction 用在不支持结构化输出的模型上，后面接 JSON Schema。
const jsonOnlyInstruction = "\n\n只输出一个 JSON 对象，不要输出任何解释、前言或 Markdown。字符串里如果要用引号，用中文引号「」，或把双引号转义成 \\\"。严格符合这个 JSON Schema：\n"

func chatOpenAI(ctx context.Context, route Route, request Request, onDelta func(Delta)) (Response, error) {
	var system strings.Builder
	for _, part := range request.System {
		system.WriteString(part.Text)
		system.WriteString("\n\n")
	}
	messages := []openAIMessage{{Role: "system", Content: strings.TrimSpace(system.String())}}
	for _, message := range request.Messages {
		messages = append(messages, toOpenAIMessages(message)...)
	}
	body := map[string]any{
		"model":          route.Model,
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if route.MaxTokens > 0 {
		body["max_tokens"] = route.MaxTokens
	}
	if route.Temperature != nil {
		body["temperature"] = *route.Temperature
	}
	if request.JSONSchema != nil {
		// 不是所有兼容服务都支持 json_schema，统一用 json_object，schema 写进系统提示词。
		body["response_format"] = map[string]any{"type": "json_object"}
		schema, _ := json.Marshal(request.JSONSchema)
		messages[0].Content = fmt.Sprint(messages[0].Content) + jsonOnlyInstruction + string(schema)
		body["messages"] = messages
	}
	if len(request.Tools) > 0 {
		var tools []map[string]any
		for _, tool := range request.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{
				"name": tool.Name, "description": tool.Description,
				"parameters": map[string]any{"type": "object", "properties": tool.Properties, "required": tool.Required},
			}})
		}
		body["tools"] = tools
	}

	resp, err := postJSON(ctx, route, "/chat/completions", body)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	var text strings.Builder
	calls := map[int]*openAIToolCall{}
	finish := ""
	usage := Usage{}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string           `json:"content"`
					ToolCalls []openAIToolCall `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage != nil {
			usage.InputTokens, usage.OutputTokens = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				text.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(Delta{Type: "text", Text: choice.Delta.Content})
				}
			}
			for _, call := range choice.Delta.ToolCalls {
				current, ok := calls[call.Index]
				if !ok {
					copied := call
					copied.Function.Arguments = ""
					current = &copied
					calls[call.Index] = current
				}
				if call.ID != "" {
					current.ID = call.ID
				}
				if call.Function.Name != "" {
					current.Function.Name = call.Function.Name
				}
				current.Function.Arguments += call.Function.Arguments
			}
			if choice.FinishReason != nil {
				finish = *choice.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	out := Message{Role: RoleAssistant, Blocks: []Block{}}
	if text.Len() > 0 {
		out.Blocks = append(out.Blocks, Block{Type: BlockText, Text: text.String()})
	}
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		call := calls[index]
		arguments := call.Function.Arguments
		if strings.TrimSpace(arguments) == "" {
			arguments = "{}"
		}
		out.Blocks = append(out.Blocks, Block{Type: BlockToolUse, ToolUseID: call.ID, ToolName: call.Function.Name, Input: json.RawMessage(arguments)})
	}
	stop := StopEnd
	switch {
	case len(calls) > 0:
		stop = StopToolUse
	case finish == "length":
		stop = StopMaxToken
	case finish == "content_filter":
		stop = StopRefusal
	}
	return Response{Message: out, StopReason: stop, Usage: usage, Provider: KindOpenAI, Model: route.Model}, nil
}

func toOpenAIMessages(message Message) []openAIMessage {
	if message.Role == RoleAssistant {
		out := openAIMessage{Role: "assistant", Content: message.Text()}
		for index, use := range message.ToolUses() {
			call := openAIToolCall{Index: index, ID: use.ToolUseID, Type: "function"}
			call.Function.Name = use.ToolName
			call.Function.Arguments = string(use.Input)
			out.ToolCalls = append(out.ToolCalls, call)
		}
		return []openAIMessage{out}
	}
	var result []openAIMessage
	var text strings.Builder
	var images []map[string]any
	for _, block := range message.Blocks {
		switch block.Type {
		case BlockImage:
			images = append(images, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + block.MediaType + ";base64," + block.Data}})
		case BlockToolResult:
			content := block.Text
			if block.IsError {
				content = "ERROR: " + content
			}
			result = append(result, openAIMessage{Role: "tool", ToolCallID: block.ToolUseID, Content: content})
		case BlockText:
			text.WriteString(block.Text)
		}
	}
	if len(images) > 0 {
		parts := append(images, map[string]any{"type": "text", "text": text.String()})
		result = append(result, openAIMessage{Role: "user", Content: parts})
	} else if text.Len() > 0 {
		result = append(result, openAIMessage{Role: "user", Content: text.String()})
	}
	return result
}

// embedOpenAI 调 /embeddings，返回与输入同序的向量（已归一化，余弦相似度 = 点积）。
func embedOpenAI(ctx context.Context, route Route, texts []string) ([][]float32, error) {
	resp, err := postJSON(ctx, route, "/embeddings", map[string]any{"model": route.Model, "input": texts, "encoding_format": "float"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Data) != len(texts) {
		return nil, fmt.Errorf("embedding 返回 %d 条，期望 %d 条", len(payload.Data), len(texts))
	}
	result := make([][]float32, len(texts))
	for _, item := range payload.Data {
		if item.Index < 0 || item.Index >= len(texts) {
			return nil, errors.New("embedding 返回的 index 越界")
		}
		result[item.Index] = Normalize(item.Embedding)
	}
	return result, nil
}

func postJSON(ctx context.Context, route Route, path string, body any) (*http.Response, error) {
	if route.BaseURL == "" {
		return nil, fmt.Errorf("提供商 %s 没有配置 base_url", route.ProviderKey)
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(route.BaseURL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	if route.APIKey != "" {
		request.Header.Set("Authorization", "Bearer "+route.APIKey)
	}
	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return nil, &ProviderError{Status: resp.StatusCode, Message: string(detail)}
	}
	return resp, nil
}

// ProviderError 是提供商返回的非 2xx 错误。
type ProviderError struct {
	Status  int
	Message string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("模型服务返回 %d: %s", e.Status, e.Message)
}

// Normalize 把向量缩放到单位长度。
func Normalize(vector []float32) []float32 {
	var sum float64
	for _, value := range vector {
		sum += float64(value) * float64(value)
	}
	if sum == 0 {
		return vector
	}
	norm := float32(1 / math.Sqrt(sum))
	result := make([]float32, len(vector))
	for index, value := range vector {
		result[index] = value * norm
	}
	return result
}
