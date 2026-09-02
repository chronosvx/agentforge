package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestRewindOpenCollectsUserMessages(t *testing.T) {
	m := Model{
		messages: []client.Message{
			{Role: "assistant", Content: "assistant 1"},
			{Role: "user", Content: "user 1"},
			{Role: "user", Content: "user 2"},
			{Role: "system", Content: "system 1"},
		},
	}

	m.openRewind()

	if !m.rewindOpen {
		t.Fatal("expected rewindOpen to be true")
	}
	if m.rewindCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.rewindCursor)
	}
	if len(m.rewindCandidates) != 2 {
		t.Fatalf("expected 2 user candidates, got %d", len(m.rewindCandidates))
	}
	if m.rewindCandidates[0].Content != "user 1" || m.rewindCandidates[1].Content != "user 2" {
		t.Fatalf("unexpected candidates: %#v", m.rewindCandidates)
	}
}

func TestRewindEnterRefillsAndTruncatesMessages(t *testing.T) {
	input := textarea.New()
	m := Model{
		messages: []client.Message{
			{Role: "user", Content: "first user"},
			{Role: "assistant", Content: "assistant reply"},
			{Role: "user", Content: "second user"},
			{Role: "assistant", Content: "later reply"},
		},
		input: input,
	}

	m.openRewind()
	m.rewindCursor = 1

	if !m.handleRewindKey("enter") {
		t.Fatal("expected enter to be consumed")
	}
	if m.rewindOpen {
		t.Fatal("expected rewind to close after picking")
	}
	if len(m.rewindCandidates) != 0 {
		t.Fatalf("expected candidates cleared, got %d", len(m.rewindCandidates))
	}
	if m.rewindCursor != 0 {
		t.Fatalf("expected cursor reset to 0, got %d", m.rewindCursor)
	}
	if len(m.messages) != 2 {
		t.Fatalf("expected messages truncated to before selected message, got %d: %#v", len(m.messages), m.messages)
	}
	if m.messages[0].Content != "first user" || m.messages[1].Content != "assistant reply" {
		t.Fatalf("unexpected truncated messages: %#v", m.messages)
	}
	if m.input.Value() != "second user" {
		t.Fatalf("expected input value %q, got %q", "second user", m.input.Value())
	}
	if !m.dirty {
		t.Fatal("expected dirty to be true")
	}
	if !m.followBottom {
		t.Fatal("expected followBottom to be true")
	}
	if m.status != i18n.T("rewind.picked") {
		t.Fatalf("expected status %q, got %q", i18n.T("rewind.picked"), m.status)
	}
}

func TestRewindNavigation(t *testing.T) {
	m := Model{
		messages: []client.Message{
			{Role: "user", Content: "a"},
			{Role: "user", Content: "b"},
			{Role: "user", Content: "c"},
		},
	}
	m.openRewind()

	if !m.handleRewindKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.rewindCursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.rewindCursor)
	}

	if !m.handleRewindKey("ctrl+n") {
		t.Fatal("expected ctrl+n to be consumed")
	}
	if m.rewindCursor != 2 {
		t.Fatalf("expected cursor 2 after ctrl+n, got %d", m.rewindCursor)
	}

	if !m.handleRewindKey("down") {
		t.Fatal("expected down to be consumed")
	}
	if m.rewindCursor != 2 {
		t.Fatalf("expected cursor to stay at last item, got %d", m.rewindCursor)
	}

	if !m.handleRewindKey("k") {
		t.Fatal("expected k to be consumed")
	}
	if m.rewindCursor != 1 {
		t.Fatalf("expected cursor 1 after k, got %d", m.rewindCursor)
	}

	if !m.handleRewindKey("up") {
		t.Fatal("expected up to be consumed")
	}
	if m.rewindCursor != 0 {
		t.Fatalf("expected cursor 0 after up, got %d", m.rewindCursor)
	}

	if m.handleRewindKey("x") {
		t.Fatal("expected unknown key not to be consumed")
	}
}

func TestRewindEmptyList(t *testing.T) {
	m := Model{
		messages: []client.Message{
			{Role: "assistant", Content: "only assistant"},
		},
	}
	m.openRewind()

	if !m.rewindOpen {
		t.Fatal("expected rewindOpen to be true")
	}
	if len(m.rewindCandidates) != 0 {
		t.Fatalf("expected no candidates, got %d", len(m.rewindCandidates))
	}

	if m.handleRewindKey("enter") {
		t.Fatal("expected enter not to be consumed when there are no candidates")
	}
	if !m.rewindOpen {
		t.Fatal("expected rewind to stay open when enter is not consumable")
	}

	rendered := m.renderRewind()
	if !strings.Contains(rendered, i18n.T("rewind.title")) {
		t.Fatalf("expected title %q in render, got %q", i18n.T("rewind.title"), rendered)
	}
	if !strings.Contains(rendered, i18n.T("rewind.empty")) {
		t.Fatalf("expected empty text %q in render, got %q", i18n.T("rewind.empty"), rendered)
	}
	if !strings.Contains(rendered, i18n.T("rewind.hint")) {
		t.Fatalf("expected hint %q in render, got %q", i18n.T("rewind.hint"), rendered)
	}

	if !m.handleRewindKey("q") {
		t.Fatal("expected q to be consumed")
	}
	if m.rewindOpen {
		t.Fatal("expected rewind to close after q")
	}
	if len(m.rewindCandidates) != 0 || m.rewindCursor != 0 {
		t.Fatalf("expected candidates and cursor cleared, got %d/%d", len(m.rewindCandidates), m.rewindCursor)
	}
}
