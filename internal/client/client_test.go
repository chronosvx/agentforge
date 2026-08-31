package client_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/mock"
)

func TestClientOpenSubscribeInvokeClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := mock.Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start mock server: %v", err)
	}
	defer srv.Close()

	endpoint := "ws://" + srv.Addr() + "/agentforge/v1/connection"
	cl, err := client.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.CloseConn()

	snap, err := cl.Open(ctx, "workspace-a", "session-1", map[string]any{"mode": "test"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if snap.ChannelID == "" {
		t.Fatal("open returned empty channel id")
	}

	events, err := cl.Subscribe(ctx, snap.ChannelID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	result, err := cl.Invoke(ctx, snap.ChannelID, "chat", []any{"hello"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if result == nil {
		t.Fatal("invoke returned nil result")
	}

	// 收集事件直到看到 assistant 最终回复。
	deadline := time.After(3 * time.Second)
	sawAssistant := false
	for !sawAssistant {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("event channel closed early")
			}
			msgs, _ := ev.State["messages"].([]any)
			for _, m := range msgs {
				mm, _ := m.(map[string]any)
				if mm["role"] == "assistant" {
					content, _ := mm["content"].(string)
					if strings.Contains(content, "reply") {
						sawAssistant = true
						break
					}
				}
			}
		case <-deadline:
			t.Fatal("timed out waiting for assistant event")
		}
	}

	if err := cl.Close(ctx, snap.ChannelID); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestDefaultEndpoint(t *testing.T) {
	if client.DefaultEndpoint != "ws://127.0.0.1:18891/agentforge/v1/connection" {
		t.Fatalf("unexpected default endpoint: %s", client.DefaultEndpoint)
	}
}

func TestClientResumeSameSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := mock.Start(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start mock server: %v", err)
	}
	defer srv.Close()

	endpoint := "ws://" + srv.Addr() + "/agentforge/v1/connection"
	cl, err := client.Dial(ctx, endpoint)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cl.CloseConn()

	const sessionID = "resume-me"
	snap1, err := cl.Open(ctx, "workspace-a", sessionID, nil)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := cl.Invoke(ctx, snap1.ChannelID, "chat", []any{"hello"}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	snap2, err := cl.Open(ctx, "workspace-a", sessionID, nil)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if snap2.ChannelID == "" {
		t.Fatal("second open returned empty channel id")
	}
	messages, _ := snap2.State["messages"].([]any)
	if len(messages) == 0 {
		t.Fatal("expected resumed session to contain messages")
	}
}
