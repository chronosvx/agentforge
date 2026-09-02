package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestClearCommandClearsMessagesAndSetsDirty(t *testing.T) {
	m := Model{
		messages:      []client.Message{{Role: "user", Content: "hello"}},
		context:       map[string]any{"totalTokens": 10},
		cost:          map[string]any{"estimatedYuan": 0.1},
		diffs:         []any{map[string]any{"file": "a.txt"}},
		todos:         []any{map[string]any{"id": "1"}},
		tools:         []any{map[string]any{"name": "bash"}},
		searchResults: []session.Result{{SessionID: "s1", Content: "hi"}},
	}

	m.handleCommand("/clear")

	if len(m.messages) != 0 {
		t.Fatalf("messages not cleared: %#v", m.messages)
	}
	if m.context != nil || m.cost != nil || m.diffs != nil || m.todos != nil || m.tools != nil || m.searchResults != nil {
		t.Fatalf("expected session state fields to be cleared: ctx=%#v cost=%#v diffs=%#v todos=%#v tools=%#v search=%#v",
			m.context, m.cost, m.diffs, m.todos, m.tools, m.searchResults)
	}
	if !m.dirty {
		t.Fatal("expected dirty=true after /clear")
	}
	if !m.followBottom {
		t.Fatal("expected followBottom=true after /clear")
	}
	if m.status != i18n.T("status.cleared") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("status.cleared"))
	}
}

func TestStatusCommandShowsSessionSummary(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang(i18n.LangEN)

	m := Model{
		cfg: &config.Config{
			Workspace: "/work",
			Theme:     "catppuccin",
			Lang:      "en",
		},
		sessionID:  "sess-123",
		messages:   []client.Message{{Role: "user"}, {Role: "assistant"}},
		tokenUsage: map[string]any{"outputTokens": 42},
		cost:       map[string]any{"estimatedYuan": 0.5},
	}

	m.handleCommand("/status")

	for _, want := range []string{"sess-123", "/work", "catppuccin", "42", "0.5", "lang: en"} {
		if !strings.Contains(m.status, want) {
			t.Fatalf("status = %q, want it to contain %q", m.status, want)
		}
	}
}
