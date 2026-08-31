// Package mock 提供一个内存版 DSH TUI Channel WebSocket 服务端，
// 用于没有真实 DSH 环境时的开发与测试。
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// WireRevision 与当前 DSH TUI Channel 协议版本保持一致。
const WireRevision = 6

// DefaultPath 是 DSH TUI Channel WebSocket 的标准路径。
const DefaultPath = "/agentforge/v1/connection"

// Server 是一个极简的 DSH TUI Channel mock 服务端。
type Server struct {
	ln         net.Listener
	httpServer *http.Server

	mu       sync.Mutex
	channels map[string]*channel
	sessions map[string]*channel
	nextID   int

	closeOnce sync.Once
	closed    chan struct{}
}

type channel struct {
	ID      string
	Version int64
	State   map[string]any
}

// Start 在 addr（例如 "127.0.0.1:0"）上启动 mock 服务端。
// ctx 被取消时服务端会自动关闭。
func Start(ctx context.Context, addr string) (*Server, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("mock listen: %w", err)
	}

	s := &Server{
		ln:       ln,
		channels: make(map[string]*channel),
		sessions: make(map[string]*channel),
		closed:   make(chan struct{}),
	}

	mux := http.NewServeMux()
	// 为方便测试，任意路径都按 Channel WebSocket 处理。
	mux.HandleFunc("/", s.handleWebSocket)
	s.httpServer = &http.Server{Handler: mux}

	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-s.closed:
		}
	}()

	go func() {
		_ = s.httpServer.Serve(ln)
	}()

	return s, nil
}

// Addr 返回服务端监听地址，通常形如 "127.0.0.1:port"。
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Close 关闭 mock 服务端。多次调用是安全的。
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closed)
		if s.httpServer != nil {
			err = s.httpServer.Close()
		} else if s.ln != nil {
			err = s.ln.Close()
		}
	})
	return err
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "server close")
	s.serveConn(conn)
}

func (s *Server) serveConn(conn *websocket.Conn) {
	for {
		_, data, err := conn.Read(context.Background())
		if err != nil {
			return
		}

		var req struct {
			Operation string          `json:"operation"`
			Value     json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			continue
		}

		switch req.Operation {
		case "open":
			s.handleOpen(conn, req.Value)
		case "subscribe":
			s.handleSubscribe(conn, req.Value)
		case "invoke":
			s.handleInvoke(conn, req.Value)
		case "close":
			s.handleClose(conn, req.Value)
		}
	}
}

func (s *Server) handleOpen(conn *websocket.Conn, raw json.RawMessage) {
	var req struct {
		Workspace string         `json:"workspace"`
		SessionID string         `json:"sessionId"`
		Options   map[string]any `json:"options"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":    "INVALID_ARGUMENT",
			"message": err.Error(),
		})
		return
	}

	s.mu.Lock()
	// 如果 sessionId 已存在，直接复用之前的 channel，支持 resume/切换会话。
	if req.SessionID != "" {
		if existing, ok := s.sessions[req.SessionID]; ok {
			if _, ok := s.channels[existing.ID]; !ok {
				// close 后 channel 被移除但 session 状态保留，重新挂载。
				s.channels[existing.ID] = existing
			}
			s.mu.Unlock()
			_ = writeFrame(conn, "open", s.snapshot(existing))
			return
		}
	}

	s.nextID++
	id := fmt.Sprintf("ch-%d", s.nextID)
	state := map[string]any{
		"workspace": req.Workspace,
		"sessionId": req.SessionID,
		"messages":  []any{},
		"diffs":     []any{},
		"context": map[string]any{
			"totalTokens":   0,
			"cacheHits":     0,
			"uncachedInput": 0,
			"output":        0,
			"breakdown": map[string]any{
				"system":  1200,
				"history": 0,
				"tools":   800,
			},
		},
		"cost": map[string]any{
			"estimatedYuan": 0.0,
			"currency":      "CNY",
		},
	}
	if req.Options != nil {
		state["options"] = req.Options
	}
	ch := &channel{ID: id, State: state}
	s.channels[id] = ch
	if req.SessionID != "" {
		s.sessions[req.SessionID] = ch
	}
	s.mu.Unlock()

	_ = writeFrame(conn, "open", s.snapshot(ch))
}

func (s *Server) handleSubscribe(conn *websocket.Conn, raw json.RawMessage) {
	var req struct {
		ChannelID    string `json:"channelId"`
		AfterVersion int64  `json:"afterVersion"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":    "INVALID_ARGUMENT",
			"message": err.Error(),
		})
		return
	}

	s.mu.Lock()
	ch := s.channels[req.ChannelID]
	s.mu.Unlock()
	if ch == nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":      "CHANNEL_NOT_FOUND",
			"channelId": req.ChannelID,
		})
		return
	}

	_ = writeFrame(conn, "subscribe", s.snapshot(ch))
}

