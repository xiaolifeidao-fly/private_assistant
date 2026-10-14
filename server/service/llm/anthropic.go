package llm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
)

// chatAnthropic 用官方 SDK 调 Claude Messages API（流式）。
func chatAnthropic(ctx context.Context, route Route, request Request, onDelta func(Delta)) (Response, error) {
	options := []option.RequestOption{option.WithAPIKey(route.APIKey), option.WithRequestTimeout(10 * time.Minute)}
	if route.BaseURL != "" {
		options = append(options, option.WithBaseURL(route.BaseURL))
	}
	client := anthropic.NewClient(options...)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(route.Model),
		MaxTokens: int64(route.MaxTokens),
		Messages:  toAnthropicMessages(request.Messages, true),
	}
	for index, part := range request.System {
		block := anthropic.TextBlockParam{Text: part.Text}
		if part.Cache && index < 4 {
			block.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		params.System = append(params.System, block)
	}
	for _, tool := range request.Tools {
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name:        tool.Name,
			Description: anthropic.String(tool.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: tool.Properties, Required: tool.Required},
			// 流式请求里工具入参按生成进度推送；入参由 chat 层严格解析并校验，解析失败回 is_error。
			EagerInputStreaming: anthropic.Bool(true),
		}})
	}
	if route.Effort != "" {
		params.OutputConfig.Effort = anthropic.OutputConfigEffort(route.Effort)
	}
	if request.JSONSchema != nil {
		if isClaude(route) {
			params.OutputConfig.Format = anthropic.JSONOutputFormatParam{Schema: request.JSONSchema}
		} else {
			// 走 Anthropic 兼容协议的其他模型（MiniMax 等）不认结构化输出，会直接回一段文字：
			// 和 OpenAI 兼容协议一样，把 schema 写进系统提示词，解析时再容忍代码块包裹。
			schema, _ := json.Marshal(request.JSONSchema)
			params.System = append(params.System, anthropic.TextBlockParam{Text: jsonOnlyInstruction + string(schema)})
		}
	}
	if route.Temperature != nil {
		params.Temperature = anthropic.Float(*route.Temperature)
	}

	var callOptions []option.RequestOption
	if supportsDefaultFallback(route) {
		// 安全分类器拒答时由服务端按拒答类别自动换模型重跑，而不是把拒答直接丢给用户。
		callOptions = append(callOptions,
			option.WithHeaderAdd("anthropic-beta", "server-side-fallback-2026-07-01"),
			option.WithJSONSet("fallbacks", "default"))
	}

	message, err := streamAnthropic(ctx, client, params, callOptions, onDelta)
	if err != nil && isThinkingBindingError(err) {
		// 历史里的 thinking 块签名对不上（比如历史被改过）：去掉全部 thinking 块重试一次。
		// 这一轮模型拿不到之前的推理过程，但对话能继续。
		log.Printf("thinking 块校验失败，去掉 thinking 重试: %v", err)
		params.Messages = toAnthropicMessages(request.Messages, false)
		message, err = streamAnthropic(ctx, client, params, callOptions, onDelta)
	}
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return Response{}, &ProviderError{Status: apiErr.StatusCode, Message: apiErr.Error()}
		}
		return Response{}, err
	}

	response := Response{
		StopReason: string(message.StopReason),
		Provider:   KindAnthropic,
		Model:      string(message.Model),
		Usage: Usage{
			InputTokens:      int(message.Usage.InputTokens),
			OutputTokens:     int(message.Usage.OutputTokens),
			CacheReadTokens:  int(message.Usage.CacheReadInputTokens),
			CacheWriteTokens: int(message.Usage.CacheCreationInputTokens),
		},
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		// 拒答时（含流到一半被拒）丢弃已生成的部分，不进历史。
		response.Message = Message{Role: RoleAssistant, Blocks: []Block{}}
		return response, nil
	}
	out := Message{Role: RoleAssistant, Provider: KindAnthropic, Blocks: []Block{}}
	for _, block := range message.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			out.Blocks = append(out.Blocks, Block{Type: BlockText, Text: variant.Text})
		case anthropic.ToolUseBlock:
			out.Blocks = append(out.Blocks, Block{
				Type: BlockToolUse, ToolUseID: variant.ID, ToolName: variant.Name,
				Input: json.RawMessage(variant.JSON.Input.Raw()),
			})
		}
	}
	// 原生内容（含 thinking 块及其签名）整体保存，续聊时原样回传。
	if raw, err := json.Marshal(message.ToParam()); err == nil {
		out.Raw = raw
	}
	response.Message = out
	return response, nil
}

func streamAnthropic(ctx context.Context, client anthropic.Client, params anthropic.MessageNewParams, callOptions []option.RequestOption, onDelta func(Delta)) (anthropic.Message, error) {
	stream := client.Messages.NewStreaming(ctx, params, callOptions...)
	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := message.Accumulate(event); err != nil {
			return message, err
		}
		if delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.AsAny().(anthropic.TextDelta); ok && onDelta != nil {
				onDelta(Delta{Type: "text", Text: text.Text})
			}
		}
	}
	return message, stream.Err()
}

func isThinkingBindingError(err error) bool {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 400 {
		return false
	}
	text := apiErr.Error()
	return strings.Contains(text, "thinking") && strings.Contains(text, "signature")
}

// toAnthropicMessages 转换历史。useRaw=true 时 Claude 产生的 assistant 消息原样回传（含 thinking）；
// false 时只用文本和工具调用重建（不含 thinking），用于签名校验失败后的重试。
func toAnthropicMessages(messages []Message, useRaw bool) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(messages))
	for _, message := range messages {
		if useRaw && message.Role == RoleAssistant && message.Provider == KindAnthropic && len(message.Raw) > 0 {
			result = append(result, param.Override[anthropic.MessageParam](json.RawMessage(message.Raw)))
			continue
		}
		var blocks []anthropic.ContentBlockParamUnion
		for _, block := range message.Blocks {
			switch block.Type {
			case BlockText:
				if block.Text != "" {
					blocks = append(blocks, anthropic.NewTextBlock(block.Text))
				}
			case BlockImage:
				blocks = append(blocks, anthropic.NewImageBlockBase64(block.MediaType, block.Data))
			case BlockToolUse:
				input := block.Input
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropic.NewToolUseBlock(block.ToolUseID, input, block.ToolName))
			case BlockToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(block.ToolUseID, block.Text, block.IsError))
			}
		}
		if len(blocks) == 0 {
			continue
		}
		if message.Role == RoleAssistant {
			result = append(result, anthropic.NewAssistantMessage(blocks...))
		} else {
			result = append(result, anthropic.NewUserMessage(blocks...))
		}
	}
	return result
}

// supportsDefaultFallback：fallbacks:"default" 只在 Claude API 直连上可用，
// 走自建代理或第三方平台时不发，免得请求被拒。
// isClaude 判断路由是不是 Claude 模型（结构化输出、effort 等参数只有 Claude 认）。
func isClaude(route Route) bool {
	return strings.HasPrefix(strings.ToLower(route.Model), "claude")
}

func supportsDefaultFallback(route Route) bool {
	if route.BaseURL != "" && !strings.Contains(route.BaseURL, "api.anthropic.com") {
		return false
	}
	for _, prefix := range []string{"claude-opus-5", "claude-sonnet-5-5", "claude-fable-5"} {
		if strings.HasPrefix(route.Model, prefix) {
			return true
		}
	}
	return false
}
