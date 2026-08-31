package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryStoreSaveGetListDelete(t *testing.T) {
	store, err := NewMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	now := time.Now().UTC()
	older := now.Add(-time.Hour)

	entries := []Entry{
		{ID: "e1", Project: "demo", Key: "build", Content: "Run `make build` first", Tags: []string{"go", "build"}, UpdatedAt: now},
		{ID: "e2", Project: "demo", Key: "test", Content: "Use `go test ./...`", Tags: []string{"go", "test"}, UpdatedAt: older},
	}
	for _, entry := range entries {
		if err := store.Save(context.Background(), entry); err != nil {
			t.Fatalf("Save(%q): %v", entry.Key, err)
		}
	}

	got, err := store.Get(context.Background(), "demo", "build")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "e1" || got.Key != "build" || got.Content != "Run `make build` first" {
		t.Fatalf("unexpected Get result: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "go" {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}

	list, err := store.List(context.Background(), "demo")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List length = %d, want 2", len(list))
	}
	// UpdatedAt 倒序：build 应在 test 前面。
	if list[0].Key != "build" || list[1].Key != "test" {
		t.Fatalf("List order = %q, %q; want build, test", list[0].Key, list[1].Key)
	}

	if err := store.Delete(context.Background(), "demo", "build"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(context.Background(), "demo", "build"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Get after Delete error = %v, want os.ErrNotExist", err)
	}
	remaining, err := store.List(context.Background(), "demo")
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Key != "test" {
		t.Fatalf("remaining = %+v, want only test", remaining)
	}
}

func TestMemoryStoreOverwriteSameKey(t *testing.T) {
	store, err := NewMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	first := time.Now().UTC()
	if err := store.Save(context.Background(), Entry{Project: "demo", Key: "cmd", Content: "old", UpdatedAt: first}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	second := first.Add(time.Minute)
	if err := store.Save(context.Background(), Entry{Project: "demo", Key: "cmd", Content: "new", UpdatedAt: second}); err != nil {
		t.Fatalf("Save second: %v", err)
	}

	got, err := store.Get(context.Background(), "demo", "cmd")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "new" {
		t.Fatalf("Content = %q, want new", got.Content)
	}
	list, err := store.List(context.Background(), "demo")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List length = %d, want 1 after overwrite", len(list))
	}
}

func TestMemoryStoreFilePersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	want := Entry{
		ID:        "note-1",
		Project:   "demo",
		Key:       "style",
		Content:   "Prefer table-driven tests",
		Tags:      []string{"go", "testing"},
		UpdatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, "memory", "demo.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read memory file: %v", err)
	}
	if !strings.Contains(string(data), "Prefer table-driven tests") {
		t.Fatalf("memory file does not contain saved content:\n%s", data)
	}

	reopened, err := NewMemoryStore(dir)
	if err != nil {
		t.Fatalf("NewMemoryStore reopened: %v", err)
	}
	got, err := reopened.Get(context.Background(), "demo", "style")
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got.Content != want.Content || got.UpdatedAt.Unix() != want.UpdatedAt.Unix() {
		t.Fatalf("reopened entry = %+v, want %+v", got, want)
	}
}

func TestSearcherSearch(t *testing.T) {
	store, err := NewMemoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	now := time.Now().UTC()
	entries := []Entry{
		{ID: "1", Project: "demo", Key: "build", Content: "Run go build and go vet", Tags: []string{"go", "ci"}, UpdatedAt: now},
		{ID: "2", Project: "demo", Key: "python", Content: "Use ruff for linting", Tags: []string{"python", "lint"}, UpdatedAt: now.Add(-time.Minute)},
		{ID: "3", Project: "other", Key: "build", Content: "Other project build", Tags: nil, UpdatedAt: now.Add(-2 * time.Minute)},
	}
	for _, entry := range entries {
		if err := store.Save(context.Background(), entry); err != nil {
			t.Fatalf("Save(%q): %v", entry.Key, err)
		}
	}

	searcher := NewSearcher(store)

	if got := searcher.Search("demo", "go", 0); len(got) != 1 {
		t.Fatalf("Search(go) length = %d, want 1: %+v", len(got), got)
	}
	if got := searcher.Search("demo", "GOLANG", 0); len(got) != 0 {
		t.Fatalf("Search(GOLANG) length = %d, want 0 (tag is go, not golang)", len(got))
	}
	if got := searcher.Search("demo", "go build", 0); len(got) != 1 || got[0].Key != "build" {
		t.Fatalf("Search(go build) = %+v, want build entry", got)
	}
	if got := searcher.Search("demo", "python", 0); len(got) != 1 || got[0].Key != "python" {
		t.Fatalf("Search(python) = %+v, want python entry", got)
	}
	if got := searcher.Search("demo", "lint", 0); len(got) != 1 || got[0].Key != "python" {
		t.Fatalf("Search(lint via tag) = %+v, want python entry", got)
	}
	if got := searcher.Search("demo", "missing", 0); len(got) != 0 {
		t.Fatalf("Search(missing) = %+v, want empty", got)
	}
	if got := searcher.Search("demo", "go", 0); len(got) > 1 {
		t.Fatalf("project isolation failed: %+v", got)
	}
	if got := searcher.Search("demo", "go", 1); len(got) != 1 {
		t.Fatalf("Search(go, limit=1) length = %d, want 1", len(got))
	}
	if got := searcher.Search("", "", 0); got != nil {
		t.Fatalf("empty search = %+v, want nil", got)
	}
}

func TestSearchEntries(t *testing.T) {
	entries := []Entry{
		{Key: "a", Content: "Alpha beta gamma", Tags: []string{"x"}},
		{Key: "b", Content: "Beta only", Tags: nil},
	}
	if got := SearchEntries(entries, "alpha", 0); len(got) != 1 || got[0].Key != "a" {
		t.Fatalf("SearchEntries alpha = %+v", got)
	}
	if got := SearchEntries(entries, "alpha beta", 0); len(got) != 1 || got[0].Key != "a" {
		t.Fatalf("SearchEntries alpha beta = %+v", got)
	}
	if got := SearchEntries(entries, "beta", 1); len(got) != 1 || got[0].Key != "a" {
		t.Fatalf("SearchEntries beta limit 1 = %+v", got)
	}
}
