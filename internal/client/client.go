// Package client 实现 DSH TUI Channel (tui.dsh/v1alpha1) 的 WebSocket 客户端。
//
// 当前实现保持最小可用：使用 github.com/coder/websocket 传输 JSON 帧，
// 并尽量兼容服务端不同的信封形式（operation/value 嵌套或顶层字段）。
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/chronosvx/agentforge/internal/security"
	"github.com/coder/websocket"
)

// DefaultEndpoint 是 DSH TUI Channel WebSocket 默认地址。
const DefaultEndpoint = "ws://127.0.0.1:18891/agentforge/v1/connection"

// ErrConnectionClosed 表示底层 WebSocket 连接已关闭。
var ErrConnectionClosed = errors.New("dsh tui channel connection closed")

// Message 是聊天消息的简化表示。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Snapshot 是 Channel 的完整状态快照。
type Snapshot struct {
	WireRevision int            `json:"wireRevision"`
	ChannelID    string         `json:"channelId"`
	Version      int64          `json:"version"`
	State        map[string]any `json:"state"`
}

// Event 是订阅流中的增量事件。
type Event struct {
	Operation string         `json:"operation"`
	ChannelID string         `json:"channelId,omitempty"`
	Version   int64          `json:"version"`
	State     map[string]any `json:"state,omitempty"`
	Value     any            `json:"value,omitempty"`
}

// serverMessage 是服务端消息的宽松结构。未知字段会被忽略，便于后续适配。
type serverMessage struct {
	Operation    string          `json:"operation"`
	Type         string          `json:"type"`
	ChannelID    string          `json:"channelId"`
	Version      *int64          `json:"version"`
	State        json.RawMessage `json:"state"`
	Value        json.RawMessage `json:"value"`
	ValueDefined *bool           `json:"valueDefined"`
	Closed       *bool           `json:"closed"`
	WireRevision int             `json:"wireRevision"`
	Code         string          `json:"code"`
	Message      string          `json:"message"`
}

// Client 是 DSH TUI Channel 的 WebSocket 客户端。
type Client struct {
	conn *websocket.Conn

	// sendMu 保护所有写操作；callMu 串行化“请求-响应”型调用，
	// 因为当前协议帧没有 request id，串行化可以避免响应错配。
	sendMu sync.Mutex
	callMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan serverMessage

	subsMu sync.Mutex
	subs   map[string]map[chan Event]struct{}

	done      chan struct{}
	closeOnce sync.Once
}

// Dial 连接指定的 WebSocket endpoint。endpoint 为空时使用 DefaultEndpoint。
func Dial(ctx context.Context, endpoint string) (*Client, error) {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	conn, _, err := websocket.Dial(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("dial dsh tui channel: %w", err)
	}
	c := &Client{
		conn:    conn,
		pending: make(map[string]chan serverMessage),
		subs:    make(map[string]map[chan Event]struct{}),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

// Open 打开（或恢复）一个 Channel，返回完整 Snapshot。
func (c *Client) Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*Snapshot, error) {
	if options == nil {
		options = map[string]any{}
	}
	payload := map[string]any{
		"operation": "open",
		"value": map[string]any{
			"workspace": workspace,
			"sessionId": sessionID,
			"options":   options,
		},
	}
	resp, err := c.roundTrip(ctx, "open", payload)
	if err != nil {
		return nil, err
	}
	snap, err := parseSnapshot(resp)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return snap, nil
}

// Subscribe 订阅指定 Channel 的事件流。返回的 channel 在连接关闭或
// Channel 被 Close 后会被关闭。
func (c *Client) Subscribe(ctx context.Context, channelID string, after int64) (<-chan Event, error) {
	ch := make(chan Event, 256)
	c.subsMu.Lock()
	if c.subs[channelID] == nil {
		c.subs[channelID] = make(map[chan Event]struct{})
	}
	c.subs[channelID][ch] = struct{}{}
	c.subsMu.Unlock()

	payload := map[string]any{
		"operation": "subscribe",
		"value": map[string]any{
			"channelId":    channelID,
			"afterVersion": after,
		},
	}
	resp, err := c.roundTrip(ctx, "subscribe", payload)
	if err != nil {
		c.unsubscribe(channelID, ch)
		return nil, err
	}

	// 若 subscribe 响应本身是快照，把它作为首个事件推给订阅者。
	if ev, ok := eventFromServerMessage(resp); ok {
		select {
		case ch <- ev:
		default:
		}
	}
	return ch, nil
}

// Invoke 在 Channel 上调用一个方法，并等待最终返回值。
func (c *Client) Invoke(ctx context.Context, channelID, method string, args []any) (any, error) {
	if args == nil {
		args = []any{}
	}
	payload := map[string]any{
		"operation": "invoke",
		"value": map[string]any{
			"channelId": channelID,
			"method":    method,
			"arguments": args,
		},
	}
	resp, err := c.roundTrip(ctx, "invoke", payload)
	if err != nil {
		return nil, err
	}
	return parseInvokeResult(resp)
}

// Close 关闭指定 Channel，并关闭其本地订阅流。
func (c *Client) Close(ctx context.Context, channelID string) error {
	payload := map[string]any{
		"operation": "close",
		"value": map[string]any{
			"channelId": channelID,
		},
	}
	resp, err := c.roundTrip(ctx, "close", payload)
	if err != nil {
		return err
	}
	c.removeSubscribers(channelID)
	return parseCloseResult(resp)
}

// CloseConn 关闭底层 WebSocket 连接，并等待读取循环退出。
func (c *Client) CloseConn() error {
	var err error
	c.closeOnce.Do(func() {
		err = c.conn.Close(websocket.StatusNormalClosure, "client close")
		<-c.done
	})
	return err
}

// readLoop 持续读取服务端消息，并分发给等待响应的调用或订阅者。
func (c *Client) readLoop() {
	defer func() {
		c.closeSubs()
		close(c.done)
	}()

	for {
		_, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		var msg serverMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			// 非 JSON 帧直接忽略，保持连接可用。
			continue
		}
		c.route(msg)
	}
}

