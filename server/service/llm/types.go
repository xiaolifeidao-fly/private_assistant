// Package llm 是模型网关：按「任务」路由到提供商和模型，统一流式和工具调用的形状，
// 记录每次调用的用量。
//
// 对话历史以与提供商无关的 Message/Block 存库；但 assistant 消息会同时保存提供商
// 原生内容（Raw）。同一提供商续聊时原样回传 Raw —— Claude 的 thinking 块带签名，
// 改动或丢掉会让后续请求失败或丢失推理上下文。
package llm

import "encoding/json"

const (
	KindAnthropic = "anthropic"
	KindOpenAI    = "openai" // OpenAI 兼容协议：DeepSeek、通义千问、豆包、Ollama……

	BlockText       = "text"
	BlockImage      = "image" // 只用于单次请求（如截图抽取），不进对话历史
	BlockToolUse    = "tool_use"
	BlockToolResult = "tool_result"

	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Block 是一段内容：文本、模型发起的工具调用、或工具结果。
type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"toolUseId,omitempty"`
	ToolName  string          `json:"toolName,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	IsError   bool            `json:"isError,omitempty"`
	MediaType string          `json:"mediaType,omitempty"` // image：image/jpeg、image/png……
	Data      string          `json:"data,omitempty"`      // image：base64
}

// Message 是一条对话消息。工具结果放在 role=user 的消息里（与 Claude 的约定一致）。
type Message struct {
	Role     string          `json:"role"`
	Blocks   []Block         `json:"blocks"`
	Provider string          `json:"provider,omitempty"` // 产生 Raw 的提供商类型
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// Text 拼出消息里的全部文本块。
func (m Message) Text() string {
	text := ""
	for _, block := range m.Blocks {
		if block.Type == BlockText {
			text += block.Text
		}
	}
	return text
}

// ToolUses 返回消息里的工具调用块。
func (m Message) ToolUses() []Block {
	var uses []Block
	for _, block := range m.Blocks {
		if block.Type == BlockToolUse {
			uses = append(uses, block)
		}
	}
	return uses
}

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Properties  map[string]any `json:"properties"`
	Required    []string       `json:"required"`
}

// SystemPart 是系统提示词的一段。Cache=true 的段及其之前的内容会被缓存，
// 稳定的内容（人设、常驻记忆）放前面并打缓存标记，变化的内容（检索结果、时间）放后面。
type SystemPart struct {
	Text  string
	Cache bool
}

type Request struct {
	Task     string // 路由键，见 pa_llm_route.task
	System   []SystemPart
	Messages []Message
	Tools    []Tool
	RefKey   string // 记账用的关联键（会话、记忆）
	// JSONSchema 非空时要求模型只输出符合该 schema 的 JSON（Claude 用结构化输出保证；
	// OpenAI 兼容协议用 json_object 模式 + 提示词约束）。所有 object 都要写 additionalProperties:false。
	JSONSchema map[string]any
}

type Usage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	CacheReadTokens  int `json:"cacheReadTokens"`
	CacheWriteTokens int `json:"cacheWriteTokens"`
}

const (
	StopEnd      = "end_turn"
	StopToolUse  = "tool_use"
	StopMaxToken = "max_tokens"
	StopRefusal  = "refusal"
)

type Response struct {
	Message    Message `json:"message"`
	StopReason string  `json:"stopReason"`
	Usage      Usage   `json:"usage"`
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
}

// Delta 是流式输出的增量。Type: text（正文增量）。
type Delta struct {
	Type string
	Text string
}

// Route 是一条任务路由解析后的完整配置。
type Route struct {
	Task        string
	ProviderKey string
	Kind        string
	BaseURL     string
	APIKey      string
	Model       string
	MaxTokens   int
	Effort      string
	Temperature *float64
}
