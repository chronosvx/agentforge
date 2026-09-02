package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestRenderSessionsPanelShowsTitleWorkspaceAndHint(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)
	i18n.SetLang(i18n.LangEN)

	m := Model{
		sessionID: "sess-1",
		sessions: []session.Entry{
			{
				ID:        "sess-1",
				Title:     "Project plan",
				Workspace: "/work/demo",
				UpdatedAt: time.Date(2025, 2, 3, 4, 5, 0, 0, time.UTC),
			},
		},
	}

	got := m.renderSessionsPanel()
	for _, want := range []string{
		"Project plan",
		i18n.T("panel.sessions_workspace", "/work/demo"),
		"02-03 04:05",
		i18n.T("panel.sessions_hint"),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSessionsPanel missing %q, got:\n%s", want, got)
		}
	}
}

func TestRenderSessionsPanelFallsBackToIDWhenNoTitle(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)
	i18n.SetLang(i18n.LangEN)

	m := Model{
		sessions: []session.Entry{
			{
				ID:        "session-abc",
				Workspace: "",
				UpdatedAt: time.Date(2025, 2, 3, 4, 5, 0, 0, time.UTC),
			},
		},
	}

	got := m.renderSessionsPanel()
	if !strings.Contains(got, "session-abc") {
		t.Fatalf("renderSessionsPanel should fall back to ID, got:\n%s", got)
	}
	if strings.Contains(got, "Workspace:") {
		t.Fatalf("renderSessionsPanel should omit empty workspace, got:\n%s", got)
	}
}
