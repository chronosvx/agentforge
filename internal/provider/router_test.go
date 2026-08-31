package provider

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/cost"
)

type stubProvider struct {
	name   string
	chat   func(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	stream func(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
}

func (s *stubProvider) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

func (s *stubProvider) Model() string { return "" }

func (s *stubProvider) CostEstimator() *cost.Estimator { return cost.NewEstimator() }

func (s *stubProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if s != nil && s.chat != nil {
		return s.chat(ctx, req)
	}
	return &ChatResponse{}, nil
}

func (s *stubProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	if s != nil && s.stream != nil {
		return s.stream(ctx, req)
	}
	ch := make(chan StreamEvent)
	close(ch)
	return ch, nil
}

func TestRouterPrioritySelection(t *testing.T) {
	low := &stubProvider{name: "low"}
	high := &stubProvider{name: "high"}
	mid := &stubProvider{name: "mid"}

	r := NewRouter()
	r.Add(low, 1)
	r.Add(mid, 5)
	r.Add(high, 10)

	got, err := r.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got != high {
		t.Fatalf("expected high priority provider, got %q", got.Name())
	}
}

func TestRouterRoundRobinSamePriority(t *testing.T) {
	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}
	c := &stubProvider{name: "c"}

	r := NewRouter()
	r.Add(a, 1)
	r.Add(b, 1)
	r.Add(c, 1)

	want := []Provider{a, b, c, a}
	for i, w := range want {
		got, err := r.Next(context.Background())
		if err != nil {
			t.Fatalf("Next %d: %v", i, err)
		}
		if got != w {
			t.Fatalf("Next %d = %q, want %q", i, got.Name(), w.Name())
		}
	}
}

func TestRouterMarkFailedAndReset(t *testing.T) {
	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}

	r := NewRouter()
	r.Add(a, 10)
	r.Add(b, 5)

	if got, _ := r.Next(context.Background()); got != a {
		t.Fatalf("first Next = %q, want a", got.Name())
	}

	r.MarkFailed(a)
	got, err := r.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after fail: %v", err)
	}
	if got != b {
		t.Fatalf("Next after fail = %q, want b", got.Name())
	}

	r.Reset(a)
	got, err = r.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after reset: %v", err)
	}
	if got != a {
		t.Fatalf("Next after reset = %q, want a", got.Name())
	}
}

func TestRouterAllFailed(t *testing.T) {
	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}

	r := NewRouter()
	r.Add(a, 1)
	r.Add(b, 1)

	r.MarkFailed(a)
	r.MarkFailed(b)

	if _, err := r.Next(context.Background()); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("expected ErrNoProvider, got %v", err)
	}
}

func TestRouterAutoRecovery(t *testing.T) {
	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}

	r := NewRouter()
	r.Add(a, 10)
	r.Add(b, 5)
	r.SetRetryAfter(time.Hour)

	base := time.Now()
	r.now = func() time.Time { return base }
	r.MarkFailed(a)

	if got, _ := r.Next(context.Background()); got != b {
		t.Fatalf("while failed, Next = %q, want b", got.Name())
	}

	r.now = func() time.Time { return base.Add(time.Hour + time.Second) }
	got, err := r.Next(context.Background())
	if err != nil {
		t.Fatalf("Next after cooldown: %v", err)
	}
	if got != a {
		t.Fatalf("Next after cooldown = %q, want a", got.Name())
	}
}

func TestRouterConcurrentNext(t *testing.T) {
	a := &stubProvider{name: "a"}
	b := &stubProvider{name: "b"}

	r := NewRouter()
	r.Add(a, 1)
	r.Add(b, 1)

	const n = 100
	var wg sync.WaitGroup
	got := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := r.Next(context.Background())
			if err != nil {
				t.Errorf("Next: %v", err)
				return
			}
			got[i] = p.Name()
		}(i)
	}
	wg.Wait()

	counts := map[string]int{}
	for _, name := range got {
		counts[name]++
	}
	if counts["a"] == 0 || counts["b"] == 0 {
		t.Fatalf("expected both providers used, got %#v", counts)
	}
}
