package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/offline"
	"github.com/chronosvx/agentforge/internal/session"
)

// Backend 把 Provider 直连模式适配成 UI 层需要的 ChatBackend。
//
// 它与 DSH Backend 的差异是：会话状态保存在本地内存 + session.Store，
// Invoke 时调用 Provider.StreamChat，并把文本增量转换为 client.Event 的
// state.messages 更新，使 Bubble Tea UI 可以直接流式渲染。
type Backend struct {
	mu       sync.Mutex
	provider Provider
	store    session.LocalStore
	model    string
	offline  *offline.Queue
	sessions map[string]*sessionState
}

// sessionState 是 Provider Backend 中一个本地会话的运行时状态。
type sessionState struct {
	id         string
	workspace  string
	messages   []client.Message
	context    map[string]any
	cost       map[string]any
	tokenUsage map[string]any
	seq        int64
	streaming  bool
	closed     bool
	ctx        context.Context
	cancel     context.CancelFunc
	subs       map[chan client.Event]struct{}
}

// NewBackend 创建基于 Provider 的 UI 后端。store 可为 nil，此时不写本地历史。
func NewBackend(p Provider, store session.LocalStore) *Backend {
	model := ""
	if p != nil {
		model = p.Model()
	}
	return &Backend{
		provider: p,
		store:    store,
		model:    model,
		sessions: make(map[string]*sessionState),
	}
}

// NewBackendWithModel 创建基于 Provider 的 UI 后端，并显式指定初始模型名。
func NewBackendWithModel(p Provider, store session.LocalStore, model string) *Backend {
	b := NewBackend(p, store)
	if model != "" {
		b.model = model
	}
	return b
}

// NewBackendWithOffline 创建基于 Provider 的 UI 后端，并设置离线消息队列。
func NewBackendWithOffline(p Provider, store session.LocalStore, q *offline.Queue) *Backend {
	b := NewBackend(p, store)
	b.offline = q
	return b
}

// SetOfflineQueue 设置离线消息队列。队列用于 StreamChat 首次失败时暂存用户消息。
func (b *Backend) SetOfflineQueue(q *offline.Queue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.offline = q
}

// ProviderName 返回当前 Provider 的显示名称。
func (b *Backend) ProviderName() string {
	if b == nil || b.provider == nil {
		return ""
	}
	return b.provider.Name()
}

// Model 返回当前使用的模型名。
func (b *Backend) Model() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.model
}

// SetModel 切换当前模型名。Provider 直连模式下该名称会用于后续请求。
func (b *Backend) SetModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("provider backend: model name is empty")
	}
	b.mu.Lock()
	b.model = model
	b.mu.Unlock()
	return nil
}

// Open 打开（或恢复）一个本地 Provider 会话，并返回 UI 需要的快照。
func (b *Backend) Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*client.Snapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sessionID == "" {
		sessionID = fmt.Sprintf("session-%d", time.Now().UnixMilli())
	}

	st := b.sessions[sessionID]
	if st == nil {
		st = &sessionState{
			id:        sessionID,
			workspace: workspace,
			subs:      make(map[chan client.Event]struct{}),
		}
		if b.store != nil {
			if entry, err := b.store.Load(sessionID); err == nil {
				st.messages = append([]client.Message(nil), entry.Messages...)
				st.context = cloneMap(entry.Context)
				st.cost = cloneMap(entry.Cost)
			}
		}
		b.sessions[sessionID] = st
	}

	if st.cancel != nil {
		st.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.ctx = ctx
	st.cancel = cancel
	st.closed = false

	return &client.Snapshot{
		ChannelID: sessionID,
		Version:   st.seq,
		State:     b.stateLocked(st),
	}, nil
}

// Subscribe 返回当前会话的事件流。Provider Backend 的流在 Close/CloseConn 时关闭。
func (b *Backend) Subscribe(ctx context.Context, channelID string, after int64) (<-chan client.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.sessions[channelID]
	if st == nil {
		return nil, fmt.Errorf("provider backend: session %q not open", channelID)
	}

	ch := make(chan client.Event, 256)
	st.subs[ch] = struct{}{}
	return ch, nil
}

