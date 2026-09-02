package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestPersistNowUsesSessionTitle(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		store:        store,
		sessionID:    "s1",
		cfg:          &config.Config{Workspace: "ws"},
		persistDirty: true,
		sessionTitle: "Renamed title",
		messages: []client.Message{
			{Role: "user", Content: "this should not override the renamed title"},
		},
	}

	m.persistNow()

	entry, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "Renamed title" {
		t.Fatalf("title = %q, want %q", entry.Title, "Renamed title")
	}
	if m.sessionTitle != "Renamed title" {
		t.Fatalf("sessionTitle = %q, want %q", m.sessionTitle, "Renamed title")
	}
}

func TestPersistNowAutoTitlesFromFirstUserMessage(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	longMessage := "hello this is a fairly long first user message that should be truncated"
	m := Model{
		store:        store,
		sessionID:    "s1",
		cfg:          &config.Config{Workspace: "ws"},
		persistDirty: true,
		messages: []client.Message{
			{Role: "user", Content: longMessage},
			{Role: "assistant", Content: "ignored"},
		},
	}

	m.persistNow()

	wantTitle := string([]rune(longMessage)[:40])
	entry, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != wantTitle {
		t.Fatalf("title = %q, want %q", entry.Title, wantTitle)
	}
	if m.sessionTitle != wantTitle {
		t.Fatalf("sessionTitle = %q, want %q", m.sessionTitle, wantTitle)
	}
}

func TestRenameCommandSetsSessionTitle(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	m := Model{store: store, sessionID: "s1"}

	m.handleCommand("/rename New Title")

	if m.sessionTitle != "New Title" {
		t.Fatalf("sessionTitle = %q, want %q", m.sessionTitle, "New Title")
	}
	entry, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "New Title" {
		t.Fatalf("stored title = %q, want %q", entry.Title, "New Title")
	}
}

func TestNewLoadsSessionTitleFromEntry(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s1", Title: "Existing title"}); err != nil {
		t.Fatal(err)
	}

	m := New(&config.Config{DataDir: t.TempDir(), SessionID: "s1"}, nil, store)
	if m.sessionTitle != "Existing title" {
		t.Fatalf("sessionTitle = %q, want %q", m.sessionTitle, "Existing title")
	}
}

func TestSwitchSessionLoadsSessionTitle(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s1", Title: "Old title"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s2", Title: "Switched title"}); err != nil {
		t.Fatal(err)
	}

	m := Model{store: store, sessionID: "s1", sessionTitle: "Old title"}
	res, _ := m.Update(switchSessionMsg{id: "s2"})
	updated := res.(Model)
	if updated.sessionTitle != "Switched title" {
		t.Fatalf("sessionTitle = %q, want %q", updated.sessionTitle, "Switched title")
	}
}

func TestNewCommandClearsSessionTitle(t *testing.T) {
	m := Model{sessionTitle: "Old title"}
	m.handleCommand("/new")
	if m.sessionTitle != "" {
		t.Fatalf("sessionTitle = %q, want empty after /new", m.sessionTitle)
	}
}

func TestRenderSessionsPanelGroupsByWorkspace(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)
	i18n.SetLang(i18n.LangEN)

	m := Model{
		sessionID: "s1",
		sessions: []session.Entry{
			{ID: "s1", Title: "Alpha", Workspace: "/work/one", UpdatedAt: time.Date(2025, 2, 3, 4, 5, 0, 0, time.UTC)},
			{ID: "s2", Title: "Beta", Workspace: "/work/one", UpdatedAt: time.Date(2025, 2, 4, 5, 6, 0, 0, time.UTC)},
			{ID: "s3", Title: "Gamma", Workspace: "/work/two", UpdatedAt: time.Date(2025, 2, 5, 6, 7, 0, 0, time.UTC)},
			{ID: "s4", Title: "No workspace", Workspace: "", UpdatedAt: time.Date(2025, 2, 6, 7, 8, 0, 0, time.UTC)},
		},
	}

	got := m.renderSessionsPanel()
	for _, want := range []string{
		i18n.T("panel.sessions_workspace", "/work/one"),
		i18n.T("panel.sessions_workspace", "/work/two"),
		"Alpha",
		"Beta",
		"Gamma",
		"No workspace",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSessionsPanel missing %q, got:\n%s", want, got)
		}
	}
	if strings.Count(got, "Workspace:") != 2 {
		t.Fatalf("expected exactly two workspace group headings, got:\n%s", got)
	}
	if strings.Index(got, "/work/one") > strings.Index(got, "/work/two") {
		t.Fatalf("expected /work/one group before /work/two, got:\n%s", got)
	}
}
