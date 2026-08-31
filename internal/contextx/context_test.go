package contextx

import (
	"strings"
	"testing"
)

func TestWindowAddAndTokenCount(t *testing.T) {
	w := NewWindow()
	if w.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", w.Len())
	}
	if got := w.TokenCount(); got != 0 {
		t.Fatalf("TokenCount() = %d, want 0", got)
	}

	w.Add(Message{Role: "user", Content: "hello"})
	if w.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", w.Len())
	}
	want := int64(messageOverheadTokens + DefaultTokenEstimator("user") + DefaultTokenEstimator("hello"))
	if got := w.TokenCount(); got != want {
		t.Fatalf("TokenCount() = %d, want %d", got, want)
	}
}

func TestWindowMessagesReturnsCopy(t *testing.T) {
	w := NewWindow()
	w.Add(Message{Role: "user", Content: "hello"})

	msgs := w.Messages()
	msgs[0].Content = "mutated"

	got := w.Messages()
	if got[0].Content != "hello" {
		t.Fatalf("Messages() copy was mutated: %#v", got[0])
	}
}

func TestWindowTrimToLimit(t *testing.T) {
	w := NewWindow()
	for i := 0; i < 5; i++ {
		w.Add(Message{Role: "user", Content: "hello"})
	}

	// 每条消息固定为 messageOverheadTokens + user(1) + hello(2) = 7 tokens，
	// 5 条共 35 tokens；limit=15 时应裁剪 3 条，剩余 2 条共 14 tokens。
	removed := w.TrimToLimit(15)
	if len(removed) != 3 {
		t.Fatalf("TrimToLimit removed %d messages, want 3", len(removed))
	}
	if w.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", w.Len())
	}
	if got := w.TokenCount(); got > 15 {
		t.Fatalf("TokenCount() = %d, want <= 15", got)
	}
	if removed[0].Content != "hello" || removed[2].Content != "hello" {
		t.Fatalf("removed messages not in order: %#v", removed)
	}

	// 即使最新一条超过 limit，也应至少保留一条。
	w2 := NewWindow()
	w2.Add(Message{Role: "user", Content: "hello world this is a long message"})
	w2.Add(Message{Role: "assistant", Content: "hello world this is another long message"})
	_ = w2.TrimToLimit(1)
	if w2.Len() != 1 {
		t.Fatalf("TrimToLimit(1) Len() = %d, want 1", w2.Len())
	}
}

func TestWindowCheckpointAndRewind(t *testing.T) {
	w := NewWindow()
	w.Add(Message{Role: "user", Content: "hello"})
	w.Add(Message{Role: "assistant", Content: "hi there"})

	cp := w.Checkpoint("cp-before")
	if cp.ID != "cp-before" {
		t.Fatalf("Checkpoint().ID = %q, want cp-before", cp.ID)
	}
	if len(cp.Messages) != 2 {
		t.Fatalf("Checkpoint messages len = %d, want 2", len(cp.Messages))
	}
	if cp.TokenCount != w.TokenCount() {
		t.Fatalf("Checkpoint TokenCount = %d, want %d", cp.TokenCount, w.TokenCount())
	}

	// 继续追加消息后回滚。
	w.Add(Message{Role: "user", Content: "extra"})
	if w.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", w.Len())
	}
	w.Rewind(cp)
	if w.Len() != 2 {
		t.Fatalf("Rewind Len() = %d, want 2", w.Len())
	}
	msgs := w.Messages()
	if msgs[0].Content != "hello" || msgs[1].Content != "hi there" {
		t.Fatalf("Rewind messages = %#v, want original two messages", msgs)
	}
}

func TestSummarizeMessagesJoinsAndTruncates(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}

	summary := SummarizeMessages(msgs, 0)
	if !strings.Contains(summary.Text, "user: hello") {
		t.Fatalf("summary text missing user message: %q", summary.Text)
	}
	if !strings.Contains(summary.Text, "assistant: world") {
		t.Fatalf("summary text missing assistant message: %q", summary.Text)
	}
	if summary.TokenCount <= 0 {
		t.Fatalf("summary TokenCount = %d, want > 0", summary.TokenCount)
	}

	truncated := SummarizeMessages(msgs, 1)
	if !strings.Contains(truncated.Text, "…[truncated]") {
		t.Fatalf("truncated summary missing marker: %q", truncated.Text)
	}
}

func TestWindowCompactUsesSummarizer(t *testing.T) {
	w := NewWindow()
	// 5 条消息，每条 7 tokens，共 35。
	for i := 0; i < 5; i++ {
		w.Add(Message{Role: "user", Content: "hello"})
	}

	removed, err := w.Compact(25, SimpleSummarizer{MaxTokens: 4})
	if err != nil {
		t.Fatalf("Compact error: %v", err)
	}
	if len(removed) != 4 {
		t.Fatalf("Compact removed %d messages, want 4", len(removed))
	}
	if w.Len() != 2 {
		t.Fatalf("Compact Len() = %d, want 2 (summary + newest)", w.Len())
	}
	msgs := w.Messages()
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "user: hello") {
		t.Fatalf("first message is not summary: %#v", msgs[0])
	}
	if msgs[1].Content != "hello" {
		t.Fatalf("last message should remain newest: %#v", msgs[1])
	}
	if w.TokenCount() > 25 {
		t.Fatalf("TokenCount() = %d, want <= 25", w.TokenCount())
	}
}
