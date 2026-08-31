package provider

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/offline"
)

type backendFakeProvider struct {
	streamErr error
}

func (p *backendFakeProvider) Name() string                   { return "fake" }
func (p *backendFakeProvider) Model() string                  { return "fake-model" }
func (p *backendFakeProvider) CostEstimator() *cost.Estimator { return cost.NewEstimator() }

func (p *backendFakeProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{}, nil
}

func (p *backendFakeProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: StreamEventDone}
	close(ch)
	return ch, nil
}

func TestBackendStreamErrorQueuesOffline(t *testing.T) {
	q := offline.NewQueue(filepath.Join(t.TempDir(), "queue.json"))
	p := &backendFakeProvider{streamErr: errors.New("network down")}
	b := NewBackendWithOffline(p, nil, q)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	events, err := b.Subscribe(ctx, "sess", 0)
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"hello offline"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	item := waitForOfflineItem(t, q)
	if item.SessionID != "sess" {
		t.Fatalf("offline item SessionID = %q, want %q", item.SessionID, "sess")
	}
	if item.Message != "hello offline" {
		t.Fatalf("offline item Message = %q, want %q", item.Message, "hello offline")
	}
	if !item.Replay {
		t.Fatalf("offline item Replay = false, want true because the user message is already in the session")
	}

	ev := waitForOfflineEvent(t, events)
	if ev.Operation != "error" {
		t.Fatalf("offline event Operation = %q, want %q", ev.Operation, "error")
	}
	if ev.State["offline_queued"] != true {
		t.Fatalf("offline event offline_queued = %#v, want true", ev.State["offline_queued"])
	}
	costState, ok := ev.State["cost"].(map[string]any)
	if !ok {
		t.Fatalf("offline event state has no cost map: %#v", ev.State["cost"])
	}
	if costState["lastError"] != "network down" {
		t.Fatalf("offline event lastError = %#v, want %q", costState["lastError"], "network down")
	}
}

func TestBackendFlushOfflineRemovesQueuedItem(t *testing.T) {
	q := offline.NewQueue(filepath.Join(t.TempDir(), "queue.json"))
	p := &backendFakeProvider{streamErr: errors.New("network down")}
	b := NewBackend(p, nil)
	b.SetOfflineQueue(q)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"hello retry"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	item := waitForOfflineItem(t, q)
	if item.Message != "hello retry" {
		t.Fatalf("offline item Message = %q, want %q", item.Message, "hello retry")
	}
	waitForStreamingDone(t, b, "sess")

	// Provider 恢复后，FlushOffline 应重发消息并从队列移除。
	p.streamErr = nil
	if err := b.FlushOffline(ctx); err != nil {
		t.Fatalf("FlushOffline() error: %v", err)
	}

	b.mu.Lock()
	st := b.sessions["sess"]
	if st == nil {
		b.mu.Unlock()
		t.Fatal("session was removed unexpectedly")
	}
	if len(st.messages) != 1 {
		b.mu.Unlock()
		t.Fatalf("len(st.messages) after replay = %d, want 1 (no duplicate user message)", len(st.messages))
	}
	b.mu.Unlock()

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() after flush error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(Pending()) after flush = %d, want 0; items = %+v", len(items), items)
	}
}

func TestBackendFlushOfflineKeepsItemWhenRetryFails(t *testing.T) {
	q := offline.NewQueue(filepath.Join(t.TempDir(), "queue.json"))
	p := &backendFakeProvider{streamErr: errors.New("still down")}
	b := NewBackend(p, nil)
	b.SetOfflineQueue(q)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"keep me"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	item := waitForOfflineItem(t, q)
	if !item.Replay {
		t.Fatalf("offline item Replay = false, want true")
	}
	waitForStreamingDone(t, b, "sess")

	// Provider 仍然失败：FlushOffline 应返回错误且保留队列项。
	if err := b.FlushOffline(ctx); err == nil {
		t.Fatal("FlushOffline() error = nil, want error when retry still fails")
	}

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() after failed flush error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(Pending()) after failed flush = %d, want 1", len(items))
	}
	if items[0].ID != item.ID || items[0].Message != "keep me" {
		t.Fatalf("unexpected retained item: %+v", items[0])
	}

	b.mu.Lock()
	st := b.sessions["sess"]
	if st == nil {
		b.mu.Unlock()
		t.Fatal("session was removed unexpectedly")
	}
	if len(st.messages) != 1 {
		b.mu.Unlock()
		t.Fatalf("len(st.messages) after failed replay = %d, want 1 (no duplicate)", len(st.messages))
	}
	b.mu.Unlock()
}

