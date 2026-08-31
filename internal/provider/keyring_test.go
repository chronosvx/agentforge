package provider

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestKeyRingRoundRobin(t *testing.T) {
	r := NewKeyRing("a", "b", "c")

	want := []string{"a", "b", "c", "a", "b"}
	for i, wantKey := range want {
		got, err := r.Next()
		if err != nil {
			t.Fatalf("Next() #%d error: %v", i, err)
		}
		if got != wantKey {
			t.Fatalf("Next() #%d = %q, want %q", i, got, wantKey)
		}
	}
}

func TestKeyRingSkipsFailed(t *testing.T) {
	r := NewKeyRing("a", "b", "c")
	r.MarkFailed("b")

	want := []string{"a", "c", "a", "c"}
	for i, wantKey := range want {
		got, err := r.Next()
		if err != nil {
			t.Fatalf("Next() #%d error: %v", i, err)
		}
		if got != wantKey {
			t.Fatalf("Next() #%d = %q, want %q", i, got, wantKey)
		}
	}
}

func TestKeyRingReset(t *testing.T) {
	r := NewKeyRing("a", "b", "c")
	r.MarkFailed("a")
	r.MarkFailed("b")
	r.MarkFailed("c")

	if _, err := r.Next(); !errors.Is(err, ErrAllKeysFailed) {
		t.Fatalf("Next() error = %v, want ErrAllKeysFailed", err)
	}

	r.Reset("b")
	got, err := r.Next()
	if err != nil {
		t.Fatalf("Next() after reset error: %v", err)
	}
	if got != "b" {
		t.Fatalf("Next() after reset = %q, want %q", got, "b")
	}
}

func TestKeyRingEmpty(t *testing.T) {
	r := &KeyRing{}
	if _, err := r.Next(); !errors.Is(err, ErrNoKeys) {
		t.Fatalf("Next() error = %v, want ErrNoKeys", err)
	}
}

func TestKeyRingAddDuplicateAndEmpty(t *testing.T) {
	r := NewKeyRing("a")
	r.Add("a")
	r.Add("")
	r.Add("b")

	want := []string{"a", "b", "a", "b"}
	for i, wantKey := range want {
		got, err := r.Next()
		if err != nil {
			t.Fatalf("Next() #%d error: %v", i, err)
		}
		if got != wantKey {
			t.Fatalf("Next() #%d = %q, want %q", i, got, wantKey)
		}
	}
}

func TestKeyRingMarkFailedUnknownKeyNoop(t *testing.T) {
	r := NewKeyRing("a")
	r.MarkFailed("unknown")
	r.Reset("unknown")

	got, err := r.Next()
	if err != nil {
		t.Fatalf("Next() error: %v", err)
	}
	if got != "a" {
		t.Fatalf("Next() = %q, want %q", got, "a")
	}
}

func TestKeyRingConcurrentUse(t *testing.T) {
	r := NewKeyRing("a", "b", "c")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = r.Next()
				r.Add(fmt.Sprintf("k%d-%d", n, j))
				if j%2 == 0 {
					r.MarkFailed("b")
				} else {
					r.Reset("b")
				}
			}
		}(i)
	}
	wg.Wait()

	if _, err := r.Next(); err != nil {
		t.Fatalf("Next() after concurrent use error: %v", err)
	}
}