// Invoke 发送用户消息，并异步调用 Provider.StreamChat 产生事件流。
// 方法本身会尽快返回，流式事件通过 Subscribe 返回的 channel 推送。
func (b *Backend) Invoke(ctx context.Context, channelID, method string, args []any) (any, error) {
	if method != "chat" && method != "sendMessage" {
		return nil, nil
	}
	text := firstText(args)
	if text == "" {
		return nil, fmt.Errorf("provider backend: empty message")
	}

	b.mu.Lock()
	st := b.sessions[channelID]
	if st == nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("provider backend: session %q not open", channelID)
	}
	if st.streaming {
		b.mu.Unlock()
		return nil, fmt.Errorf("provider backend: session %q is already streaming", channelID)
	}

	st.streaming = true
	st.messages = append(st.messages, client.Message{Role: "user", Content: text})
	st.seq++
	ev := client.Event{
		Operation: "event",
		ChannelID: channelID,
		Version:   st.seq,
		State:     b.stateLocked(st),
	}
	b.broadcastLocked(ev, st)
	model := b.model
	workspace := st.workspace
	messages := append([]client.Message(nil), st.messages...)
	contextMap := cloneMap(st.context)
	costMap := cloneMap(st.cost)
	b.mu.Unlock()

	b.persist(channelID, workspace, messages, contextMap, costMap)
	go b.stream(st, channelID, model, false)
	return text, nil
}

// FlushOffline 依次重发离线队列中的消息。每条消息重发成功后从队列移除，
// 返回处理过程中遇到的第一个错误。
func (b *Backend) FlushOffline(ctx context.Context) error {
	b.mu.Lock()
	q := b.offline
	b.mu.Unlock()
	if q == nil {
		return nil
	}

	items, err := q.Pending()
	if err != nil {
		return err
	}

	var firstErr error
	for _, item := range items {
		if err := b.sendOffline(ctx, item); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := q.Remove(item.ID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// sendOffline 同步重发一条离线队列消息，并在收到终态事件后返回。
// 返回 nil 表示重发成功；返回 error 表示重发失败，调用方应保留队列项。
func (b *Backend) sendOffline(ctx context.Context, item offline.Item) error {
	b.mu.Lock()
	st := b.sessions[item.SessionID]
	if st == nil {
		b.mu.Unlock()
		if _, err := b.Open(ctx, "", item.SessionID, nil); err != nil {
			return fmt.Errorf("provider backend: reopen offline session %q: %w", item.SessionID, err)
		}
		b.mu.Lock()
		st = b.sessions[item.SessionID]
		if st == nil {
			b.mu.Unlock()
			return fmt.Errorf("provider backend: offline session %q unavailable after reopen", item.SessionID)
		}
		// 新打开/恢复的会话尚未包含这条用户消息，需要重新追加。
		item.Replay = false
	}

	if st.streaming {
		b.mu.Unlock()
		return fmt.Errorf("provider backend: session %q is already streaming", item.SessionID)
	}

	if item.Replay {
		// 消息已存在于内存会话中，只重放流，不重复追加。
		st.streaming = true
	} else {
		st.messages = append(st.messages, client.Message{Role: "user", Content: item.Message})
		st.seq++
		ev := client.Event{
			Operation: "event",
			ChannelID: item.SessionID,
			Version:   st.seq,
			State:     b.stateLocked(st),
		}
		b.broadcastLocked(ev, st)
		st.streaming = true
	}

	model := b.model
	workspace := st.workspace
	messages := append([]client.Message(nil), st.messages...)
	contextMap := cloneMap(st.context)
	costMap := cloneMap(st.cost)
	doneCh := make(chan client.Event, 1)
	st.subs[doneCh] = struct{}{}
	b.mu.Unlock()

	if !item.Replay {
		b.persist(item.SessionID, workspace, messages, contextMap, costMap)
	}

	go b.stream(st, item.SessionID, model, true)

	return b.waitForOfflineTerminal(ctx, item.SessionID, doneCh)
}

func (b *Backend) waitForOfflineTerminal(ctx context.Context, sessionID string, ch chan client.Event) error {
	defer func() {
		b.mu.Lock()
		if st := b.sessions[sessionID]; st != nil {
			delete(st.subs, ch)
		}
		b.mu.Unlock()
	}()

	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return errors.New("provider backend: offline stream subscriber closed before terminal event")
			}
			switch ev.Operation {
			case "done":
				return nil
			case "error":
				if cost, ok := ev.State["cost"].(map[string]any); ok {
					if msg, ok := cost["lastError"].(string); ok && msg != "" {
						return errors.New(msg)
					}
				}
				return errors.New("provider backend: offline stream failed")
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("provider backend: offline stream timed out")
		}
	}
}

// Close 关闭指定会话，并关闭该会话的订阅流。
func (b *Backend) Close(ctx context.Context, channelID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.sessions[channelID]
	if st == nil {
		return nil
	}
	b.closeStateLocked(st)
	delete(b.sessions, channelID)
	return nil
}

// CloseConn 关闭所有会话与订阅流。
func (b *Backend) CloseConn() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, st := range b.sessions {
		b.closeStateLocked(st)
		delete(b.sessions, id)
	}
	return nil
}

