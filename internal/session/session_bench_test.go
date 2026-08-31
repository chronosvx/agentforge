package session_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
)

// benchEntry 构造一个包含 1000 条消息的会话，模拟超长会话场景。
func benchEntry() session.Entry {
	msgs := make([]client.Message, 0, 1000)
	for i := 0; i < 1000; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, client.Message{
			Role:    role,
			Content: fmt.Sprintf("message %d: 这是用于基准测试的较长内容，包含一些关键字 needle-%d 和 padding %s", i, i, "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"),
		})
	}
	return session.Entry{
		ID:        "bench-session",
		Workspace: "bench-workspace",
		Title:     "Benchmark Session",
		CreatedAt: time.Unix(1700000000, 0),
		UpdatedAt: time.Unix(1700000001, 0),
		Messages:  msgs,
		Context:   map[string]any{"note": "benchmark"},
		Cost:      map[string]any{"tokens": 12345},
	}
}

func BenchmarkSessionSave1000Messages(b *testing.B) {
	dir := b.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	entry := benchEntry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Save(entry); err != nil {
			b.Fatalf("save: %v", err)
		}
	}
}

func BenchmarkSessionLoad1000Messages(b *testing.B) {
	dir := b.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		b.Fatalf("new store: %v", err)
	}
	if err := store.Save(benchEntry()); err != nil {
		b.Fatalf("seed save: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := store.Load("bench-session"); err != nil {
			b.Fatalf("load: %v", err)
		}
	}
}
