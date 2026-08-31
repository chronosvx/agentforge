package dshapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/coder/websocket"
)

func TestBackendOpenSubscribeInvoke(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/api/session.create", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{"sessionId": "real-session"})
	})
	mux.HandleFunc("/api/session.list", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{
			"items": []map[string]any{
				{"sessionId": "real-session", "updatedAt": 1, "running": false, "blank": false},
			},
		})
	})
	mux.HandleFunc("/api/session.history", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{"events": []any{}, "hasMore": false})
	})
	mux.HandleFunc("/api/session.prompt", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{"accepted": true})
	})
	mux.HandleFunc("/api/events.mux", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "test done")

		todoFrame := map[string]any{
			"type": "server-request", "rpcId": "rpc-mux", "method": "events.mux",
			"payload": map[string]any{
				"type": "session/event", "sessionId": "real-session",
				"event": map[string]any{
					"type": "tool/call", "seq": 1, "time": 1,
					"data": map[string]any{
						"name":      "todo_write",
						"arguments": `{"todos":[{"content":"task A","status":"in_progress"}]}`,
					},
				},
			},
		}
		todoData, _ := json.Marshal(todoFrame)
		_ = conn.Write(context.Background(), websocket.MessageText, todoData)

		diffFrame := map[string]any{
			"type": "server-request", "rpcId": "rpc-mux", "method": "events.mux",
			"payload": map[string]any{
				"type": "session/event", "sessionId": "real-session",
				"event": map[string]any{
					"type": "tool/result", "seq": 2, "time": 1,
					"data": map[string]any{
						"message": map[string]any{
							"content": []any{
								map[string]any{
									"content": []any{
										map[string]any{"type": "text", "text": "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new"},
									},
								},
							},
						},
					},
				},
			},
		}
		diffData, _ := json.Marshal(diffFrame)
		_ = conn.Write(context.Background(), websocket.MessageText, diffData)

		for i, text := range []string{"hello ", "world"} {
			frame := map[string]any{
				"type": "server-request", "rpcId": "rpc-mux", "method": "events.mux",
				"payload": map[string]any{
					"type": "session/event", "sessionId": "real-session",
					"event": map[string]any{
						"type": "assistant/chunk", "seq": 3 + i, "time": 1,
						"data": map[string]any{
							"chunk": map[string]any{"type": "text-delta", "text": text},
						},
					},
				},
			}
			data, _ := json.Marshal(frame)
			_ = conn.Write(context.Background(), websocket.MessageText, data)
		}
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	api := dshapi.New(srv.URL)
	backend := dshapi.NewBackend(api)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	snap, err := backend.Open(ctx, "/tmp", "", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if snap.ChannelID != "real-session" {
		t.Fatalf("unexpected channel id: %q", snap.ChannelID)
	}

	sessions, err := backend.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "real-session" {
		t.Fatalf("unexpected remote sessions: %#v", sessions)
	}

	events, err := backend.Subscribe(ctx, snap.ChannelID, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// 读取 tool/call 事件，确认 todos 被解析。
	select {
	case ev := <-events:
		todos, _ := ev.State["todos"].([]any)
		if len(todos) != 1 {
			t.Fatalf("expected todos from tool/call, got %#v", ev.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool/call event")
	}

	// 读取 tool/result 事件，确认 git diff 被解析。
	select {
	case ev := <-events:
		diffs, _ := ev.State["diffs"].([]any)
		if len(diffs) != 1 {
			t.Fatalf("expected diffs from tool/result, got %#v", ev.State)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tool/result event")
	}

	// 读取两个 text-delta 事件，确认流式内容被累积为 "hello world"。
	var lastContent string
	for i := 0; i < 2; i++ {
		select {
		case ev := <-events:
			msgs, _ := ev.State["messages"].([]any)
			if len(msgs) == 0 {
				t.Fatalf("expected assistant message in event state, got %#v", ev.State)
			}
			lastContent, _ = msgs[len(msgs)-1].(map[string]any)["content"].(string)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for event")
		}
	}
	if lastContent != "hello world" {
		t.Fatalf("unexpected streamed content: %q", lastContent)
	}

	if _, err := backend.Invoke(ctx, snap.ChannelID, "chat", []any{"hello"}); err != nil {
		t.Fatalf("invoke: %v", err)
	}
}
