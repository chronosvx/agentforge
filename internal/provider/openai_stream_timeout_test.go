package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIStreamChatSurvivesSlowBodyBeyondClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		flusher.Flush()

		// The configured client Timeout is far shorter than this delay. A stream
		// client with http.Client.Timeout set would kill the SSE body read here.
		time.Sleep(300 * time.Millisecond)

		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"late\"}}]}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{
		BaseURL: srv.URL,
		APIKey:  "key",
		Model:   "m",
		Timeout: 100 * time.Millisecond,
	})
	ch, err := p.StreamChat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat() error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var got string
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("stream closed before done event")
			}
			switch ev.Type {
			case StreamEventText:
				got += ev.Text
			case StreamEventDone:
				if got != "late" {
					t.Fatalf("text = %q, want %q", got, "late")
				}
				return
			case StreamEventError:
				t.Fatalf("stream error event: %v", ev.Err)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for slow SSE stream")
		}
	}
}
