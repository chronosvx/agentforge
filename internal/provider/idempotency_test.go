package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDUniqueAndFormat(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := RequestID()
		if id == "" {
			t.Fatal("RequestID() returned empty string")
		}
		if len(id) != 36 {
			t.Fatalf("RequestID() length = %d, want 36 (UUID v4): %q", len(id), id)
		}
		if strings.Count(id, "-") != 4 {
			t.Fatalf("RequestID() = %q, want 4 hyphens", id)
		}
		if seen[id] {
			t.Fatalf("RequestID() returned duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestWithRequestID(t *testing.T) {
	base := ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}
	got := WithRequestID(base, "req-123")

	if got.Headers[RequestIDHeader] != "req-123" {
		t.Fatalf("Headers[%q] = %q, want req-123", RequestIDHeader, got.Headers[RequestIDHeader])
	}
	if base.Headers != nil {
		t.Fatalf("original ChatRequest was mutated: %#v", base.Headers)
	}
}

func TestOpenAIChatSetsRequestIDHeader(t *testing.T) {
	var gotRequestID, gotIdempotencyKey string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get(RequestIDHeader)
		gotIdempotencyKey = r.Header.Get(IdempotencyKeyHeader)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-1",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m", IdempotencyKey: "idem-1"})
	if _, err := p.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if gotRequestID != "idem-1" {
		t.Fatalf("X-Request-Id = %q, want idem-1 when IdempotencyKey is configured", gotRequestID)
	}
	if gotIdempotencyKey != "idem-1" {
		t.Fatalf("Idempotency-Key = %q, want idem-1", gotIdempotencyKey)
	}
}

func TestOpenAIWithRequestIDHeaderUsed(t *testing.T) {
	var gotRequestID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get(RequestIDHeader)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-1",
			"choices": []map[string]any{
				{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m"})
	req := WithRequestID(ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, "fixed-request-id")
	if _, err := p.Chat(context.Background(), req); err != nil {
		t.Fatalf("Chat() error: %v", err)
	}

	if gotRequestID != "fixed-request-id" {
		t.Fatalf("X-Request-Id = %q, want fixed-request-id", gotRequestID)
	}
}

func TestOpenAIStreamChatSetsRequestIDHeader(t *testing.T) {
	var gotRequestID string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get(RequestIDHeader)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m"})
	ch, err := p.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("StreamChat() error: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventError {
			t.Fatalf("stream error: %v", ev.Err)
		}
	}

	if gotRequestID == "" {
		t.Fatal("X-Request-Id header is empty for stream request")
	}
}
