package dshapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/coder/websocket"
)

func TestBackendSubscribeBackfillsHistoryAndDedups(t *testing.T) {
	var historyCalls atomic.Int32

	var mux http.ServeMux
	mux.HandleFunc("/api/session.history", func(w http.ResponseWriter, r *http.Request) {
		call := historyCalls.Add(1)
		switch call {
		case 1:
			// Open 时看到的最新历史只到 seq=2。
			writeServerResponse(t, w, r, map[string]any{
				"events": []any{
					historyEvent(1, "user/message", map[string]any{"content": "old-1"}),
					historyEvent(2, "user/message", map[string]any{"content": "old-2"}),
				},
				"hasMore": false,
			})
		default:
			// Subscribe(after=2) 时发现 seq=3..5 已经产生。
			// 故意包含 seq=2 和 seq=3..5 的重复，验证按 seq 去重。
			writeServerResponse(t, w, r, map[string]any{
				"events": []any{
					historyEvent(2, "user/message", map[string]any{"content": "old-2"}),
					historyEvent(3, "user/message", map[string]any{"content": "three"}),
					historyEvent(4, "user/message", map[string]any{"content": "four"}),
					historyEvent(5, "user/message", map[string]any{"content": "five"}),
				},
				"hasMore": false,
			})
		}
	})
	mux.HandleFunc("/api/events.mux", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "test done")

		// mux 也会重放 seq=3..5（与 history 重复），随后是 seq=6 的新事件。
		for _, seq := range []int{3, 4, 5, 6} {
			text := map[int]string{3: "three", 4: "four", 5: "five", 6: "six"}[seq]
			frame := map[string]any{
				"type": "server-request", "rpcId": "rpc-mux", "method": "events.mux",
				"payload": map[string]any{
					"type": "session/event", "sessionId": "seq-session",
					"event": map[string]any{
						"type": "user/message", "seq": seq, "time": 1,
						"data": map[string]any{"content": text},
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

	snap, err := backend.Open(ctx, "/tmp", "seq-session", nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if snap.Version != 2 {
		t.Fatalf("expected open snapshot version 2, got %d", snap.Version)
	}

	events, err := backend.Subscribe(ctx, snap.ChannelID, snap.Version)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	var versions []int64
	var last client.Event
	for len(versions) < 4 {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed early, got %v", versions)
			}
			versions = append(versions, ev.Version)
			last = ev
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for events, got %v", versions)
		}
	}

	want := []int64{3, 4, 5, 6}
	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("unexpected versions %v, want %v", versions, want)
		}
	}

	// 最终状态应包含 old-1、old-2、three、four、five、six，且没有重复。
	msgs, _ := last.State["messages"].([]any)
	contents := make([]string, 0, len(msgs))
	for _, m := range msgs {
		mm, _ := m.(map[string]any)
		contents = append(contents, mm["content"].(string))
	}
	wantContents := []string{"old-1", "old-2", "three", "four", "five", "six"}
	if len(contents) != len(wantContents) {
		t.Fatalf("unexpected messages %#v", contents)
	}
	for i := range wantContents {
		if contents[i] != wantContents[i] {
			t.Fatalf("unexpected messages %#v, want %#v", contents, wantContents)
		}
	}
}

func historyEvent(seq int64, typ string, data map[string]any) map[string]any {
	return map[string]any{
		"event": map[string]any{
			"type": typ,
			"seq":  seq,
			"time": int64(1),
			"data": data,
		},
	}
}