func (s *Server) handleInvoke(conn *websocket.Conn, raw json.RawMessage) {
	var req struct {
		ChannelID string `json:"channelId"`
		Method    string `json:"method"`
		Arguments []any  `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":    "INVALID_ARGUMENT",
			"message": err.Error(),
		})
		return
	}

	s.mu.Lock()
	ch := s.channels[req.ChannelID]
	s.mu.Unlock()
	if ch == nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":      "CHANNEL_NOT_FOUND",
			"channelId": req.ChannelID,
		})
		return
	}

	if req.Method != "chat" && req.Method != "sendMessage" {
		_ = writeFrame(conn, "invoke", map[string]any{
			"value":        nil,
			"valueDefined": false,
			"snapshot":     s.snapshot(ch),
		})
		return
	}

	text := firstArgText(req.Arguments)

	// 先追加用户消息并发送一个事件。
	s.mu.Lock()
	ch.Version++
	msgs, _ := ch.State["messages"].([]any)
	msgs = append(msgs, map[string]any{"role": "user", "content": text})
	ch.State["messages"] = msgs
	updateMockUsage(ch.State, text, false)
	s.mu.Unlock()

	ver, state := s.stateAndVersion(ch)
	_ = writeFrame(conn, "event", map[string]any{
		"channelId": ch.ID,
		"version":   ver,
		"state":     state,
	})

	// 模拟 assistant 流式回复：逐段增长 content 并发送事件。
	chunks := []string{"This ", "is ", "a ", "mock ", "assistant ", "reply."}
	acc := ""
	for _, chunk := range chunks {
		acc += chunk

		s.mu.Lock()
		ch.Version++
		msgs, _ := ch.State["messages"].([]any)
		if len(msgs) > 0 {
			if last, ok := msgs[len(msgs)-1].(map[string]any); ok && last["role"] == "assistant" {
				last["content"] = acc
			} else {
				msgs = append(msgs, map[string]any{"role": "assistant", "content": acc})
			}
		} else {
			msgs = append(msgs, map[string]any{"role": "assistant", "content": acc})
		}
		ch.State["messages"] = msgs
		updateMockUsage(ch.State, chunk, true)
		s.mu.Unlock()

		ver, state := s.stateAndVersion(ch)
		_ = writeFrame(conn, "event", map[string]any{
			"channelId": ch.ID,
			"version":   ver,
			"state":     state,
		})
	}

	// 模拟一次代码修改产生的 diff 和 todo，供面板使用。
	s.mu.Lock()
	ch.State["diffs"] = []any{
		map[string]any{
			"file": "internal/ui/model.go",
			"diff": "@@ -1,3 +1,4 @@\n package ui\n \n+// Diff Review 示例\n type Model struct {",
		},
		map[string]any{
			"file": "README.md",
			"diff": "@@ -1,2 +1,3 @@\n # agentforge\n \n+支持 Diff Review 面板",
		},
	}
	ch.State["todos"] = []any{
		map[string]any{"content": "实现 ChatBackend 抽象", "status": "completed"},
		map[string]any{"content": "接入真实 DSH Web API", "status": "completed"},
		map[string]any{"content": "实现 Diff Review 工作台", "status": "in_progress"},
		map[string]any{"content": "实现多会话 Tab 工作区", "status": "pending"},
	}
	ch.Version++
	s.mu.Unlock()

	ver, state = s.stateAndVersion(ch)
	_ = writeFrame(conn, "event", map[string]any{
		"channelId": ch.ID,
		"version":   ver,
		"state":     state,
	})

	_ = writeFrame(conn, "invoke", map[string]any{
		"value":        acc,
		"valueDefined": true,
		"snapshot":     s.snapshot(ch),
	})
}

// updateMockUsage 更新 mock state 中的 context/cost 统计。
// isOutput 为 true 时表示 assistant 输出 token，否则表示用户输入 token。
func updateMockUsage(state map[string]any, text string, isOutput bool) {
	ctxRaw, _ := state["context"].(map[string]any)
	if ctxRaw == nil {
		ctxRaw = map[string]any{}
		state["context"] = ctxRaw
	}
	total := toFloat(ctxRaw["totalTokens"])
	uncached := toFloat(ctxRaw["uncachedInput"])
	output := toFloat(ctxRaw["output"])
	cacheHits := toFloat(ctxRaw["cacheHits"])

	approxTokens := float64(len([]rune(text)) * 2)
	if approxTokens < 8 {
		approxTokens = 8
	}
	if isOutput {
		output += approxTokens
	} else {
		uncached += approxTokens
	}
	total += approxTokens
	cacheHits += approxTokens * 2 // 模拟缓存命中，让成本更接近真实场景

	ctxRaw["totalTokens"] = total
	ctxRaw["uncachedInput"] = uncached
	ctxRaw["output"] = output
	ctxRaw["cacheHits"] = cacheHits

	// 极简成本估算：按缓存命中 0.05 元/百万 token、未命中 1.5 元/百万、输出 4.5 元/百万。
	cost := (cacheHits*0.05 + uncached*1.5 + output*4.5) / 1e6
	costRaw, _ := state["cost"].(map[string]any)
	if costRaw == nil {
		costRaw = map[string]any{}
		state["cost"] = costRaw
	}
	costRaw["estimatedYuan"] = cost
	costRaw["currency"] = "CNY"
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func (s *Server) handleClose(conn *websocket.Conn, raw json.RawMessage) {
	var req struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = writeFrame(conn, "error", map[string]any{
			"code":    "INVALID_ARGUMENT",
			"message": err.Error(),
		})
		return
	}

	s.mu.Lock()
	delete(s.channels, req.ChannelID)
	s.mu.Unlock()

	_ = writeFrame(conn, "close", map[string]any{"closed": true})
}

// snapshot 返回 Channel 当前快照的 JSON 值。
func (s *Server) snapshot(ch *channel) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := map[string]any{}
	if ch.State != nil {
		state = cloneValue(ch.State).(map[string]any)
	}
	return map[string]any{
		"wireRevision": WireRevision,
		"channelId":    ch.ID,
		"version":      ch.Version,
		"state":        state,
	}
}

// stateAndVersion 在锁内返回当前版本和 state 的深拷贝。
func (s *Server) stateAndVersion(ch *channel) (int64, map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ch.State == nil {
		return ch.Version, map[string]any{}
	}
	return ch.Version, cloneValue(ch.State).(map[string]any)
}

func cloneValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, val := range x {
			m[k] = cloneValue(val)
		}
		return m
	case []any:
		s := make([]any, len(x))
		for i, val := range x {
			s[i] = cloneValue(val)
		}
		return s
	default:
		return v
	}
}

func firstArgText(args []any) string {
	if len(args) == 0 {
		return ""
	}
	switch v := args[0].(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v["content"].(string); ok {
			return s
		}
		if s, ok := v["text"].(string); ok {
			return s
		}
	}
	return fmt.Sprintf("%v", args[0])
}

func writeFrame(conn *websocket.Conn, operation string, value any) error {
	data, err := json.Marshal(map[string]any{
		"operation": operation,
		"value":     value,
	})
	if err != nil {
		return err
	}
	return conn.Write(context.Background(), websocket.MessageText, data)
}
