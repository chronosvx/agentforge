package search_test

import (
	"fmt"
	"testing"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/search"
	"github.com/chronosvx/agentforge/internal/session"
)

// benchEntry 构造一个包含 1000 条消息的会话，用于搜索基准测试。
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
		ID:       "bench-session",
		Messages: msgs,
	}
}

func BenchmarkSearchIndex1000Messages(b *testing.B) {
	entry := benchEntry()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx := &search.Index{}
		idx.Add(entry)
	}
}

func BenchmarkSearchSearch1000Messages(b *testing.B) {
	entry := benchEntry()
	idx := &search.Index{}
	idx.Add(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("needle-999", 50)
	}
}

func BenchmarkSearchSearch1000MessagesMiss(b *testing.B) {
	entry := benchEntry()
	idx := &search.Index{}
	idx.Add(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = idx.Search("this-query-does-not-exist", 50)
	}
}