// route 根据 operation 分发服务端消息。
func (c *Client) route(msg serverMessage) {
	op := msg.Operation
	if op == "" {
		op = msg.Type
	}

	switch op {
	case "open", "invoke", "close":
		c.deliverPending(op, msg)
	case "subscribe":
		// 优先交给同步等待 subscribe 响应的调用；没有等待者时按事件广播。
		if !c.deliverPending("subscribe", msg) {
			c.broadcastEvent(msg)
		}
	case "error":
		// 错误帧没有 request id；当前 roundTrip 串行化，直接交给所有等待者。
		c.deliverPendingAll(msg)
	case "event", "snapshot":
		if op == "snapshot" {
			// 兼容某些服务端用 snapshot 作为 open/subscribe 的响应。
			if c.deliverPending("open", msg) || c.deliverPending("subscribe", msg) {
				return
			}
		}
		c.broadcastEvent(msg)
	default:
		// 未知帧只要带版本/状态，就按事件广播，方便兼容后续服务端实现。
		if msg.Version != nil || len(msg.State) > 0 || len(msg.Value) > 0 {
			c.broadcastEvent(msg)
		}
	}
}

// roundTrip 发送一个请求并等待同名 operation 的响应。
// 目前通过 callMu 串行化，避免无 request id 时的响应错配。
func (c *Client) roundTrip(ctx context.Context, op string, payload any) (serverMessage, error) {
	c.callMu.Lock()
	defer c.callMu.Unlock()

	ch := c.registerPending(op)
	if err := c.writeJSON(ctx, payload); err != nil {
		c.removePending(op, ch)
		return serverMessage{}, err
	}

	select {
	case msg := <-ch:
		if msg.Operation == "error" || msg.Type == "error" {
			c.removePending(op, ch)
			return serverMessage{}, parseServerError(msg)
		}
		return msg, nil
	case <-ctx.Done():
		c.removePending(op, ch)
		return serverMessage{}, ctx.Err()
	case <-c.done:
		c.removePending(op, ch)
		return serverMessage{}, ErrConnectionClosed
	}
}

func (c *Client) registerPending(op string) chan serverMessage {
	ch := make(chan serverMessage, 1)
	c.pendingMu.Lock()
	c.pending[op] = ch
	c.pendingMu.Unlock()
	return ch
}

func (c *Client) removePending(op string, ch chan serverMessage) {
	c.pendingMu.Lock()
	if cur, ok := c.pending[op]; ok && cur == ch {
		delete(c.pending, op)
	}
	c.pendingMu.Unlock()
}

