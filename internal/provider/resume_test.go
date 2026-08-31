package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type resumeStubProvider struct {
	stubProvider
	gotReq    ChatRequest
	gotOffset int
	ch        chan StreamEvent
}

func (s *resumeStubProvider) ResumeStream(ctx context.Context, req ChatRequest, lastOffset int) (<-chan StreamEvent, error) {
	s.gotReq = req
	s.gotOffset = lastOffset
	if s.ch == nil {
		s.ch = make(chan StreamEvent)
		close(s.ch)
	}
	return s.ch, nil
}

func TestStreamResumerRecordsOffsets(t *testing.T) {
	r := NewStreamResumer(nil)

	r.Record(StreamEvent{Type: StreamEventText, Text: "hello"})
	r.Record(StreamEvent{Type: StreamEventToolCall, ToolCall: &ToolCall{ID: "call_1"}})
	r.Record(StreamEvent{Type: StreamEventText, Text: " world"})
	r.Record(StreamEvent{Type: StreamEventDone})

	if got := r.EventIndex(); got != 4 {
		t.Fatalf("EventIndex() = %d, want 4", got)
	}
	if got := r.CharOffset(); got != 11 {
		t.Fatalf("CharOffset() = %d, want 11", got)
	}
	if got := r.Offset(); got != 11 {
		t.Fatalf("Offset() = %d, want 11", got)
	}

	r.Reset()
	if r.EventIndex() != 0 || r.CharOffset() != 0 {
		t.Fatalf("after Reset: EventIndex=%d CharOffset=%d, want 0/0", r.EventIndex(), r.CharOffset())
	}
}

func TestStreamResumerResumeUnsupported(t *testing.T) {
	r := NewStreamResumer(&stubProvider{name: "plain"})
	_, err := r.Resume(context.Background(), ChatRequest{}, 1)
	if !errors.Is(err, ErrResumeUnsupported) {
		t.Fatalf("Resume() error = %v, want ErrResumeUnsupported", err)
	}
}

func TestStreamResumerResumeSupported(t *testing.T) {
	sp := &resumeStubProvider{}
	sp.ch = make(chan StreamEvent, 1)
	sp.ch <- StreamEvent{Type: StreamEventDone}
	close(sp.ch)

	r := NewStreamResumer(sp)
	ch, err := r.Resume(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, 42)
	if err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	if ch == nil {
		t.Fatal("Resume() returned nil channel")
	}
	if sp.gotOffset != 42 {
		t.Fatalf("Resume() passed offset = %d, want 42", sp.gotOffset)
	}
	if sp.gotReq.Headers == nil || sp.gotReq.Headers[ResumeOffsetHeader] != "42" {
		t.Fatalf("Resume() request headers = %#v, want X-Resume-Offset=42", sp.gotReq.Headers)
	}
}

func TestOpenAIResumeStreamSetsOffsetHeader(t *testing.T) {
	var gotOffset string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOffset = r.Header.Get(ResumeOffsetHeader)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := NewOpenAI(OpenAIConfig{BaseURL: srv.URL, APIKey: "key", Model: "m"})
	r := NewStreamResumer(p)
	ch, err := r.Resume(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}}, 42)
	if err != nil {
		t.Fatalf("Resume() error: %v", err)
	}
	for ev := range ch {
		if ev.Type == StreamEventError {
			t.Fatalf("stream error: %v", ev.Err)
		}
	}

	if gotOffset != "42" {
		t.Fatalf("X-Resume-Offset = %q, want 42", gotOffset)
	}
}
