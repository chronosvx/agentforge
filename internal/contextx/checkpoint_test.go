package contextx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckpointStoreMemory(t *testing.T) {
	store := NewCheckpointStore()

	cp := Checkpoint{
		ID:         "cp-1",
		Messages:   []Message{{Role: "user", Content: "hello"}},
		TokenCount: 7,
		Note:       "before compaction",
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	got, err := store.Load("cp-1")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got.ID != cp.ID || got.Note != cp.Note || len(got.Messages) != 1 {
		t.Fatalf("Load got %#v, want %#v", got, cp)
	}
	if got.Messages[0].Content != "hello" {
		t.Fatalf("Load message = %#v, want hello", got.Messages[0])
	}

	if err := store.Delete("cp-1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, err := store.Load("cp-1"); !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("Load after delete err = %v, want ErrCheckpointNotFound", err)
	}
}

func TestCheckpointStoreFilePersistsAcrossStores(t *testing.T) {
	dir := t.TempDir()

	store, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore error: %v", err)
	}

	cp := Checkpoint{
		ID:         "cp-file",
		CreatedAt:  mustParseTime(t, "2025-01-02T03:04:05Z"),
		Messages:   []Message{{Role: "user", Content: "persist me"}},
		TokenCount: 9,
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// 模拟重启：使用同一目录新建 store，内存为空，应从文件恢复。
	restarted, err := NewFileCheckpointStore(dir)
	if err != nil {
		t.Fatalf("NewFileCheckpointStore(restart) error: %v", err)
	}
	got, err := restarted.Load("cp-file")
	if err != nil {
		t.Fatalf("Load after restart error: %v", err)
	}
	if got.ID != cp.ID || got.TokenCount != cp.TokenCount {
		t.Fatalf("Load after restart = %#v, want %#v", got, cp)
	}
	if got.Messages[0].Content != "persist me" {
		t.Fatalf("Load after restart message = %#v, want persist me", got.Messages[0])
	}

	list, err := restarted.List()
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(list) != 1 || list[0].ID != "cp-file" {
		t.Fatalf("List = %#v, want one cp-file checkpoint", list)
	}

	if err := restarted.Delete("cp-file"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if _, err := restarted.Load("cp-file"); !errors.Is(err, ErrCheckpointNotFound) {
		t.Fatalf("Load after delete err = %v, want ErrCheckpointNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cp-file.json")); !os.IsNotExist(err) {
		t.Fatalf("checkpoint file still exists after delete: %v", err)
	}
}

func TestCheckpointStoreRejectsInvalidID(t *testing.T) {
	store := NewCheckpointStore()
	if err := store.Save(Checkpoint{ID: "../evil"}); err == nil {
		t.Fatal("Save with invalid ID succeeded, want error")
	}
	if _, err := store.Load("a/b"); err == nil {
		t.Fatal("Load with invalid ID succeeded, want error")
	}
	if err := store.Delete(""); err == nil {
		t.Fatal("Delete with empty ID succeeded, want error")
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}
