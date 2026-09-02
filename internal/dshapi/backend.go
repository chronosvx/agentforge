package dshapi

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
)

// Backend 把 dshapi 适配成 UI 层需要的聊天后端接口。
// 它内部维护 events.mux 流，并把 DSH 的 session/event 转换为 client.Event。
type Backend struct {
	api *Client

	mu         sync.Mutex
	session    string
	mux        <-chan ServerRequest
	cancel     context.CancelFunc
	lastSeq    int64
	messages   []client.Message
	context    map[string]any
	cost       map[string]any
	tokenUsage map[string]any
	todos      []any
	diffs      []any
	tools      []any
}

// NewBackend 创建基于真实 DSH Web API 的 UI 后端。
func NewBackend(api *Client) *Backend {
	return &Backend{api: api}
}

// Open 打开/创建一个会话，并开始监听 events.mux。
func (b *Backend) Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*client.Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sessionID == "" {
		req := CreateSessionRequest{Cwd: workspace}
		if req.Cwd == "" {
			req.Cwd = "."
		}
		created, err := b.api.CreateSession(ctx, req)
		if err != nil {
			return nil, err
		}
		sessionID = created.SessionID
	}

	// 取消上一次 mux 流，避免切换会话后旧事件继续占用资源。
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	// mux 的生命周期只由 Backend 自己管理：从调用方 context 剥离取消信号，
	// 避免 UI 的短生命周期 openCtx 取消后把 events.mux 一并关掉。
	muxCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	mux, err := b.api.OpenMux(muxCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("dshapi backend open mux: %w", err)
	}

	b.session = sessionID
	b.mux = mux
	b.cancel = cancel
	b.lastSeq = 0
	b.messages = nil
	b.context = map[string]any{}
	b.cost = map[string]any{}
	b.tokenUsage = map[string]any{}
	b.todos = nil
	b.diffs = nil
	b.tools = nil

	// 尝试加载历史，让 resume 后 UI 能立即显示旧状态，并初始化 lastSeq。
	if hist, err := b.api.History(ctx, sessionID, nil, nil); err == nil {
		b.applyHistoryLocked(hist)
	}

	return &client.Snapshot{
		ChannelID: sessionID,
		Version:   b.lastSeq,
		State: map[string]any{
			"messages":   messagesToAny(b.messages),
			"context":    b.context,
			"cost":       b.cost,
			"tokenUsage": b.tokenUsage,
			"todos":      b.todos,
			"diffs":      b.diffs,
			"tools":      b.tools,
		},
	}, nil
}

// Subscribe 返回一个持续产出 client.Event 的 channel。
// 当 after > 0 时，会先通过 session.history 补齐 after 之后的事件，
// 再继续读取 events.mux 实时流；重复 seq 会被跳过。
func (b *Backend) Subscribe(ctx context.Context, channelID string, after int64) (<-chan client.Event, error) {
	b.mu.Lock()
	mux := b.mux
	startSeq := after
	if b.lastSeq > startSeq {
		startSeq = b.lastSeq
	}
	b.mu.Unlock()
	if mux == nil {
		return nil, fmt.Errorf("dshapi backend: mux not opened")
	}

	ch := make(chan client.Event, 256)
	go func() {
		defer close(ch)
		cursor := startSeq

		// 断线/重连时，先补齐 after 之后的历史事件。
		if cursor > 0 {
			if newCursor, err := b.backfillHistory(ctx, channelID, cursor, ch); err == nil {
				cursor = newCursor
			}
			// History 失败时不阻断实时流；避免重连后完全收不到新事件。
		}

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-mux:
				if !ok {
					return
				}
				seq, hasSeq := frameSeq(frame, channelID)
				if hasSeq && seq <= cursor {
					continue
				}
				ev := b.frameToEvent(frame, channelID)
				if ev == nil {
					continue
				}
				if hasSeq && seq > cursor {
					cursor = seq
				}
				select {
				case ch <- *ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

// Invoke 发送 chat/sendMessage 消息。
func (b *Backend) Invoke(ctx context.Context, channelID, method string, args []any) (any, error) {
	if method != "chat" && method != "sendMessage" {
		return nil, nil
	}
	text := firstText(args)
	if text == "" {
		return nil, fmt.Errorf("dshapi backend: empty message")
	}
	_, err := b.api.Prompt(ctx, channelID, text, "queue")
	if err != nil {
		return nil, err
	}
	return text, nil
}

// Cancel 请求 DSH 取消当前生成。
func (b *Backend) Cancel(ctx context.Context, channelID string) error {
	return b.api.Cancel(ctx, channelID)
}

// Close 关闭当前 mux 流。
func (b *Backend) Close(ctx context.Context, channelID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.mux = nil
	return nil
}

// CloseConn 关闭后端资源。
func (b *Backend) CloseConn() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.mux = nil
	return nil
}

// ListSessions 返回 DSH 侧的远程会话列表，供 UI 的会话面板使用。
func (b *Backend) ListSessions(ctx context.Context) ([]session.Entry, error) {
	list, err := b.api.ListSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]session.Entry, 0, len(list.Items))
	for _, item := range list.Items {
		out = append(out, session.Entry{
			ID:        item.SessionID,
			Workspace: item.Cwd,
			Title:     item.SessionID,
			CreatedAt: time.UnixMilli(item.UpdatedAt),
			UpdatedAt: time.UnixMilli(item.UpdatedAt),
		})
	}
	return out, nil
}

