package provider

import (
	"context"
	"errors"
	"testing"
)

func TestFailoverChatSwitchesOnRetryableError(t *testing.T) {
	a := &stubProvider{name: "a", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, &ProviderError{Kind: ServerError, Message: "boom", Retryable: true}
	}}
	b := &stubProvider{name: "b", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	}}

	r := NewRouter()
	r.Add(a, 10)
	r.Add(b, 5)
	fr := NewFailoverRouter(r)

	got, err := fr.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.Content != "ok" {
		t.Fatalf("Chat.Content = %q, want ok", got.Content)
	}
}

func TestFailoverChatSwitchesOnProviderErrorEvenNonRetryable(t *testing.T) {
	a := &stubProvider{name: "a", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, &ProviderError{Kind: AuthError, Message: "bad key", Retryable: false}
	}}
	b := &stubProvider{name: "b", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	}}

	fr := NewFailoverRouter(a, b)
	got, err := fr.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got.Content != "ok" {
		t.Fatalf("Chat.Content = %q, want ok", got.Content)
	}
}

func TestFailoverChatAllFailedReturnsAggregate(t *testing.T) {
	a := &stubProvider{name: "a", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, &ProviderError{Kind: ServerError, Message: "a down", Retryable: true}
	}}
	b := &stubProvider{name: "b", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, &ProviderError{Kind: ServerError, Message: "b down", Retryable: true}
	}}

	fr := NewFailoverRouterWithProviders(a, b)
	_, err := fr.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	var agg *AggregateError
	if !errors.As(err, &agg) {
		t.Fatalf("expected *AggregateError, got %T: %v", err, err)
	}
	if len(agg.Errors) != 2 {
		t.Fatalf("expected 2 aggregate errors, got %d", len(agg.Errors))
	}
	var pe *ProviderError
	if !errors.As(agg, &pe) {
		t.Fatalf("expected aggregate to unwrap to *ProviderError, got %T", err)
	}
}

func TestFailoverChatStopsOnGenericNonProviderError(t *testing.T) {
	a := &stubProvider{name: "a", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, errors.New("permanent")
	}}
	b := &stubProvider{name: "b", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "should not reach"}, nil
	}}

	r := NewRouter()
	r.Add(a, 10)
	r.Add(b, 5)
	fr := NewFailoverRouter(r)

	_, err := fr.Chat(context.Background(), ChatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.As(err, new(*AggregateError)) {
		t.Fatalf("expected non-aggregate error, got %T: %v", err, err)
	}
}

func TestFailoverStreamChatSwitchesBeforeStreamStarts(t *testing.T) {
	a := &stubProvider{name: "a", stream: func(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
		return nil, &ProviderError{Kind: Network, Message: "stream setup failed", Retryable: true}
	}}
	b := &stubProvider{name: "b", stream: func(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
		ch := make(chan StreamEvent, 1)
		ch <- StreamEvent{Type: StreamEventDone}
		close(ch)
		return ch, nil
	}}

	fr := NewFailoverRouterWithProviders(a, b)
	got, err := fr.StreamChat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil stream")
	}
}

func TestFailoverChatMarksFailedProvider(t *testing.T) {
	a := &stubProvider{name: "a", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return nil, &ProviderError{Kind: ServerError, Message: "down", Retryable: true}
	}}
	b := &stubProvider{name: "b", chat: func(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
		return &ChatResponse{Content: "ok"}, nil
	}}

	r := NewRouter()
	r.Add(a, 10)
	r.Add(b, 5)
	fr := NewFailoverRouter(r)

	if _, err := fr.Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// a 已被标记失败，因此下一次应直接选择 b。
	got, err := fr.Router.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != b {
		t.Fatalf("Next after failover = %q, want b", got.Name())
	}
}
