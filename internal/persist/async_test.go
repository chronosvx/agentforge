package persist_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/persist"
)

func TestWriterSerialOrder(t *testing.T) {
	w := persist.NewWriter(4)
	defer w.Close()

	var mu sync.Mutex
	var got []int

	for i := 0; i < 10; i++ {
		i := i
		if err := w.Submit(func() error {
			time.Sleep(time.Millisecond)
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 10 {
		t.Fatalf("expected 10 executed tasks, got %d: %v", len(got), got)
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("expected serial order, got %v at index %d", v, i)
		}
	}
}

func TestWriterErrorCollection(t *testing.T) {
	w := persist.NewWriter(2)
	errCh := w.Errors()

	var mu sync.Mutex
	var got []error
	done := make(chan struct{})
	go func() {
		defer close(done)
		for err := range errCh {
			mu.Lock()
			got = append(got, err)
			mu.Unlock()
		}
	}()

	boom := errors.New("boom")
	if err := w.Submit(func() error { return nil }); err != nil {
		t.Fatalf("submit ok: %v", err)
	}
	if err := w.Submit(func() error { return boom }); err != nil {
		t.Fatalf("submit boom: %v", err)
	}
	if err := w.Submit(func() error { return nil }); err != nil {
		t.Fatalf("submit last: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("expected 1 collected error, got %d: %v", len(got), got)
	}
	if !errors.Is(got[0], boom) {
		t.Fatalf("unexpected error: %v", got[0])
	}
}

func TestWriterCloseFlushesPending(t *testing.T) {
	w := persist.NewWriter(1)

	var mu sync.Mutex
	var got []int
	for i := 0; i < 5; i++ {
		i := i
		if err := w.Submit(func() error {
			time.Sleep(time.Millisecond)
			mu.Lock()
			got = append(got, i)
			mu.Unlock()
			return nil
		}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	if len(got) != 5 {
		mu.Unlock()
		t.Fatalf("expected Close to flush all pending tasks, got %d: %v", len(got), got)
	}
	mu.Unlock()

	if err := w.Submit(func() error { return nil }); err == nil {
		t.Fatal("expected Submit after Close to return an error")
	}
}

func TestWriterFlushRespectsContext(t *testing.T) {
	w := persist.NewWriter(1)
	defer w.Close()

	block := make(chan struct{})
	defer close(block)
	if err := w.Submit(func() error {
		<-block
		return nil
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := w.Flush(ctx); err == nil {
		t.Fatal("expected Flush to return context error when worker is blocked")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
