package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/cost"
)

// ProviderOpenAI 是 OpenAI 兼容 Provider 的固定名称。
const ProviderOpenAI = "openai"

// OpenAIConfig 是 OpenAI 兼容服务的连接配置。
//
// BaseURL 可直接配置为各厂商的兼容根地址，例如：
//   - OpenAI:      https://api.openai.com/v1
//   - DeepSeek:    https://api.deepseek.com
//   - Moonshot:    https://api.moonshot.cn/v1
//   - 智谱:        https://open.bigmodel.cn/api/paas/v4
//   - Ollama:      http://localhost:11434/v1
type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
	// ProxyURL 可选；非空时所有 HTTP 请求都通过该代理发送。
	// 支持 http://、https://、socks5:// 与 socks5h:// 地址，优先使用标准库 http.ProxyURL。
	ProxyURL string
	// IdempotencyKey 可选；设置后每个请求都会附带该 Idempotency-Key header，
	// 用于支持幂等重试的厂商减少重复扣费。
	IdempotencyKey string
}

// openAIClient 是 OpenAI 兼容协议的 Provider 实现。
type openAIClient struct {
	baseURL          string
	apiKey           string
	model            string
	idempotencyKey   string
	httpClient       *http.Client
	streamHTTPClient *http.Client
	estimator        *cost.Estimator
}

// NewOpenAI 创建 OpenAI 兼容 Provider。
func NewOpenAI(cfg OpenAIConfig) Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &openAIClient{
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:           cfg.APIKey,
		model:            cfg.Model,
		idempotencyKey:   cfg.IdempotencyKey,
		httpClient:       newHTTPClient(cfg.Timeout, cfg.ProxyURL),
		streamHTTPClient: newStreamHTTPClient(cfg.ProxyURL),
		estimator:        cost.NewEstimator(),
	}
}

// Name 返回 Provider 名称。
func (c *openAIClient) Name() string {
	return ProviderOpenAI
}

// Model 返回配置的模型名。
func (c *openAIClient) Model() string {
	return c.model
}

// CostEstimator 返回默认单价估算器。
func (c *openAIClient) CostEstimator() *cost.Estimator {
	return c.estimator
}

// Chat 调用 POST /chat/completions 获取非流式响应。
func (c *openAIClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(c.buildChatRequest(req, false))
	if err != nil {
		return nil, fmt.Errorf("provider openai: marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("provider openai: create request: %w", err)
	}
	c.setHeaders(httpReq, false, req)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider openai: send chat request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("provider openai: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, MapHTTPError(resp.StatusCode, string(data))
	}

	var wire openAIChatResponse
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, fmt.Errorf("provider openai: decode response: %w", err)
	}
	if len(wire.Choices) == 0 {
		return nil, errors.New("provider openai: response has no choices")
	}

	choice := wire.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		Usage:        wire.Usage,
		FinishReason: choice.FinishReason,
	}, nil
}

// StreamChat 调用 POST /chat/completions 并解析 SSE 流。
func (c *openAIClient) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	body, err := json.Marshal(c.buildChatRequest(req, true))
	if err != nil {
		return nil, fmt.Errorf("provider openai: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("provider openai: create stream request: %w", err)
	}
	c.setHeaders(httpReq, true, req)

	httpClient := c.streamHTTPClient
	if httpClient == nil {
		httpClient = c.httpClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("provider openai: send stream request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("provider openai: read error response: %w", readErr)
		}
		return nil, MapHTTPError(resp.StatusCode, string(data))
	}

	ch := make(chan StreamEvent, 64)
	go c.readStream(ctx, resp, ch)
	return ch, nil
}

// ResumeStream 从 lastOffset 处继续流式请求。
// 它通过 X-Resume-Offset header 向服务端传递偏移量；若服务端不支持该扩展，
// 通常会在 HTTP 响应中返回错误，由调用方按实际错误处理。
func (c *openAIClient) ResumeStream(ctx context.Context, req ChatRequest, lastOffset int) (<-chan StreamEvent, error) {
	req = WithResumeOffset(req, lastOffset)
	return c.StreamChat(ctx, req)
}

// endpoint 返回 chat/completions 完整 URL。
func (c *openAIClient) endpoint() string {
	if strings.HasSuffix(c.baseURL, "/chat/completions") {
		return c.baseURL
	}
	return c.baseURL + "/chat/completions"
}

func (c *openAIClient) setHeaders(req *http.Request, stream bool, chatReq ChatRequest) {
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// 先应用调用方显式传入的自定义 header，再补默认值，避免覆盖业务方设置。
	for k, v := range chatReq.Headers {
		if k != "" {
			req.Header.Set(k, v)
		}
	}

	// 自动生成并附带请求 ID，便于链路追踪与幂等去重。
	// 配置了 IdempotencyKey 时优先复用它作为请求 ID，便于服务端关联幂等请求。
	if req.Header.Get(RequestIDHeader) == "" {
		if c.idempotencyKey != "" {
			req.Header.Set(RequestIDHeader, c.idempotencyKey)
		} else {
			req.Header.Set(RequestIDHeader, RequestID())
		}
	}
	// 可选配置的 Idempotency-Key。
	if c.idempotencyKey != "" && req.Header.Get(IdempotencyKeyHeader) == "" {
		req.Header.Set(IdempotencyKeyHeader, c.idempotencyKey)
	}
}

