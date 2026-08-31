package session_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestAsyncStoreSaveAfterFlushCanRead(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	async := session.NewAsyncStore(store, 4)
	defer async.Close()

	entry := session.Entry{
		ID:        "s1",
		Title:     "async title",
		UpdatedAt: time.Unix(100, 0),
		Messages: []client.Message{
			{Role: "user", Content: "hello"},
		},
	}
	if err := async.Save(entry); err != nil {
		t.Fatalf("async save: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := async.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got, err := async.Load("s1")
	if err != nil {
		t.Fatalf("load after flush: %v", err)
	}
	if got.Title != entry.Title {
		t.Fatalf("title = %q, want %q", got.Title, entry.Title)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Fatalf("unexpected messages: %#v", got.Messages)
	}
}

func TestAsyncStoreDeleteFlushesPendingSave(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	async := session.NewAsyncStore(store, 2)
	defer async.Close()

	if err := async.Save(session.Entry{ID: "s1", Title: "to be deleted"}); err != nil {
		t.Fatalf("async save: %v", err)
	}
	if err := async.Delete("s1"); err != nil {
		t.Fatalf("async delete: %v", err)
	}

	if _, err := async.Load("s1"); !os.IsNotExist(err) {
		t.Fatalf("expected session to be deleted, got err=%v", err)
	}
}

func TestAsyncStoreSaveInvalidID(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	async := session.NewAsyncStore(store, 2)
	defer async.Close()

	if err := async.Save(session.Entry{ID: "../escape"}); err == nil {
		t.Fatal("expected invalid ID to return an immediate error")
	}
}

func TestAsyncStoreCollectsWriteErrors(t *testing.T) {
	// 构造一个无法创建的目录，使后台 Save 必然失败。
	base := t.TempDir()
	store, err := session.NewStore(base)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	// 删除 sessions 目录并放一个同名普通文件，让后续 MkdirAll 失败。
	sessionsDir := base + "/sessions"
	if err := os.RemoveAll(sessionsDir); err != nil {
		t.Fatalf("remove sessions dir: %v", err)
	}
	if err := os.WriteFile(sessionsDir, []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("create blocking file: %v", err)
	}
	defer os.RemoveAll(sessionsDir)

	async := session.NewAsyncStore(store, 2)
	errCh := async.Errors()
	done := make(chan struct{})
	var collected []error
	go func() {
		defer close(done)
		for err := range errCh {
			collected = append(collected, err)
		}
	}()

	if err := async.Save(session.Entry{ID: "s1", Title: "boom"}); err != nil {
		t.Fatalf("async save: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := async.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if err := async.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	<-done

	if len(collected) == 0 {
		t.Fatal("expected at least one async write error")
	}
	if !errors.Is(collected[0], os.ErrNotExist) && !errors.Is(collected[0], os.ErrExist) {
		// 不同平台可能返回不同错误，这里只确认确实收到了错误。
		t.Logf("collected async error: %v", collected[0])
	}
}
