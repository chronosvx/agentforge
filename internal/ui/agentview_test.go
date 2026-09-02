package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestAgentViewOpenClose(t *testing.T) {
	m := Model{}
	m.openAgentView()

	if !m.agentViewOpen {
		t.Fatal("expected agentViewOpen to be true after open")
	}
	if m.agentViewCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.agentViewCursor)
	}
	if m.agentViewQuery != "" {
		t.Fatalf("expected empty query, got %q", m.agentViewQuery)
	}

	m.agentViewCursor = 3
	m.agentViewQuery = "abc"
	m.closeAgentView()

	if m.agentViewOpen {
		t.Fatal("expected agentViewOpen to be false after close")
	}
	if m.agentViewCursor != 0 {
		t.Fatalf("expected cursor reset to 0, got %d", m.agentViewCursor)
	}
	if m.agentViewQuery != "" {
		t.Fatalf("expected query reset to empty, got %q", m.agentViewQuery)
	}
}

func TestAgentViewNavigation(t *testing.T) {
	m := Model{
		sessions: []session.Entry{
			{ID: "s1", Title: "one"},
			{ID: "s2", Title: "two"},
			{ID: "s3", Title: "three"},
		},
	}

	for _, tc := range []struct {
		key  string
		want int
	}{
		{"j", 1},
		{"ctrl+n", 2},
		{"down", 2},
		{"k", 1},
		{"up", 0},
	} {
		handled, _ := m.handleAgentViewKey(tc.key)
		if !handled {
			t.Fatalf("expected %q to be consumed", tc.key)
		}
		if m.agentViewCursor != tc.want {
			t.Fatalf("after %q expected cursor %d, got %d", tc.key, tc.want, m.agentViewCursor)
		}
	}

	if handled, _ := m.handleAgentViewKey("x"); handled {
		t.Fatal("expected unknown key not to be consumed")
	}
}

func TestAgentViewEnterSwitchesSession(t *testing.T) {
	m := Model{
		sessionID: "current",
		sessions: []session.Entry{
			{ID: "other"},
			{ID: "current"},
		},
		agentViewCursor: 0,
	}

	handled, cmd := m.handleAgentViewKey("enter")
	if !handled {
		t.Fatal("expected enter to be consumed")
	}
	if cmd == nil {
		t.Fatal("expected switch command for selected session")
	}
	msg := cmd()
	sw, ok := msg.(switchSessionMsg)
	if !ok {
		t.Fatalf("expected switchSessionMsg, got %T", msg)
	}
	if sw.id != "other" {
		t.Fatalf("expected switch to other, got %q", sw.id)
	}
}

func TestAgentViewEnterOnCurrentSessionCloses(t *testing.T) {
	m := Model{
		sessionID: "current",
		sessions: []session.Entry{
			{ID: "current"},
		},
		agentViewCursor: 0,
		agentViewOpen:   true,
	}

	handled, cmd := m.handleAgentViewKey("enter")
	if !handled {
		t.Fatal("expected enter to be consumed")
	}
	if cmd != nil {
		t.Fatal("expected no command when selecting current session")
	}
	if m.agentViewOpen {
		t.Fatal("expected agent view to close when selecting current session")
	}
}

func TestAgentViewRenderContainsTitle(t *testing.T) {
	m := Model{
		sessionID: "sess-1",
		sessions: []session.Entry{
			{ID: "sess-1", Title: "Current"},
			{ID: "sess-2", Title: "Another"},
		},
	}

	rendered := m.renderAgentView()
	for _, want := range []string{
		i18n.T("agentview.title"),
		i18n.T("agentview.current"),
		i18n.T("agentview.recent"),
		i18n.T("agentview.hint"),
		"sess-1",
		"Another",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("expected %q in render, got %q", want, rendered)
		}
	}
}