// frameToEvent 将 ServerRequest 转换为 client.Event；只处理当前会话的帧。
func (b *Backend) frameToEvent(frame ServerRequest, channelID string) *client.Event {
	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil
	}
	typ, _ := payload["type"].(string)
	sid, _ := payload["sessionId"].(string)
	if sid != channelID {
		return nil
	}

	switch typ {
	case "session/event":
		rawEvent, ok := payload["event"].(map[string]any)
		if !ok {
			return nil
		}
		b.applyEvent(rawEvent)
		seq, _ := rawEvent["seq"].(float64)
		seqInt := int64(seq)
		b.markSeq(seqInt)
		if text, ok := textDeltaFromEvent(rawEvent); ok {
			return &client.Event{
				Operation: "delta",
				ChannelID: channelID,
				Version:   seqInt,
				Value: map[string]any{
					"type":    "text",
					"content": text,
				},
			}
		}
		return &client.Event{
			Operation: "event",
			ChannelID: channelID,
			Version:   seqInt,
			State:     b.snapshotState(),
		}
	case "session/projection":
		key, _ := payload["key"].(string)
		value := payload["value"]
		b.applyProjection(key, value)
		seq, _ := payload["seq"].(float64)
		b.markSeq(int64(seq))
		return &client.Event{
			Operation: "event",
			ChannelID: channelID,
			Version:   int64(seq),
			State:     b.snapshotState(),
		}
	default:
		return nil
	}
}

// markSeq 记录当前会话已消费到的最大事件 seq。
func (b *Backend) markSeq(seq int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if seq > b.lastSeq {
		b.lastSeq = seq
	}
}

// frameSeq 从 ServerRequest 中提取当前会话帧的 seq；不是事件帧时返回 false。
func frameSeq(frame ServerRequest, channelID string) (int64, bool) {
	var payload map[string]any
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return 0, false
	}
	sid, _ := payload["sessionId"].(string)
	if sid != channelID {
		return 0, false
	}
	switch typ, _ := payload["type"].(string); typ {
	case "session/event":
		rawEvent, ok := payload["event"].(map[string]any)
		if !ok {
			return 0, false
		}
		seq, _ := rawEvent["seq"].(float64)
		return int64(seq), true
	case "session/projection":
		seq, _ := payload["seq"].(float64)
		return int64(seq), true
	default:
		return 0, false
	}
}

// backfillHistory 用 session.history 补齐 after 之后的事件，并把它们作为
// client.Event 发送到 ch。返回补齐后的最新 cursor。
func (b *Backend) backfillHistory(ctx context.Context, channelID string, after int64, ch chan<- client.Event) (int64, error) {
	// session.history 只支持 beforeSeq，没有 afterSeq；这里向后翻页取全量历史，
	// 再在客户端按 seq > after 过滤，实现“从 after 继续”。
	var entries []HistoryEntry
	var before *int64
	const maxHistoryPages = 100
	for page := 0; page < maxHistoryPages; page++ {
		hist, err := b.api.History(ctx, channelID, before, nil)
		if err != nil {
			return after, err
		}
		entries = append(entries, hist.Events...)
		if !hist.HasMore || len(hist.Events) == 0 {
			break
		}
		minSeq := hist.Events[0].Event.Seq
		for _, e := range hist.Events[1:] {
			if e.Event.Seq < minSeq {
				minSeq = e.Event.Seq
			}
		}
		before = &minSeq
		if minSeq <= after {
			break
		}
	}

	// history 接口返回顺序不保证；这里按 seq 升序应用。
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Event.Seq < entries[j].Event.Seq
	})

	cursor := after
	for _, entry := range entries {
		seq := entry.Event.Seq
		if seq <= cursor {
			continue
		}
		ev := b.historyEntryToEvent(entry, channelID)
		if ev == nil {
			continue
		}
		if seq > cursor {
			cursor = seq
		}
		select {
		case ch <- *ev:
		case <-ctx.Done():
			return cursor, ctx.Err()
		}
	}
	return cursor, nil
}

