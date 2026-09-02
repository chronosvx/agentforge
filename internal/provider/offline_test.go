package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestQueueEnqueuePendingRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q := NewQueue(path)

	if err := q.Enqueue("session-a", "hello"); err != nil {
		t.Fatalf("Enqueue(hello) error: %v", err)
	}
	if err := q.Enqueue("session-a", "world"); err != nil {
		t.Fatalf("Enqueue(world) error: %v", err)
	}
	if err := q.Enqueue("session-b", "third"); err != nil {
		t.Fatalf("Enqueue(third) error: %v", err)
	}

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("len(Pending()) = %d, want 3", len(items))
	}
	if items[0].SessionID != "session-a" || items[0].Message != "hello" {
		t.Fatalf("items[0] = %+v, want session-a/hello", items[0])
	}
	if items[0].ID == "" {
		t.Fatal("items[0].ID = empty, want generated id")
	}
	if items[0].Replay {
		t.Fatalf("items[0].Replay = true, want false by default")
	}
	if items[0].CreatedAt.IsZero() {
		t.Fatal("items[0].CreatedAt is zero, want set")
	}
	if items[1].Message != "world" || items[2].SessionID != "session-b" {
		t.Fatalf("unexpected order: %+v", items)
	}

	if err := q.Remove(items[1].ID); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	items, err = q.Pending()
	if err != nil {
		t.Fatalf("Pending() after remove error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(Pending()) after remove = %d, want 2", len(items))
	}
	if items[0].Message != "hello" || items[1].Message != "third" {
		t.Fatalf("items after remove = %+v, want hello and third", items)
	}

	// 删除不存在的 ID 应是幂等操作。
	if err := q.Remove("not-exist"); err != nil {
		t.Fatalf("Remove(not-exist) error: %v", err)
	}
}

func TestQueuePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q1 := NewQueue(path)
	if err := q1.Enqueue("session-x", "persisted"); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}

	q2 := NewQueue(path)
	items, err := q2.Pending()
	if err != nil {
		t.Fatalf("Pending() on new instance error: %v", err)
	}
	if len(items) != 1 || items[0].Message != "persisted" {
		t.Fatalf("items from disk = %+v, want one persisted message", items)
	}
}

func TestQueueFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	base := t.TempDir()
	dir := filepath.Join(base, "private")
	path := filepath.Join(dir, "queue.json")
	q := NewQueue(path)

	if err := q.Enqueue("session-secret", "secret message"); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(queue file) error: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("queue file permission = %o, want 600", perm)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(queue dir) error: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("queue dir permission = %o, want 700", perm)
	}
}

func TestQueueEnqueueReplayPersistsReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	q := NewQueue(path)
	if err := q.EnqueueReplay("session-r", "replay me"); err != nil {
		t.Fatalf("EnqueueReplay error: %v", err)
	}

	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(Pending()) = %d, want 1", len(items))
	}
	if !items[0].Replay {
		t.Fatalf("items[0].Replay = false, want true")
	}
	if items[0].Message != "replay me" {
		t.Fatalf("items[0].Message = %q, want %q", items[0].Message, "replay me")
	}
}

func TestQueueEmptyPending(t *testing.T) {
	q := NewQueue(filepath.Join(t.TempDir(), "queue.json"))
	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(Pending()) = %d, want 0", len(items))
	}
}

func TestQueueReadsArrayFileFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.json")
	content := `[{"id":"a1","session_id":"s1","message":"hello","created_at":"2025-01-01T00:00:00Z"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	q := NewQueue(path)
	items, err := q.Pending()
	if err != nil {
		t.Fatalf("Pending() error: %v", err)
	}
	if len(items) != 1 || items[0].ID != "a1" || items[0].Message != "hello" {
		t.Fatalf("items = %+v, want one array-format item", items)
	}
}