func (b *Backend) stream(st *sessionState, channelID, model string, fromOffline bool) {
	defer func() {
		b.mu.Lock()
		st.streaming = false
		b.mu.Unlock()
	}()

	b.mu.Lock()
	msgs := make([]Message, 0, len(st.messages))
	userText := ""
	for _, m := range st.messages {
		msgs = append(msgs, Message{Role: m.Role, Content: m.Content})
		if m.Role == "user" {
			userText = m.Content
		}
	}
	streamCtx := st.ctx
	if streamCtx == nil {
		streamCtx = context.Background()
	}
	offlineQueue := b.offline
	b.mu.Unlock()

	// 使用会话级 context；Close 时通过 st.cancel 取消。
	ch, err := b.provider.StreamChat(streamCtx, ChatRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
	})
	if err != nil {
		if !fromOffline && offlineQueue != nil && userText != "" {
			if enqueueErr := offlineQueue.EnqueueReplay(channelID, userText); enqueueErr != nil {
				b.emitError(st, channelID, fmt.Errorf("provider backend: queue offline message: %w", enqueueErr))
				return
			}
			b.emitErrorWithOffline(st, channelID, err, true)
			return
		}
		b.emitError(st, channelID, err)
		return
	}

	var sb strings.Builder
	for ev := range ch {
		switch ev.Type {
		case StreamEventText:
			sb.WriteString(ev.Text)
			b.emitAssistantDelta(st, channelID, sb.String())
		case StreamEventUsage:
			b.emitUsage(st, channelID, ev.Usage)
		case StreamEventError:
			if !fromOffline && offlineQueue != nil && userText != "" {
				if enqueueErr := offlineQueue.EnqueueReplay(channelID, userText); enqueueErr != nil {
					b.emitError(st, channelID, fmt.Errorf("provider backend: queue offline message: %w", enqueueErr))
					return
				}
				b.emitErrorWithOffline(st, channelID, ev.Err, true)
				return
			}
			b.emitError(st, channelID, ev.Err)
			return
		case StreamEventDone:
			b.emitDone(st, channelID, sb.String())
			return
		}
	}

	// Provider 流意外关闭但没有 done 事件时，尽量保留已生成的文本。
	if sb.Len() > 0 {
		b.emitDone(st, channelID, sb.String())
		return
	}
	b.emitError(st, channelID, errors.New("provider backend: stream closed without done event"))
}

func (b *Backend) emitAssistantDelta(st *sessionState, channelID, content string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if st.closed {
		return
	}
	applyAssistantContentLocked(st, content)
	st.seq++
	ev := client.Event{
		Operation: "event",
		ChannelID: channelID,
		Version:   st.seq,
		State:     b.stateLocked(st),
	}
	b.broadcastLocked(ev, st)
}

