package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/contextx"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestCompactCommandSummarizesOldMessagesAndKeepsLast(t *testing.T) {
	old := []client.Message{
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer"},
	}
	newest := client.Message{Role: "user", Content: "second question"}
	m := Model{
		messages: append(append([]client.Message(nil), old...), newest),
	}

	m.handleCommand("/compact")

	if len(m.messages) != 2 {
		t.Fatalf("expected 2 messages after compact, got %d: %#v", len(m.messages), m.messages)
	}
	if m.messages[0].Role != "system" {
		t.Fatalf("expected first message role system, got %q", m.messages[0].Role)
	}
	if m.messages[1] != newest {
		t.Fatalf("expected last message to be preserved, got %#v", m.messages[1])
	}

	summary := contextx.SummarizeMessages(old, 2000)
	if m.messages[0].Content != summary.Text {
		t.Fatalf("unexpected summary content:\n got: %q\nwant: %q", m.messages[0].Content, summary.Text)
	}
	if !strings.Contains(m.messages[0].Content, "first question") {
		t.Fatalf("expected summary to contain old message content, got %q", m.messages[0].Content)
	}

	if m.status != i18n.T("status.compacted", len(old)) {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("status.compacted", len(old)))
	}
	if !m.dirty {
		t.Fatal("expected dirty=true after /compact")
	}
	if !m.followBottom {
		t.Fatal("expected followBottom=true after /compact")
	}
	if m.window == nil || m.window.Len() != 2 {
		t.Fatalf("expected window to contain 2 messages, got %#v", m.window)
	}
}

func TestCompactCommandEmptyMessages(t *testing.T) {
	m := Model{}

	m.handleCommand("/compact")

	if m.status != i18n.T("status.compact_empty") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("status.compact_empty"))
	}
	if len(m.messages) != 0 {
		t.Fatalf("expected messages to stay empty, got %#v", m.messages)
	}
}

func TestCompactCommandSingleMessage(t *testing.T) {
	msg := client.Message{Role: "user", Content: "only message"}
	m := Model{messages: []client.Message{msg}}

	m.handleCommand("/compact")

	if m.status != i18n.T("status.compact_empty") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("status.compact_empty"))
	}
	if len(m.messages) != 1 || m.messages[0] != msg {
		t.Fatalf("expected the single message to be preserved, got %#v", m.messages)
	}
}