func TestBackendFlushOfflineReopensClosedSession(t *testing.T) {
	q := offline.NewQueue(filepath.Join(t.TempDir(), "queue.json"))
	p := &backendFakeProvider{streamErr: errors.New("network down")}
	b := NewBackend(p, nil)
	b.SetOfflineQueue(q)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"reopen me"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	item := waitForOfflineItem(t, q)
	if !item.Replay {
		t.Fatalf("offline item Replay = false, want true")
	}
	waitForStreamingDone(t, b, "sess")

	if err := b.Close(ctx, "sess"); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// 会话已从内存中移除；恢复后 FlushOffline 应重新打开会话并补上用户消息。
	p.streamErr = nil
	if err := b.FlushOffline(ctx); err != nil {
		t.Fatalf("FlushOffline() error: %v", err)
	}

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() after flush error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(Pending()) after flush = %d, want 0; items = %+v", len(items), items)
	}

	b.mu.Lock()
	st := b.sessions["sess"]
	if st == nil {
		b.mu.Unlock()
		t.Fatal("reopened session not present after flush")
	}
	if len(st.messages) != 1 || st.messages[0].Role != "user" || st.messages[0].Content != "reopen me" {
		b.mu.Unlock()
		t.Fatalf("reopened session messages = %+v, want one user message", st.messages)
	}
	b.mu.Unlock()
}

func waitForOfflineItem(t *testing.T, q *offline.Queue) offline.Item {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		items, err := q.Pending()
		if err != nil {
			t.Fatalf("Pending() error: %v", err)
		}
		if len(items) > 0 {
			return items[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for offline queue item")
	return offline.Item{}
}

func waitForOfflineEvent(t *testing.T, events <-chan client.Event) client.Event {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event channel closed before offline event")
			}
			if v, ok := ev.State["offline_queued"].(bool); ok && v {
				return ev
			}
		case <-deadline:
			t.Fatal("timed out waiting for offline_queued event")
		}
	}
}

func waitForStreamingDone(t *testing.T, b *Backend, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		st := b.sessions[sessionID]
		done := st == nil || !st.streaming
		b.mu.Unlock()
		if done {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for streaming to finish")
}

func TestBackendBroadcastLockedDoesNotDropTerminalEvent(t *testing.T) {
	for _, op := range []string{"done", "error"} {
		t.Run(op, func(t *testing.T) {
			b := NewBackend(nil, nil)
			ch := make(chan client.Event, 1)
			// Fill the subscriber buffer so a non-blocking send would drop the
			// terminal event.
			ch <- client.Event{Operation: "event", ChannelID: "sess", Version: 1}
			st := &sessionState{
				subs: map[chan client.Event]struct{}{
					ch: {},
				},
			}

			finished := make(chan struct{})
			go func() {
				defer close(finished)
				b.broadcastLocked(client.Event{Operation: op, ChannelID: "sess", Version: 2}, st)
			}()

			select {
			case ev := <-ch:
				if ev.Operation != "event" {
					t.Fatalf("first event Operation = %q, want %q", ev.Operation, "event")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("timed out reading buffered event")
			}

			select {
			case ev := <-ch:
				if ev.Operation != op {
					t.Fatalf("terminal event Operation = %q, want %q", ev.Operation, op)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("terminal %q event was dropped or not delivered", op)
			}

			select {
			case <-finished:
			case <-time.After(2 * time.Second):
				t.Fatalf("broadcastLocked did not finish after %q event was delivered", op)
			}
		})
	}
}
