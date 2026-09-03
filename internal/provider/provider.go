// Package provider 定义直连模型厂商的通用 Provider 抽象。
//
// 这是“路线 B”的 Provider 层：通过统一的 Chat/StreamChat 接口对接
// OpenAI 兼容协议（DeepSeek、Moonshot/Kimi、智谱、Ollama、OpenAI 等），
// 供上层在不依赖 DSH Web API 的情况下直接调用模型。
package provider

import (
	"context"
	"encoding/json"

	"github.com/chronosvx/agentforge/internal/cost"
)

// Message 是发送给模型的一条消息。
// Content 为文本内容；ToolCalls 用于 assistant 消息中的工具调用；
// ToolCallID 用于 tool 角色消息回传工具执行结果。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// Images 是消息附带的图片，由 MarshalJSON 编码进 content parts，不直接输出为独立字段。
	Images []ImagePart `json:"-"`
}

// ToolCall 是模型发起的一次工具调用。
// Name 与 Arguments 对应 OpenAI 兼容协议中 function.name / function.arguments。
type ToolCall struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// toolCallWire 是 OpenAI 兼容协议在 JSON 线上使用的嵌套结构。
type toolCallWire struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MarshalJSON 将 ToolCall 编码为 OpenAI 兼容的 tool_calls 元素。
func (t ToolCall) MarshalJSON() ([]byte, error) {
	typ := t.Type
	if typ == "" {
		typ = "function"
	}
	var wire toolCallWire
	wire.ID = t.ID
	wire.Type = typ
	wire.Function.Name = t.Name
	wire.Function.Arguments = t.Arguments
	return json.Marshal(wire)
}

// UnmarshalJSON 解析 OpenAI 兼容的 tool_calls 元素。
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	var wire toolCallWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	t.ID = wire.ID
	t.Type = wire.Type
	if t.Type == "" {
		t.Type = "function"
	}
	t.Name = wire.Function.Name
	t.Arguments = wire.Function.Arguments
	return nil
}

// Usage 记录一次请求的 token 用量。
type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

// ToolFunction 描述一个可被模型调用的函数。
type ToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Tool 是 OpenAI 兼容协议中的 tools 元素。
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ChatRequest 是发给 Provider 的聊天请求。
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []Tool    `json:"tools,omitempty"`
	Stream      bool      `json:"stream"`
	Temperature *float64  `json:"temperature,omitempty"`

	// Headers 是随请求发送的额外 HTTP header，不进入 JSON 请求体。
	// 可用于携带请求 ID、幂等键、断点续传偏移等 OpenAI 兼容扩展字段。
	Headers map[string]string `json:"-"`
}

// ChatResponse 是 Provider 返回的非流式响应。
type ChatResponse struct {
	Content      string     `json:"content"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
	Usage        *Usage     `json:"usage,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`
}

// StreamEventType 是流式事件类型。
type StreamEventType string

const (
	StreamEventText     StreamEventType = "text"
	StreamEventToolCall StreamEventType = "tool_call"
	StreamEventUsage    StreamEventType = "usage"
	StreamEventDone     StreamEventType = "done"
	StreamEventError    StreamEventType = "error"
)

// StreamEvent 是流式响应中的一条事件。
type StreamEvent struct {
	Type     StreamEventType `json:"type"`
	Text     string          `json:"text,omitempty"`
	ToolCall *ToolCall       `json:"tool_call,omitempty"`
	Usage    *Usage          `json:"usage,omitempty"`
	Err      error           `json:"-"`
}

// Provider 是直连模型厂商的统一抽象。
type Provider interface {
	// Chat 发送非流式聊天请求并返回完整响应。
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// StreamChat 发送流式聊天请求并返回事件通道；通道在流结束后由实现方关闭。
	StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
	// Name 返回厂商/Provider 名称，例如 "openai"、"deepseek"。
	Name() string
	// Model 返回当前配置的模型名。
	Model() string
	// CostEstimator 返回该 Provider 使用的单价估算器。
	CostEstimator() *cost.Estimator
}