func (b *Backend) emitUsage(st *sessionState, channelID string, usage *Usage) {
	if usage == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if st.closed {
		return
	}
	if st.tokenUsage == nil {
		st.tokenUsage = map[string]any{}
	}
	st.tokenUsage["inputTokens"] = usage.InputTokens
	st.tokenUsage["outputTokens"] = usage.OutputTokens
	st.tokenUsage["cacheReadTokens"] = usage.CacheReadTokens
	st.tokenUsage["cacheWriteTokens"] = usage.CacheWriteTokens
	st.tokenUsage["totalTokens"] = usage.InputTokens + usage.OutputTokens
	if st.cost == nil {
		st.cost = map[string]any{}
	}
	if b.provider != nil && b.provider.CostEstimator() != nil {
		st.cost["estimatedYuan"] = b.provider.CostEstimator().Estimate(
			usage.InputTokens,
			usage.OutputTokens,
			usage.CacheReadTokens,
			usage.CacheWriteTokens,
		)
	} else {
		st.cost["estimatedYuan"] = 0.0
	}
	st.seq++
	ev := client.Event{
		Operation: "event",
		ChannelID: channelID,
		Version:   st.seq,
		State:     b.stateLocked(st),
	}
	b.broadcastLocked(ev, st)
}

func (b *Backend) emitDone(st *sessionState, channelID, content string) {
	b.mu.Lock()
	if st.closed {
		b.mu.Unlock()
		return
	}
	if content != "" {
		applyAssistantContentLocked(st, content)
	}
	st.seq++
	ev := client.Event{
		Operation: "done",
		ChannelID: channelID,
		Version:   st.seq,
		State:     b.stateLocked(st),
	}
	b.broadcastLocked(ev, st)
	workspace := st.workspace
	messages := append([]client.Message(nil), st.messages...)
	contextMap := cloneMap(st.context)
	costMap := cloneMap(st.cost)
	b.mu.Unlock()

	b.persist(channelID, workspace, messages, contextMap, costMap)
}

func (b *Backend) emitError(st *sessionState, channelID string, err error) {
	b.emitErrorWithOffline(st, channelID, err, false)
}

func (b *Backend) emitErrorWithOffline(st *sessionState, channelID string, err error, offlineQueued bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if st.closed {
		return
	}
	if st.cost == nil {
		st.cost = map[string]any{}
	}
	st.cost["lastError"] = err.Error()
	st.seq++
	state := b.stateLocked(st)
	if offlineQueued {
		state["offline_queued"] = true
	}
	ev := client.Event{
		Operation: "error",
		ChannelID: channelID,
		Version:   st.seq,
		State:     state,
	}
	b.broadcastLocked(ev, st)
}

func (b *Backend) closeStateLocked(st *sessionState) {
	if st.cancel != nil {
		st.cancel()
		st.cancel = nil
	}
	st.closed = true
	for ch := range st.subs {
		close(ch)
	}
	st.subs = make(map[chan client.Event]struct{})
}

func (b *Backend) broadcastLocked(ev client.Event, st *sessionState) {
	for ch := range st.subs {
		if ev.Operation == "done" || ev.Operation == "error" {
			// Terminal events must not be dropped: block until the subscriber
			// is ready to receive them so UI/headless cannot miss completion.
			ch <- ev
			continue
		}
		select {
		case ch <- ev:
		default:
		}
	}
}

func (b *Backend) stateLocked(st *sessionState) map[string]any {
	return map[string]any{
		"messages":   messagesToAny(st.messages),
		"context":    cloneMap(st.context),
		"cost":       cloneMap(st.cost),
		"tokenUsage": cloneMap(st.tokenUsage),
		"diffs":      []any{},
		"todos":      []any{},
		"tools":      []any{},
		"model":      b.model,
		"provider":   b.providerName(),
	}
}

func (b *Backend) providerName() string {
	if b == nil || b.provider == nil {
		return ""
	}
	return b.provider.Name()
}

func (b *Backend) persist(channelID, workspace string, messages []client.Message, contextMap, costMap map[string]any) {
	if b == nil || b.store == nil || channelID == "" {
		return
	}
	_ = b.store.Save(session.Entry{
		ID:        channelID,
		Workspace: workspace,
		Title:     workspace,
		UpdatedAt: time.Now(),
		Messages:  messages,
		Context:   contextMap,
		Cost:      costMap,
	})
}

