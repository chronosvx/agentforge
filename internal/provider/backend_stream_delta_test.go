package provider

import (
	"context"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
)

type streamingBackendProvider struct {
	backendFakeProvider
	texts []string
}

func (p *streamingBackendProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(p.texts)+1)
	for _, text := range p.texts {
		ch <- StreamEvent{Type: StreamEventText, Text: text}
	}
	ch <- StreamEvent{Type: StreamEventDone}
	close(ch)
	return ch, nil
}

func TestBackendStreamEmitsDeltaWithoutFullStateAndDoneWithFullState(t *testing.T) {
	p := &streamingBackendProvider{texts: []string{"Hel", "lo"}}
	b := NewBackend(p, nil)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	events, err := b.Subscribe(ctx, "sess", 0)
	if err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}
	if _, err := b.Invoke(ctx, "sess", "chat", []any{"hi"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	// 用户消息仍以完整 state 事件广播。
	ev := readProviderEvent(t, events)
	if ev.Operation != "event" {
		t.Fatalf("user message Operation = %q, want %q", ev.Operation, "event")
	}
	if ev.State == nil {
		t.Fatal("user message event should include full state")
	}

	// 每个文本增量只广播 delta，不携带完整 messages。
	for i, want := range []string{"Hel", "lo"} {
		ev := readProviderEvent(t, events)
		if ev.Operation != "delta" {
			t.Fatalf("delta %d Operation = %q, want %q", i, ev.Operation, "delta")
		}
		if ev.State != nil {
			t.Fatalf("delta %d unexpectedly contains full state: %#v", i, ev.State)
		}
		value, ok := ev.Value.(map[string]any)
		if !ok {
			t.Fatalf("delta %d Value type = %T, want map[string]any", i, ev.Value)
		}
		if got, _ := value["content"].(string); got != want {
			t.Fatalf("delta %d content = %q, want %q", i, got, want)
		}
		if typ, _ := value["type"].(string); typ != "text" {
			t.Fatalf("delta %d type = %q, want %q", i, typ, "text")
		}
	}

	// done 事件仍携带最终全量 state，便于不处理 delta 的订阅者同步。
	ev = readProviderEvent(t, events)
	if ev.Operation != "done" {
		t.Fatalf("terminal Operation = %q, want %q", ev.Operation, "done")
	}
	if ev.State == nil {
		t.Fatal("done event should include full state")
	}
	msgs, _ := ev.State["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("done messages length = %d, want 2; state=%#v", len(msgs), ev.State)
	}
	last, _ := msgs[len(msgs)-1].(map[string]any)
	if last["role"] != "assistant" || last["content"] != "Hello" {
		t.Fatalf("done final message = %#v, want assistant %q", last, "Hello")
	}
}

func readProviderEvent(t *testing.T, events <-chan client.Event) client.Event {
	t.Helper()
	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("event channel closed before expected event")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for event")
		return client.Event{}
	}
}