func (c *openAIClient) buildChatRequest(req ChatRequest, stream bool) openAIChatRequest {
	model := req.Model
	if model == "" {
		model = c.model
	}
	tools := make([]openAITool, 0, len(req.Tools))
	for _, t := range req.Tools {
		typ := t.Type
		if typ == "" {
			typ = "function"
		}
		tools = append(tools, openAITool{Type: typ, Function: t.Function})
	}
	return openAIChatRequest{
		Model:       model,
		Messages:    req.Messages,
		Tools:       tools,
		Stream:      stream,
		Temperature: req.Temperature,
	}
}

var (
	sseDataPrefix = []byte("data:")
	sseDone       = []byte("[DONE]")
)

// readStream 在后台读取 SSE 并发送事件。
func (c *openAIClient) readStream(ctx context.Context, resp *http.Response, ch chan<- StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	toolAcc := make(map[int]*streamToolCall)

	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.HasPrefix(line, sseDataPrefix) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, sseDataPrefix))
		if len(data) == 0 {
			continue
		}
		if bytes.Equal(data, sseDone) {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventError, Err: fmt.Errorf("provider openai: decode stream chunk: %w", err)}) {
				return
			}
			return
		}

		if chunk.Usage != nil {
			if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventUsage, Usage: chunk.Usage}) {
				return
			}
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventText, Text: choice.Delta.Content}) {
				return
			}
		}

		for _, tc := range choice.Delta.ToolCalls {
			acc := toolAcc[tc.Index]
			if acc == nil {
				acc = &streamToolCall{}
				toolAcc[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.Arguments += tc.Function.Arguments
			}
			call := &ToolCall{
				ID:        acc.ID,
				Type:      "function",
				Name:      acc.Name,
				Arguments: acc.Arguments,
			}
			if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventToolCall, ToolCall: call}) {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventError, Err: fmt.Errorf("provider openai: read stream: %w", err)}) {
			return
		}
		return
	}

	if !sendStreamEvent(ctx, ch, StreamEvent{Type: StreamEventDone}) {
		return
	}
}

// streamToolCall 是流式工具调用的累积状态。
type streamToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// openAIChatRequest 是对 OpenAI /chat/completions 的请求体。
type openAIChatRequest struct {
	Model       string       `json:"model"`
	Messages    []Message    `json:"messages"`
	Tools       []openAITool `json:"tools,omitempty"`
	Stream      bool         `json:"stream"`
	Temperature *float64     `json:"temperature,omitempty"`
}

// openAITool 是请求体中的 tools 元素。
type openAITool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// openAIChatResponse 是非流式响应体。
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

// openAIStreamChunk 是 SSE 中的单个 data 块。
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role         string                 `json:"role"`
			Content      string                 `json:"content"`
			ToolCalls    []openAIStreamToolCall `json:"tool_calls"`
			FinishReason string                 `json:"finish_reason"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage,omitempty"`
}

// openAIStreamToolCall 是流式 delta 中的 tool_calls 元素。
type openAIStreamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func sendStreamEvent(ctx context.Context, ch chan<- StreamEvent, ev StreamEvent) bool {
	select {
	case ch <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}

// UnmarshalJSON 解析 OpenAI 兼容的 usage 字段，并兼容常见厂商别名。
func (u *Usage) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	u.InputTokens = numberFromRaw(raw, "prompt_tokens", "input_tokens")
	u.OutputTokens = numberFromRaw(raw, "completion_tokens", "output_tokens")
	u.CacheReadTokens = numberFromRaw(raw, "cache_read_tokens", "prompt_cache_hit_tokens", "cache_hit_tokens")
	u.CacheWriteTokens = numberFromRaw(raw, "cache_write_tokens", "prompt_cache_miss_tokens", "cache_miss_tokens")

	if details, ok := raw["prompt_tokens_details"].(map[string]any); ok {
		if v, ok := details["cached_tokens"]; ok {
			u.CacheReadTokens = numberFromValue(v)
		}
		if v, ok := details["cache_creation_input_tokens"]; ok {
			u.CacheWriteTokens = numberFromValue(v)
		}
	}
	return nil
}

func numberFromRaw(raw map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := raw[key]; ok {
			return numberFromValue(v)
		}
	}
	return 0
}

func numberFromValue(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n <= uint64(^uint64(0)>>1) {
			return int64(n)
		}
		return 0
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
		if f, err := n.Float64(); err == nil {
			return int64(f)
		}
		return 0
	default:
		return 0
	}
}