func (c *Client) deliverPending(op string, msg serverMessage) bool {
	c.pendingMu.Lock()
	ch, ok := c.pending[op]
	if ok {
		delete(c.pending, op)
	}
	c.pendingMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- msg:
	default:
	}
	return true
}

// deliverPendingAll 将消息交给所有等待中的 roundTrip。当前实现串行化请求，
// 正常情况下 pending 里最多只有一个元素；错误帧没有 request id 时使用它。
func (c *Client) deliverPendingAll(msg serverMessage) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for op, ch := range c.pending {
		delete(c.pending, op)
		select {
		case ch <- msg:
		default:
		}
	}
}

func (c *Client) writeJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if err := c.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("write websocket message: %w", err)
	}
	return nil
}

func (c *Client) broadcastEvent(msg serverMessage) {
	ev, ok := eventFromServerMessage(msg)
	if !ok {
		return
	}
	channelID := channelIDFromMessage(msg)

	c.subsMu.Lock()
	defer c.subsMu.Unlock()

	if channelID != "" {
		for ch := range c.subs[channelID] {
			// 非阻塞投递：慢消费者丢事件比阻塞读循环/死锁更安全。
			select {
			case ch <- ev:
			default:
			}
		}
		return
	}

	// 没有 channelId 时广播给所有订阅者。
	for _, m := range c.subs {
		for ch := range m {
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

func (c *Client) unsubscribe(channelID string, ch chan Event) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	if m, ok := c.subs[channelID]; ok {
		if _, ok := m[ch]; ok {
			delete(m, ch)
			close(ch)
			if len(m) == 0 {
				delete(c.subs, channelID)
			}
		}
	}
}

func (c *Client) removeSubscribers(channelID string) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	if m, ok := c.subs[channelID]; ok {
		for ch := range m {
			close(ch)
		}
		delete(c.subs, channelID)
	}
}

func (c *Client) closeSubs() {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for _, m := range c.subs {
		for ch := range m {
			close(ch)
		}
	}
	c.subs = make(map[string]map[chan Event]struct{})
}

// parseSnapshot 从服务端消息中解析 Snapshot，兼容顶层字段和 value 嵌套两种形式。
func parseSnapshot(msg serverMessage) (*Snapshot, error) {
	if msg.ChannelID != "" || msg.Version != nil || msg.WireRevision != 0 || len(msg.State) > 0 {
		snap := &Snapshot{
			WireRevision: msg.WireRevision,
			ChannelID:    msg.ChannelID,
			State:        map[string]any{},
		}
		if msg.Version != nil {
			snap.Version = *msg.Version
		}
		if len(msg.State) > 0 && !isJSONNull(msg.State) {
			if err := json.Unmarshal(msg.State, &snap.State); err != nil {
				return nil, fmt.Errorf("parse snapshot state: %w", err)
			}
		}
		if snap.State == nil {
			snap.State = map[string]any{}
		}
		return snap, nil
	}

	if len(msg.Value) > 0 && !isJSONNull(msg.Value) {
		var v struct {
			WireRevision int            `json:"wireRevision"`
			ChannelID    string         `json:"channelId"`
			Version      int64          `json:"version"`
			State        map[string]any `json:"state"`
		}
		if err := json.Unmarshal(msg.Value, &v); err != nil {
			return nil, fmt.Errorf("parse snapshot value: %w", err)
		}
		if v.ChannelID != "" || v.Version != 0 || v.WireRevision != 0 || v.State != nil {
			if v.State == nil {
				v.State = map[string]any{}
			}
			return &Snapshot{
				WireRevision: v.WireRevision,
				ChannelID:    v.ChannelID,
				Version:      v.Version,
				State:        v.State,
			}, nil
		}
	}

	return nil, errors.New("server message does not contain a snapshot")
}

// parseInvokeResult 解析 invoke 响应中的 value。
func parseInvokeResult(msg serverMessage) (any, error) {
	if len(msg.Value) > 0 && !isJSONNull(msg.Value) {
		var out struct {
			Value        json.RawMessage `json:"value"`
			ValueDefined *bool           `json:"valueDefined"`
			Snapshot     json.RawMessage `json:"snapshot"`
		}
		if err := json.Unmarshal(msg.Value, &out); err == nil {
			if out.ValueDefined != nil && !*out.ValueDefined {
				return nil, nil
			}
			if len(out.Value) > 0 && !isJSONNull(out.Value) {
				var v any
				if err := json.Unmarshal(out.Value, &v); err == nil {
					return v, nil
				}
			}
			return nil, nil
		}

		// 非标准输出对象时，直接把 value 当作返回值。
		var v any
		if err := json.Unmarshal(msg.Value, &v); err == nil {
			return v, nil
		}
	}

	if msg.ValueDefined != nil && !*msg.ValueDefined {
		return nil, nil
	}
	return nil, nil
}

// parseServerError 从错误帧中提取错误信息。
func parseServerError(msg serverMessage) error {
	code := msg.Code
	message := msg.Message
	if len(msg.Value) > 0 && !isJSONNull(msg.Value) {
		var v struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(msg.Value, &v); err == nil {
			if v.Code != "" {
				code = v.Code
			}
			if v.Message != "" {
				message = v.Message
			}
		}
	}
	if code == "" {
		code = "ERROR"
	}
	if message != "" {
		return fmt.Errorf("dsh tui channel error %s: %s", code, security.Redact(message))
	}
	return fmt.Errorf("dsh tui channel error: %s", code)
}

// parseCloseResult 校验 close 响应。
func parseCloseResult(msg serverMessage) error {
	if msg.Closed != nil {
		if *msg.Closed {
			return nil
		}
		return errors.New("server returned closed=false")
	}
	if len(msg.Value) > 0 && !isJSONNull(msg.Value) {
		var v struct {
			Closed bool `json:"closed"`
		}
		if err := json.Unmarshal(msg.Value, &v); err == nil {
			if v.Closed {
				return nil
			}
			return errors.New("server returned closed=false")
		}
	}
	return nil
}

// eventFromServerMessage 将服务端消息转换为 Event。兼容顶层字段和 value 嵌套。
func eventFromServerMessage(msg serverMessage) (Event, bool) {
	op := msg.Operation
	if op == "" {
		op = msg.Type
	}
	// invoke/close 的响应不是订阅事件。
	if op == "invoke" || op == "close" {
		return Event{}, false
	}

	ev := Event{Operation: op}
	if ev.Operation == "" {
		ev.Operation = "event"
	}

	version := msg.Version
	stateRaw := msg.State
	valueRaw := msg.Value

	// 如果顶层没有版本/状态，但 value 内嵌了快照或事件，尝试从中提取。
	if len(msg.Value) > 0 && !isJSONNull(msg.Value) && (version == nil || isJSONNull(stateRaw)) {
		var nested struct {
			Operation string          `json:"operation"`
			Version   *int64          `json:"version"`
			State     json.RawMessage `json:"state"`
			Value     json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(msg.Value, &nested); err == nil {
			foundSnapshotLike := false
			if nested.Version != nil {
				version = nested.Version
				foundSnapshotLike = true
			}
			if len(nested.State) > 0 && !isJSONNull(nested.State) {
				stateRaw = nested.State
				foundSnapshotLike = true
			}
			if len(nested.Value) > 0 && !isJSONNull(nested.Value) {
				valueRaw = nested.Value
				foundSnapshotLike = true
			}
			if nested.Operation != "" {
				ev.Operation = nested.Operation
			}
			// 如果 value 只是快照/事件信封，而不是业务 value，则不要保留整个信封。
			if foundSnapshotLike && (len(nested.Value) == 0 || isJSONNull(nested.Value)) {
				valueRaw = nil
			}
		}
	}

	if version != nil {
		ev.Version = *version
	}
	if len(stateRaw) > 0 && !isJSONNull(stateRaw) {
		var state map[string]any
		if err := json.Unmarshal(stateRaw, &state); err == nil {
			ev.State = state
		}
	}
	if len(valueRaw) > 0 && !isJSONNull(valueRaw) {
		var v any
		if err := json.Unmarshal(valueRaw, &v); err == nil {
			ev.Value = v
		}
	}

	if version == nil && ev.State == nil && ev.Value == nil {
		return Event{}, false
	}
	ev.ChannelID = channelIDFromMessage(msg)
	return ev, true
}

func channelIDFromMessage(msg serverMessage) string {
	if msg.ChannelID != "" {
		return msg.ChannelID
	}
	if len(msg.Value) == 0 || isJSONNull(msg.Value) {
		return ""
	}
	var v struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.Unmarshal(msg.Value, &v); err == nil {
		return v.ChannelID
	}
	return ""
}

func isJSONNull(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}
