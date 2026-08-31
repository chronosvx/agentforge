// Package dshapi 实现 DSH Web 载体（/api HTTP + SSE）的 Go 客户端。
//
// 这是 agentforge 实际使用的浏览器传输协议：
//   - unary RPC：POST /api/<method>，信封为 client-request / server-response
//   - 下行事件：GET /api/events.mux 与 /api/events.host，SSE data: 帧
package dshapi

import (
	"encoding/json"
	"time"
)

// Wire 常量
const (
	APIPath        = "/api"
	MuxEventsPath  = "/api/events.mux"
	HostEventsPath = "/api/events.host"
)

// ClientRequest 是 C→S 的 unary 请求信封。
type ClientRequest struct {
	Type    string `json:"type"`
	RPCID   string `json:"rpcId"`
	Method  string `json:"method"`
	Payload any    `json:"payload"`
}

// ServerResponse 是 S→C 的 unary 响应信封。
type ServerResponse struct {
	Type   string    `json:"type"`
	RPCID  string    `json:"rpcId"`
	Result RPCResult `json:"result"`
}

// RPCResult 是业务成功/失败结果。
type RPCResult struct {
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error *RPCError       `json:"error,omitempty"`
}

// RPCError 是业务错误体。
type RPCError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details"`
}

// ServerRequest 是 S→C 的下行信封（SSE data 内容）。
type ServerRequest struct {
	Type    string          `json:"type"`
	RPCID   string          `json:"rpcId"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

// SessionSummary 对应 session.list 返回行。
type SessionSummary struct {
	SessionID   string `json:"sessionId"`
	UpdatedAt   int64  `json:"updatedAt"`
	Running     bool   `json:"running"`
	Blank       bool   `json:"blank"`
	Cwd         string `json:"cwd,omitempty"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// SessionListResponse 对应 session.list 响应 value。
type SessionListResponse struct {
	Items []SessionSummary `json:"items"`
}

// CreateSessionRequest 对应 session.create 请求 payload。
type CreateSessionRequest struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// CreateSessionResponse 对应 session.create 响应 value。
type CreateSessionResponse struct {
	SessionID   string `json:"sessionId"`
	AgentPreset string `json:"agentPreset,omitempty"`
}

// HistoryRequest 对应 session.history 请求 payload。
type HistoryRequest struct {
	SessionID   string `json:"sessionId"`
	BeforeSeq   *int64 `json:"beforeSeq,omitempty"`
	MaxMessages *int   `json:"maxMessages,omitempty"`
}

// HistoryEntry 是 session.history 中的一项。
type HistoryEntry struct {
	Event SessionEvent `json:"event"`
}

// SessionEvent 是事件信封：严格 envelope + 宽松 data。
type SessionEvent struct {
	Type            string  `json:"type"`
	Seq             int64   `json:"seq"`
	Time            int64   `json:"time"`
	Data            any     `json:"data"`
	SourceEventSeqs []int64 `json:"sourceEventSeqs,omitempty"`
}

// HistoryResponse 对应 session.history 响应 value。
type HistoryResponse struct {
	Events  []HistoryEntry `json:"events"`
	HasMore bool           `json:"hasMore"`
}

// PromptRequest 对应 session.prompt 请求 payload。
type PromptRequest struct {
	SessionID      string              `json:"sessionId"`
	Mode           string              `json:"mode"` // queue 或 steer
	Content        []PromptContentPart `json:"content"`
	ClientTimeZone string              `json:"clientTimeZone,omitempty"`
}

// PromptContentPart 是 prompt 内容块。
type PromptContentPart struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
	Data      string `json:"data,omitempty"`
	Name      string `json:"name,omitempty"`
}

// PromptResponse 对应 session.prompt 响应 value。
type PromptResponse struct {
	Accepted bool `json:"accepted"`
}

// MuxFrame 是 events.mux 的下行帧。
type MuxFrame struct {
	Type      string        `json:"type"`
	SessionID string        `json:"sessionId,omitempty"`
	Event     *SessionEvent `json:"event,omitempty"`
	LastSeq   int64         `json:"lastSeq,omitempty"`
	Error     *RPCError     `json:"error,omitempty"`
}

// HostFrame 是 events.host 的下行帧。
type HostFrame struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId,omitempty"`
	Running   *bool     `json:"running,omitempty"`
	Error     *RPCError `json:"error,omitempty"`
}

// SSEStream 是下行事件流。
type SSEStream struct {
	Frames <-chan ServerRequest
	Err    error
	// 内部字段
	lastSeq   int64
	startedAt time.Time
}