func applyAssistantContentLocked(st *sessionState, content string) {
	if n := len(st.messages); n > 0 && st.messages[n-1].Role == "assistant" {
		st.messages[n-1].Content = content
		return
	}
	st.messages = append(st.messages, client.Message{Role: "assistant", Content: content})
}

func messagesToAny(msgs []client.Message) []any {
	out := make([]any, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content})
	}
	return out
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
	return ""
}

// ProviderOptions 描述从命令行创建直连 Provider 的参数。
type ProviderOptions struct {
	Name    string // openai / deepseek / moonshot / kimi / zhipu / qwen / ollama
	APIKey  string // 多个 key 用逗号分隔
	BaseURL string // 可选，默认使用厂商地址
	Model   string // 可选，默认使用厂商常用模型
}

// NewProvider 根据名称创建 OpenAI 兼容的 Provider。
func NewProvider(opts ProviderOptions) (Provider, error) {
	name := strings.ToLower(strings.TrimSpace(opts.Name))
	if name == "" {
		return nil, errors.New("provider: provider name is empty")
	}
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL(name)
		if baseURL == "" {
			return nil, fmt.Errorf("provider: unknown provider %q", opts.Name)
		}
	}
	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = defaultModel(name)
	}

	keys := splitKeys(opts.APIKey)
	providers := make([]Provider, 0, len(keys))
	if len(keys) == 0 {
		keys = []string{""}
	}
	for _, key := range keys {
		p := NewOpenAI(OpenAIConfig{
			BaseURL: baseURL,
			APIKey:  key,
			Model:   model,
		})
		providers = append(providers, &namedProvider{Provider: p, name: name})
	}

	if len(providers) == 1 {
		return providers[0], nil
	}
	return &rotatingProvider{providers: providers}, nil
}

type namedProvider struct {
	Provider
	name string
}

func (n *namedProvider) Name() string {
	if n == nil || n.name == "" {
		if n.Provider != nil {
			return n.Provider.Name()
		}
		return ""
	}
	return n.name
}

type rotatingProvider struct {
	mu        sync.Mutex
	providers []Provider
	nextIdx   int
}

func (r *rotatingProvider) Name() string {
	if r == nil || len(r.providers) == 0 {
		return ""
	}
	return r.providers[0].Name()
}

func (r *rotatingProvider) Model() string {
	if r == nil || len(r.providers) == 0 {
		return ""
	}
	return r.providers[0].Model()
}

func (r *rotatingProvider) CostEstimator() *cost.Estimator {
	if r == nil || len(r.providers) == 0 {
		return cost.NewEstimator()
	}
	return r.providers[0].CostEstimator()
}

func (r *rotatingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p, err := r.nextProvider()
	if err != nil {
		return nil, err
	}
	return p.Chat(ctx, req)
}

func (r *rotatingProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	p, err := r.nextProvider()
	if err != nil {
		return nil, err
	}
	return p.StreamChat(ctx, req)
}

func (r *rotatingProvider) nextProvider() (Provider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.providers) == 0 {
		return nil, ErrNoProvider
	}
	p := r.providers[r.nextIdx%len(r.providers)]
	r.nextIdx = (r.nextIdx + 1) % len(r.providers)
	return p, nil
}

func splitKeys(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultBaseURL(name string) string {
	switch name {
	case "openai":
		return "https://api.openai.com/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "moonshot", "kimi":
		return "https://api.moonshot.cn/v1"
	case "zhipu":
		return "https://open.bigmodel.cn/api/paas/v4"
	case "qwen":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return ""
	}
}

func defaultModel(name string) string {
	switch name {
	case "openai":
		return "gpt-4o-mini"
	case "deepseek":
		return "deepseek-chat"
	case "moonshot":
		return "moonshot-v1-8k"
	case "kimi":
		return "kimi-k2"
	case "zhipu":
		return "glm-4-flash"
	case "qwen":
		return "qwen-plus"
	case "ollama":
		return "llama3"
	default:
		return ""
	}
}
