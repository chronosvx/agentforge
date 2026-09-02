package contextx

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestAddAndCurrent(t *testing.T) {
	tl := New()

	if got := tl.Current(); got != nil {
		t.Fatalf("Current() on empty timeline = %v, want nil", got)
	}

	first := tl.Add("first", []byte("one"))
	if first.ID == "" {
		t.Fatal("Add() returned snapshot with empty ID")
	}
	if first.Label != "first" {
		t.Fatalf("Add() label = %q, want %q", first.Label, "first")
	}
	if !bytes.Equal(first.Data, []byte("one")) {
		t.Fatalf("Add() data = %q, want %q", first.Data, "one")
	}
	if first.CreatedAt.IsZero() {
		t.Fatal("Add() snapshot has zero CreatedAt")
	}

	second := tl.Add("second", []byte("two"))
	if second.ID == first.ID {
		t.Fatalf("Add() returned duplicate ID %q", second.ID)
	}

	cur := tl.Current()
	if cur == nil {
		t.Fatal("Current() = nil, want non-nil after Add")
	}
	if cur.ID != second.ID {
		t.Fatalf("Current() ID = %q, want %q", cur.ID, second.ID)
	}
	if cur.Label != "second" {
		t.Fatalf("Current() label = %q, want %q", cur.Label, "second")
	}
	if !bytes.Equal(cur.Data, []byte("two")) {
		t.Fatalf("Current() data = %q, want %q", cur.Data, "two")
	}

	got := tl.List()
	if len(got) != 2 {
		t.Fatalf("List() length = %d, want 2", len(got))
	}
	if got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("List() order = [%q, %q], want [%q, %q]", got[0].ID, got[1].ID, first.ID, second.ID)
	}
}

func TestRewindToOlder(t *testing.T) {
	tl := New()
	a := tl.Add("a", []byte("a"))
	b := tl.Add("b", []byte("b"))
	_ = tl.Add("c", []byte("c"))

	got, err := tl.Rewind(a.ID)
	if err != nil {
		t.Fatalf("Rewind(%q) error = %v, want nil", a.ID, err)
	}
	if got.ID != a.ID {
		t.Fatalf("Rewind() ID = %q, want %q", got.ID, a.ID)
	}
	if got.Label != "a" {
		t.Fatalf("Rewind() label = %q, want %q", got.Label, "a")
	}

	cur := tl.Current()
	if cur == nil || cur.ID != a.ID {
		t.Fatalf("Current() after rewind = %+v, want snapshot %q", cur, a.ID)
	}

	got, err = tl.Rewind(b.ID)
	if err != nil {
		t.Fatalf("Rewind(%q) error = %v, want nil", b.ID, err)
	}
	if got.ID != b.ID {
		t.Fatalf("Rewind() ID = %q, want %q", got.ID, b.ID)
	}
	if cur := tl.Current(); cur == nil || cur.ID != b.ID {
		t.Fatalf("Current() after rewind = %+v, want snapshot %q", cur, b.ID)
	}

	if list := tl.List(); len(list) != 3 {
		t.Fatalf("List() length after rewinds = %d, want 3", len(list))
	}
}

func TestForkTruncatesFuture(t *testing.T) {
	tl := New()
	a := tl.Add("a", []byte("a"))
	b := tl.Add("b", []byte("b"))
	c := tl.Add("c", []byte("c"))

	if _, err := tl.Rewind(b.ID); err != nil {
		t.Fatalf("Rewind(%q) error = %v, want nil", b.ID, err)
	}

	d := tl.Fork("d", []byte("d"))
	if d.ID == a.ID || d.ID == b.ID || d.ID == c.ID {
		t.Fatalf("Fork() returned existing ID %q", d.ID)
	}

	list := tl.List()
	if len(list) != 3 {
		t.Fatalf("List() length after fork = %d, want 3", len(list))
	}
	wantIDs := []string{a.ID, b.ID, d.ID}
	for i, want := range wantIDs {
		if list[i].ID != want {
			t.Fatalf("List()[%d] ID = %q, want %q", i, list[i].ID, want)
		}
	}

	cur := tl.Current()
	if cur == nil || cur.ID != d.ID {
		t.Fatalf("Current() after fork = %+v, want snapshot %q", cur, d.ID)
	}
}

func TestForkOnEmptyTimeline(t *testing.T) {
	tl := New()
	s := tl.Fork("first", []byte("first"))
	if len(tl.List()) != 1 {
		t.Fatalf("List() length after fork on empty timeline = %d, want 1", len(tl.List()))
	}
	if cur := tl.Current(); cur == nil || cur.ID != s.ID {
		t.Fatalf("Current() after fork on empty timeline = %+v, want %q", cur, s.ID)
	}
}

func TestRewindMissingReturnsError(t *testing.T) {
	tl := New()
	a := tl.Add("a", []byte("a"))

	got, err := tl.Rewind("cp-does-not-exist")
	if err == nil {
		t.Fatal("Rewind(missing) error = nil, want ErrNotFound")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Rewind(missing) error = %v, want ErrNotFound", err)
	}
	if got != nil {
		t.Fatalf("Rewind(missing) snapshot = %+v, want nil", got)
	}

	if cur := tl.Current(); cur == nil || cur.ID != a.ID {
		t.Fatalf("Current() after failed rewind = %+v, want %q", cur, a.ID)
	}
}

func TestListReturnsCopies(t *testing.T) {
	tl := New()
	data := []byte("original")
	s := tl.Add("copy", data)

	data[0] = 'X'
	if !bytes.Equal(tl.List()[0].Data, []byte("original")) {
		t.Fatal("Add() stored a reference to caller data; timeline data was mutated")
	}
	if !bytes.Equal(s.Data, []byte("original")) {
		t.Fatal("Add() returned a snapshot whose data aliases caller data")
	}

	list := tl.List()
	list[0].Label = "mutated"
	list[0].Data[0] = 'Y'
	if cur := tl.Current(); cur.Label != "copy" || !bytes.Equal(cur.Data, []byte("original")) {
		t.Fatalf("List() returned references to internal snapshots: current = %+v", cur)
	}
}

func TestConcurrentSafetySmoke(t *testing.T) {
	tl := New()
	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				s := tl.Add(fmt.Sprintf("g%d-%d", id, i), []byte(fmt.Sprintf("%d-%d", id, i)))
				if s.ID == "" {
					t.Errorf("Add() returned empty ID in goroutine %d", id)
					return
				}
				_ = tl.Current()
				_ = tl.List()
				if i%10 == 0 {
					_, _ = tl.Rewind(s.ID)
					_ = tl.Fork(fmt.Sprintf("fork-%d-%d", id, i), []byte("fork"))
				}
			}
		}(g)
	}
	wg.Wait()

	list := tl.List()
	if len(list) == 0 {
		t.Fatal("List() empty after concurrent adds")
	}
	for i := range list {
		if list[i].ID == "" {
			t.Fatalf("List()[%d] has empty ID", i)
		}
	}
	if cur := tl.Current(); cur == nil || cur.ID == "" {
		t.Fatalf("Current() after concurrent operations = %+v, want non-empty snapshot", cur)
	}
}
