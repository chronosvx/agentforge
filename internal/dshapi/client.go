package dshapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/chronosvx/agentforge/internal/security"
	"github.com/coder/websocket"
)

// Client 是 DSH Web API 客户端。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// New 创建一个 DSH API 客户端。baseURL 形如 http://127.0.0.1:17891。
func New(baseURL string) *Client {
	return NewWithToken(baseURL, "")
}

// NewWithToken 创建一个 DSH API 客户端，并在 HTTP/WebSocket 请求中附加
// Authorization: Bearer <token>。token 为空时不附加认证头。
func NewWithToken(baseURL string, token string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:17891"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{},
	}
}

// NewWithTokenAndHTTP 允许同时指定 token 和自定义 HTTP client（测试用）。
func NewWithTokenAndHTTP(baseURL, token string, hc *http.Client) *Client {
	c := NewWithToken(baseURL, token)
	if hc != nil {
		c.http = hc
	}
	return c
}

// NewWithHTTP 允许注入自定义 HTTP client（测试用）。
func NewWithHTTP(baseURL string, hc *http.Client) *Client {
	return NewWithTokenAndHTTP(baseURL, "", hc)
}

// Call 执行一个 unary RPC：POST /api/<method>。
func (c *Client) Call(ctx context.Context, method string, payload any, out any) error {
	rpcID, err := newRPCID()
	if err != nil {
		return err
	}
	body, err := json.Marshal(ClientRequest{
		Type:    "client-request",
		RPCID:   rpcID,
		Method:  method,
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("dshapi: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(APIPath+"/"+method), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dshapi: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("dshapi: post %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("dshapi: %s http %d: %s", method, resp.StatusCode, security.Redact(strings.TrimSpace(string(data))))
	}

	var full ServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
		return fmt.Errorf("dshapi: decode %s response: %w", method, err)
	}
	if full.Type != "server-response" {
		return fmt.Errorf("dshapi: unexpected response type %q", full.Type)
	}
	if full.RPCID != rpcID {
		return fmt.Errorf("dshapi: rpcId mismatch for %s: sent %s got %s", method, rpcID, full.RPCID)
	}
	if !full.Result.OK {
		if full.Result.Error != nil {
			return &RPCError{Code: full.Result.Error.Code, Message: full.Result.Error.Message, Details: full.Result.Error.Details}
		}
		return fmt.Errorf("dshapi: %s failed", method)
	}

	if out != nil && len(full.Result.Value) > 0 {
		if err := json.Unmarshal(full.Result.Value, out); err != nil {
			return fmt.Errorf("dshapi: decode %s value: %w", method, err)
		}
	}
	return nil
}

// ListSessions 返回会话列表。
func (c *Client) ListSessions(ctx context.Context) (*SessionListResponse, error) {
	var out SessionListResponse
	if err := c.Call(ctx, "session.list", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSession 创建/恢复一个会话。
func (c *Client) CreateSession(ctx context.Context, req CreateSessionRequest) (*CreateSessionResponse, error) {
	var out CreateSessionResponse
	if err := c.Call(ctx, "session.create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// History 读取会话历史。
func (c *Client) History(ctx context.Context, sessionID string, beforeSeq *int64, maxMessages *int) (*HistoryResponse, error) {
	req := HistoryRequest{SessionID: sessionID, BeforeSeq: beforeSeq, MaxMessages: maxMessages}
	var out HistoryResponse
	if err := c.Call(ctx, "session.history", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Prompt 向会话发送一条文本消息。mode 通常为 "queue"。
func (c *Client) Prompt(ctx context.Context, sessionID, text, mode string) (*PromptResponse, error) {
	if mode == "" {
		mode = "queue"
	}
	req := PromptRequest{
		SessionID: sessionID,
		Mode:      mode,
		Content: []PromptContentPart{
			{Type: "text", Text: text},
		},
	}
	var out PromptResponse
	if err := c.Call(ctx, "session.prompt", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel 取消指定会话的当前生成。
func (c *Client) Cancel(ctx context.Context, sessionID string) error {
	var out struct {
		Accepted bool `json:"accepted"`
	}
	if err := c.Call(ctx, "session.cancel", map[string]any{"sessionId": sessionID}, &out); err != nil {
		return err
	}
	return nil
}

// OpenMux 打开 events.mux WebSocket 下行流，返回帧 channel。
// 调用方负责在不再需要时 cancel ctx。
func (c *Client) OpenMux(ctx context.Context) (<-chan ServerRequest, error) {
	return c.openWS(ctx, MuxEventsPath)
}

// OpenHost 打开 events.host WebSocket 下行流。
func (c *Client) OpenHost(ctx context.Context) (<-chan ServerRequest, error) {
	return c.openWS(ctx, HostEventsPath)
}

func (c *Client) openWS(ctx context.Context, path string) (<-chan ServerRequest, error) {
	wsURL := c.wsURL(path)
	opts := &websocket.DialOptions{HTTPClient: c.http}
	if c.token != "" {
		opts.HTTPHeader = http.Header{"Authorization": []string{"Bearer " + c.token}}
	}
	conn, _, err := websocket.Dial(ctx, wsURL, opts)
	if err != nil {
		return nil, fmt.Errorf("dshapi: dial %s: %w", path, err)
	}

	ch := make(chan ServerRequest, 256)
	go func() {
		defer close(ch)
		defer conn.Close(websocket.StatusNormalClosure, "client done")
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var frame ServerRequest
			if err := json.Unmarshal(data, &frame); err != nil {
				continue
			}
			select {
			case ch <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (c *Client) wsURL(path string) string {
	u := c.baseURL + path
	if strings.HasPrefix(u, "https://") {
		return "wss://" + strings.TrimPrefix(u, "https://")
	}
	if strings.HasPrefix(u, "http://") {
		return "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u
}

func (c *Client) url(path string) string {
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return c.baseURL + path
	}
	return u.String()
}

func newRPCID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RPCError 实现 error 接口。
func (e *RPCError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %s", e.Code, security.Redact(e.Message))
}

var _ error = (*RPCError)(nil)
