package dshapi

import (
	"encoding/json"
	"testing"
)

func TestFrameToEventUpdatesLastSeq(t *testing.T) {
	b := NewBackend(New("http://example.invalid"))

	payload, err := json.Marshal(map[string]any{
		"type":      "session/event",
		"sessionId": "s1",
		"event": map[string]any{
			"type": "user/message",
			"seq":  float64(7),
			"time": float64(1),
			"data": map[string]any{"content": "hello"},
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	ev := b.frameToEvent(ServerRequest{Type: "server-request", Payload: payload}, "s1")
	if ev == nil {
		t.Fatal("expected event")
	}
	if ev.Version != 7 {
		t.Fatalf("expected event version 7, got %d", ev.Version)
	}
	if b.lastSeq != 7 {
		t.Fatalf("expected lastSeq 7, got %d", b.lastSeq)
	}

	// 旧 seq 不应回退 lastSeq。
	payload2, _ := json.Marshal(map[string]any{
		"type":      "session/event",
		"sessionId": "s1",
		"event": map[string]any{
			"type": "user/message",
			"seq":  float64(5),
			"time": float64(1),
			"data": map[string]any{"content": "old"},
		},
	})
	_ = b.frameToEvent(ServerRequest{Type: "server-request", Payload: payload2}, "s1")
	if b.lastSeq != 7 {
		t.Fatalf("lastSeq should not go backwards, got %d", b.lastSeq)
	}
}