// historyEntryToEvent 把一条 session.history 记录转换为 client.Event 并应用状态。
func (b *Backend) historyEntryToEvent(entry HistoryEntry, channelID string) *client.Event {
	payload, err := json.Marshal(map[string]any{
		"type":      "session/event",
		"sessionId": channelID,
		"event": map[string]any{
			"type": entry.Event.Type,
			"seq":  float64(entry.Event.Seq),
			"time": float64(entry.Event.Time),
			"data": entry.Event.Data,
		},
	})
	if err != nil {
		return nil
	}
	return b.frameToEvent(ServerRequest{Type: "server-request", Payload: payload}, channelID)
}

// snapshotState 在锁内返回当前 UI 状态的深拷贝，避免并发读写。
func (b *Backend) snapshotState() map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return map[string]any{
		"messages":   messagesToAny(b.messages),
		"context":    cloneMap(b.context),
		"cost":       cloneMap(b.cost),
		"tokenUsage": cloneMap(b.tokenUsage),
		"todos":      cloneSlice(b.todos),
		"diffs":      cloneSlice(b.diffs),
		"tools":      cloneSlice(b.tools),
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneSlice(s []any) []any {
	if s == nil {
		return []any{}
	}
	out := make([]any, len(s))
	copy(out, s)
	return out
}

// applyHistoryLocked 在调用方已持有 b.mu 的情况下，把 history 事件完整应用到共享状态，
// 并更新 lastSeq。history 记录按 seq 升序应用，避免消息顺序错乱。
func (b *Backend) applyHistoryLocked(hist *HistoryResponse) {
	if hist == nil {
		return
	}
	entries := append([]HistoryEntry(nil), hist.Events...)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Event.Seq < entries[j].Event.Seq
	})
	for _, entry := range entries {
		if entry.Event.Seq > b.lastSeq {
			b.lastSeq = entry.Event.Seq
		}
		b.applyEventLocked(map[string]any{
			"type": entry.Event.Type,
			"data": entry.Event.Data,
		})
	}
}

// applyEvent 从 session/event 中提取 user/assistant 消息。
func (b *Backend) applyEvent(event map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyEventLocked(event)
}

func (b *Backend) applyEventLocked(event map[string]any) {
	typ, _ := event["type"].(string)
	data, _ := event["data"].(map[string]any)
	if data == nil {
		return
	}

	role := ""
	switch typ {
	case "assistant/chunk":
		b.applyChunkLocked(data)
		return
	case "tool/call":
		b.applyToolCallLocked(data)
		return
	case "tool/result":
		b.applyToolResultLocked(data)
		return
	case "user/message":
		role = "user"
	case "assistant/message":
		role = "assistant"
	default:
		return
	}

	text := extractText(data)
	if text == "" {
		return
	}
	// 避免同一消息被多个事件重复追加：简单按 role+text 去重最后一跳。
	if n := len(b.messages); n > 0 && b.messages[n-1].Role == role && b.messages[n-1].Content == text {
		return
	}
	b.messages = append(b.messages, client.Message{Role: role, Content: text})
}

// textDeltaFromEvent 判断 session/event 是否为 assistant/chunk 的文本增量。
func textDeltaFromEvent(event map[string]any) (string, bool) {
	if typ, _ := event["type"].(string); typ != "assistant/chunk" {
		return "", false
	}
	data, _ := event["data"].(map[string]any)
	if data == nil {
		return "", false
	}
	chunk, _ := data["chunk"].(map[string]any)
	if chunk == nil {
		return "", false
	}
	if chunkType, _ := chunk["type"].(string); chunkType != "text-delta" {
		return "", false
	}
	text, _ := chunk["text"].(string)
	if text == "" {
		return "", false
	}
	return text, true
}

// applyChunk 处理 assistant/chunk 的 text-delta 与 usage 事件。
func (b *Backend) applyChunk(data map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyChunkLocked(data)
}

