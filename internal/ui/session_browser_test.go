package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

func sessionBrowserTestModel() Model {
	return Model{
		panel:        panelSessions,
		panelFocused: true,
		sessionID:    "current",
		sessions: []session.Entry{
			{ID: "current", Title: "Current", UpdatedAt: time.Date(2025, 2, 3, 4, 5, 0, 0, time.UTC)},
			{ID: "other", Title: "Other", UpdatedAt: time.Date(2025, 2, 4, 5, 6, 0, 0, time.UTC)},
			{ID: "third", Title: "Third", UpdatedAt: time.Date(2025, 2, 5, 6, 7, 0, 0, time.UTC)},
		},
	}
}

func TestSessionBrowserCursorMovesDownAndUp(t *testing.T) {
	m := sessionBrowserTestModel()

	for _, tc := range []struct {
		key  tea.KeyMsg
		want int
	}{
		{key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, want: 1},
		{key: tea.KeyMsg{Type: tea.KeyCtrlN}, want: 2},
		{key: tea.KeyMsg{Type: tea.KeyDown}, want: 2}, // stays at bottom
		{key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, want: 1},
		{key: tea.KeyMsg{Type: tea.KeyUp}, want: 0},
		{key: tea.KeyMsg{Type: tea.KeyUp}, want: 0}, // stays at top
	} {
		res, _ := m.Update(tc.key)
		updated := res.(Model)
		if updated.sessionCursor != tc.want {
			t.Fatalf("after %q expected sessionCursor %d, got %d", tc.key.String(), tc.want, updated.sessionCursor)
		}
		m = updated
	}
}

func TestSessionBrowserEnterReturnsSwitchSessionCmd(t *testing.T) {
	m := sessionBrowserTestModel()
	m.sessionCursor = 1

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := res.(Model)
	if updated.sessionCursor != 1 {
		t.Fatalf("expected cursor to stay at 1, got %d", updated.sessionCursor)
	}
	if cmd == nil {
		t.Fatal("expected a switch command for the selected session")
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

func TestSessionBrowserEnterOnCurrentSessionClosesPanel(t *testing.T) {
	m := sessionBrowserTestModel()
	m.sessionCursor = 0

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := res.(Model)
	if cmd != nil {
		t.Fatal("expected no command when selecting the current session")
	}
	if updated.panel != panelNone {
		t.Fatalf("expected sessions panel to close, got %v", updated.panel)
	}
	if updated.panelFocused {
		t.Fatal("expected panel focus to be cleared")
	}
}

func TestSessionBrowserNRequestNewSession(t *testing.T) {
	m := sessionBrowserTestModel()
	m.requestNewSessionFromPanel()

	if !m.pendingNew {
		t.Fatal("expected n to set pendingNew")
	}
	if m.panel != panelNone {
		t.Fatalf("expected n to close sessions panel, got %v", m.panel)
	}
	if m.panelFocused {
		t.Fatal("expected panel focus to be cleared after n")
	}
}

func TestSessionBrowserNKeyStartsNewSessionAndClosesPanel(t *testing.T) {
	m := sessionBrowserTestModel()

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	updated := res.(Model)
	if updated.panel != panelNone {
		t.Fatalf("expected sessions panel to close after n, got %v", updated.panel)
	}
	if updated.panelFocused {
		t.Fatal("expected panel focus to be cleared after n")
	}
	if updated.sessionID != "" {
		t.Fatalf("expected n to start a new empty session, got %q", updated.sessionID)
	}
}

func TestRenderSessionsPanelShowsCursor(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)
	i18n.SetLang(i18n.LangEN)

	m := Model{
		sessionID: "current",
		sessions: []session.Entry{
			{ID: "current", Title: "Current"},
			{ID: "other", Title: "Other"},
		},
		sessionCursor: 1,
	}

	got := m.renderSessionsPanel()
	if !strings.Contains(got, ">") {
		t.Fatalf("expected cursor marker > in rendered sessions panel, got:\n%s", got)
	}
	if !strings.Contains(got, "*") {
		t.Fatalf("expected current session marker * in rendered sessions panel, got:\n%s", got)
	}
	if !strings.Contains(got, "Other") {
		t.Fatalf("expected cursor row content in rendered sessions panel, got:\n%s", got)
	}
	if !strings.Contains(got, i18n.T("panel.sessions_hint")) {
		t.Fatalf("expected sessions hint in rendered panel, got:\n%s", got)
	}
}

func TestResumeAliasOpensSessionsPanel(t *testing.T) {
	m := Model{cl: &fakeBackend{}}
	m.handleCommand("/resume")
	if m.panel != panelSessions {
		t.Fatalf("expected /resume to open sessions panel, got %v", m.panel)
	}
	m.handleCommand("/resume")
	if m.panel != panelNone {
		t.Fatalf("expected /resume to close sessions panel when already open, got %v", m.panel)
	}
}
