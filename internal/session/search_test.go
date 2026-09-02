package session_test

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestAddAllAndSearchHit(t *testing.T) {
	idx := &session.Index{}
	idx.AddAll([]session.Entry{
		{
			ID: "s1",
			Messages: []client.Message{
				{Role: "user", Content: "Hello world"},
			},
		},
		{
			ID: "s2",
			Messages: []client.Message{
				{Role: "assistant", Content: "The answer is Alpha beta."},
			},
		},
	})

	results := idx.Search("alpha", 0)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d: %#v", len(results), results)
	}
	got := results[0]
	if got.SessionID != "s2" {
		t.Errorf("unexpected session id: %q", got.SessionID)
	}
	if got.MessageIndex != 0 {
		t.Errorf("unexpected message index: %d", got.MessageIndex)
	}
	if got.Role != "assistant" {
		t.Errorf("unexpected role: %q", got.Role)
	}
	if got.Content != "The answer is Alpha beta." {
		t.Errorf("unexpected content: %q", got.Content)
	}
	if !strings.Contains(got.Snippet, "Alpha") {
		t.Errorf("snippet should contain the matched text, got %q", got.Snippet)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	idx := &session.Index{}
	idx.AddAll([]session.Entry{
		{ID: "s1", Messages: []client.Message{{Role: "user", Content: "anything"}}},
	})

	if got := idx.Search("", 10); got != nil {
		t.Fatalf("expected nil for empty query, got %#v", got)
	}
	if got := idx.Search("   ", 10); got != nil {
		t.Fatalf("expected nil for whitespace query, got %#v", got)
	}
}

func TestSearchLimit(t *testing.T) {
	idx := &session.Index{}
	idx.AddAll([]session.Entry{
		{
			ID: "s1",
			Messages: []client.Message{
				{Role: "user", Content: "first alpha"},
				{Role: "user", Content: "second alpha"},
			},
		},
		{
			ID: "s2",
			Messages: []client.Message{
				{Role: "assistant", Content: "third alpha"},
			},
		},
	})

	if got := idx.Search("alpha", 1); len(got) != 1 {
		t.Fatalf("expected 1 result with limit 1, got %d: %#v", len(got), got)
	} else if got[0].SessionID != "s1" || got[0].MessageIndex != 0 {
		t.Errorf("unexpected first limited result: %#v", got[0])
	}

	limited := idx.Search("alpha", 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 results with limit 2, got %d: %#v", len(limited), limited)
	}
	if limited[0].SessionID != "s1" || limited[0].MessageIndex != 0 {
		t.Errorf("unexpected first result: %#v", limited[0])
	}
	if limited[1].SessionID != "s1" || limited[1].MessageIndex != 1 {
		t.Errorf("unexpected second result: %#v", limited[1])
	}

	if got := idx.Search("alpha", 0); len(got) != 3 {
		t.Fatalf("expected all results when limit is 0, got %d: %#v", len(got), got)
	}
}

func TestSnippetTruncation(t *testing.T) {
	idx := &session.Index{}
	content := strings.Repeat("a", 100) + " needle " + strings.Repeat("b", 100)
	idx.Add(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: content}},
	})

	results := idx.Search("needle", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	snippet := results[0].Snippet
	if len([]rune(snippet)) >= len([]rune(content)) {
		t.Fatalf("expected snippet to be truncated, got length %d (content %d): %q", len([]rune(snippet)), len([]rune(content)), snippet)
	}
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") {
		t.Errorf("expected ellipses on both sides of truncated snippet, got %q", snippet)
	}
	if !strings.Contains(snippet, "needle") {
		t.Errorf("expected snippet to contain the match, got %q", snippet)
	}
}

func TestAddReplacesSessionMessages(t *testing.T) {
	idx := &session.Index{}
	idx.Add(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: "old content"}},
	})
	idx.Add(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "assistant", Content: "new content"}},
	})

	if got := idx.Search("old", 0); len(got) != 0 {
		t.Fatalf("expected old content to be replaced, got %#v", got)
	}
	if got := idx.Search("new", 0); len(got) != 1 {
		t.Fatalf("expected new content to be searchable, got %#v", got)
	}
}

// TestAddAsyncIncrementalUpdate 验证 AddAsync 与 Add 等价，且不会重建整个索引。
func TestAddAsyncIncrementalUpdate(t *testing.T) {
	idx := &session.Index{}
	idx.AddAsync(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: "first"}},
	})
	idx.AddAsync(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: "second"}},
	})

	if got := idx.Search("first", 0); len(got) != 0 {
		t.Fatalf("expected old content to be replaced, got %#v", got)
	}
	if got := idx.Search("second", 0); len(got) != 1 {
		t.Fatalf("expected new content to be searchable, got %#v", got)
	}
}

// FuzzSearchNeverPanics 确保搜索逻辑对任意输入都不会 panic 或死循环。
func FuzzSearchNeverPanics(f *testing.F) {
	seeds := []string{
		"",
		"alpha",
		"needle",
		"你好，世界",
		"a\x00b",
		strings.Repeat("x", 1000),
	}
	for _, seed := range seeds {
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, content string, query string) {
		idx := &session.Index{}
		idx.Add(session.Entry{
			ID:       "s1",
			Messages: []client.Message{{Role: "user", Content: content}},
		})
		_ = idx.Search(query, 0)
		_ = idx.Search(query, 1)
	})
}

func BenchmarkSearchIndex1000Messages(b *testing.B) {
	entry := benchEntry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := &session.Index{}
		idx.Add(entry)
	}
}

func BenchmarkSearchSearch1000Messages(b *testing.B) {
	entry := benchEntry()
	idx := &session.Index{}
	idx.Add(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("needle-999", 50)
	}
}

func BenchmarkSearchSearch1000MessagesMiss(b *testing.B) {
	entry := benchEntry()
	idx := &session.Index{}
	idx.Add(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("this-query-does-not-exist", 50)
	}
}