func (b *Backend) applyChunkLocked(data map[string]any) {
	chunk, _ := data["chunk"].(map[string]any)
	if chunk == nil {
		return
	}
	typ, _ := chunk["type"].(string)
	switch typ {
	case "text-delta":
		text, _ := chunk["text"].(string)
		if text == "" {
			return
		}
		if n := len(b.messages); n > 0 && b.messages[n-1].Role == "assistant" {
			b.messages[n-1].Content += text
		} else {
			b.messages = append(b.messages, client.Message{Role: "assistant", Content: text})
		}
	case "usage":
		usage, _ := chunk["usage"].(map[string]any)
		if usage == nil {
			return
		}
		if b.context == nil {
			b.context = map[string]any{}
		}
		if b.cost == nil {
			b.cost = map[string]any{}
		}
		for _, key := range []string{"inputTokens", "outputTokens", "cacheReadTokens", "reasoningTokens"} {
			if v, ok := usage[key]; ok {
				b.context[key] = v
			}
		}
		b.cost["lastUsage"] = usage
	}
}

// applyToolCall 记录 tool/call，并解析 todo_write 到 todos。
func (b *Backend) applyToolCall(data map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyToolCallLocked(data)
}

func (b *Backend) applyToolCallLocked(data map[string]any) {
	name, _ := data["name"].(string)
	argsRaw, _ := data["arguments"].(string)

	b.tools = append(b.tools, map[string]any{
		"type":      "call",
		"name":      name,
		"arguments": argsRaw,
	})
	if name == "todo_write" && argsRaw != "" {
		var args struct {
			Todos []any `json:"todos"`
		}
		if err := json.Unmarshal([]byte(argsRaw), &args); err == nil {
			b.todos = args.Todos
		}
	}
}

// applyToolResult 记录 tool/result，并解析 git diff 到 diffs。
func (b *Backend) applyToolResult(data map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.applyToolResultLocked(data)
}

func (b *Backend) applyToolResultLocked(data map[string]any) {
	text := extractToolResultText(data)
	summary := text
	if len(summary) > 200 {
		summary = summary[:200] + "…"
	}
	b.tools = append(b.tools, map[string]any{
		"type":   "result",
		"result": summary,
	})

	if text == "" {
		return
	}
	diffs := parseGitDiffs(text)
	if len(diffs) == 0 {
		return
	}
	b.diffs = diffs
}

// extractToolResultText 从 tool/result 的 message.content 中提取纯文本。
func extractToolResultText(data map[string]any) string {
	message, _ := data["message"].(map[string]any)
	if message == nil {
		return ""
	}
	content, _ := message["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	first, _ := content[0].(map[string]any)
	if first == nil {
		return ""
	}
	inner, _ := first["content"].([]any)
	if len(inner) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range inner {
		pm, ok := part.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := pm["type"].(string); t == "text" {
			if s, _ := pm["text"].(string); s != "" {
				b.WriteString(s)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// parseGitDiffs 从文本中提取 diff --git 块。
func parseGitDiffs(text string) []any {
	var diffs []any
	lines := strings.Split(text, "\n")
	var currentFile string
	var current []string
	flush := func() {
		if currentFile == "" || len(current) == 0 {
			return
		}
		diffs = append(diffs, map[string]any{
			"file": currentFile,
			"diff": strings.Join(current, "\n"),
		})
		currentFile = ""
		current = nil
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			flush()
			// 形如 diff --git a/path b/path
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				currentFile = strings.TrimPrefix(parts[3], "b/")
			} else {
				currentFile = "unknown"
			}
			current = append(current, line)
			continue
		}
		if currentFile != "" {
			current = append(current, line)
		}
	}
	flush()
	return diffs
}

// applyProjection 把 contextBreakdown 等投影写入 context/cost。
func (b *Backend) applyProjection(key string, value any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch key {
	case "contextBreakdown":
		if m, ok := value.(map[string]any); ok {
			b.context["breakdown"] = m
		}
	case "contextPressure":
		if m, ok := value.(map[string]any); ok {
			b.context["pressure"] = m
		}
	case "sessionStats":
		if m, ok := value.(map[string]any); ok {
			b.cost["stats"] = m
		}
	case "tokenUsage":
		if m, ok := value.(map[string]any); ok {
			b.tokenUsage = m
		}
	}
}

func messagesToAny(msgs []client.Message) []any {
	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content})
	}
	return out
}

func extractText(data map[string]any) string {
	content, ok := data["content"]
	if !ok {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b []byte
		for _, part := range v {
			if pm, ok := part.(map[string]any); ok {
				if t, _ := pm["type"].(string); t == "text" {
					if s, _ := pm["text"].(string); s != "" {
						if len(b) > 0 {
							b = append(b, '\n')
						}
						b = append(b, s...)
					}
				}
			}
		}
		return string(b)
	case map[string]any:
		if s, ok := v["text"].(string); ok {
			return s
		}
	}
	return ""
}

func firstText(args []any) string {
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
