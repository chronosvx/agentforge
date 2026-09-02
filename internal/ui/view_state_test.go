package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

// newViewStateModel returns a ready Model with a stable English locale and a
// small empty workspace so mention scanning stays fast and deterministic.
func newViewStateModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir:   dir,
		Workspace: dir,
		Lang:      "en",
	}
	m := New(cfg, &fakeBackend{}, nil)
	res, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })
	return res.(Model)
}

func TestWindowSizeUpdateDoesNotPanicAndLayoutIsSafe(t *testing.T) {
	tests := []struct {
		name        string
		width       int
		height      int
		narrow      bool
		expectReady bool
	}{
		{name: "zero", width: 0, height: 0, narrow: false, expectReady: true},
		{name: "zero-width-small-height", width: 0, height: 1, narrow: false, expectReady: true},
		{name: "one-by-one", width: 1, height: 1, narrow: true, expectReady: true},
		{name: "one-by-five", width: 1, height: 5, narrow: true, expectReady: true},
		{name: "ten-by-one", width: 10, height: 1, narrow: true, expectReady: true},
		{name: "ten-by-five", width: 10, height: 5, narrow: true, expectReady: true},
		{name: "seventy-nine", width: 79, height: 1, narrow: true, expectReady: true},
		{name: "eighty", width: 80, height: 1, narrow: false, expectReady: true},
		{name: "eighty-one", width: 81, height: 1, narrow: false, expectReady: true},
		{name: "small-height", width: 100, height: 1, narrow: false, expectReady: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newViewStateModel(t)
			m.tabs = []session.Entry{{ID: "s1"}, {ID: "s2"}}
			m.sessionID = "s1"
			m.rightPanel = true
			m.splitEnabled = true
			m.splitSessionID = "s2"
			m.messages = []client.Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "world reply"},
			}

			res, _ := m.Update(tea.WindowSizeMsg{Width: tt.width, Height: tt.height})
			got, ok := res.(Model)
			if !ok {
				t.Fatalf("unexpected model type %T", res)
			}
			if got.ready != tt.expectReady {
				t.Fatalf("ready = %v, want %v", got.ready, tt.expectReady)
			}
			if got.width != tt.width || got.height != tt.height {
				t.Fatalf("stored size = %dx%d, want %dx%d", got.width, got.height, tt.width, tt.height)
			}
			if got.viewport.Width < 0 {
				t.Fatalf("viewport width must not be negative, got %d", got.viewport.Width)
			}
			if got.viewport.Height < 1 {
				t.Fatalf("viewport height must be at least 1, got %d", got.viewport.Height)
			}

			// A zero-width/zero-height message is effectively ignored by layout,
			// so existing panel state is preserved. Any positive width below the
			// narrow threshold must hide both auxiliary columns.
			if tt.width > 0 && tt.height > 0 {
				if tt.narrow {
					if got.rightPanel || got.splitEnabled || got.splitSessionID != "" {
						t.Fatalf("narrow layout should hide right panel and split, got right=%v split=%v splitID=%q",
							got.rightPanel, got.splitEnabled, got.splitSessionID)
					}
				} else if !got.rightPanel || !got.splitEnabled || got.splitSessionID == "" {
					t.Fatalf("wide layout should preserve right panel and split, got right=%v split=%v splitID=%q",
						got.rightPanel, got.splitEnabled, got.splitSessionID)
				}
			}

			// View must also be safe after tiny window sizes.
			_ = got.View()
		})
	}
}

func TestUpdateConnectedAndStreamMessages(t *testing.T) {
	m := newViewStateModel(t)
	m.sessionEpoch = 7
	events := make(chan client.Event)

	state := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"role": "assistant", "content": "hi"},
		},
		"context": map[string]any{"totalTokens": float64(12)},
		"cost":    map[string]any{"estimatedYuan": 0.1},
	}
	res, cmd := m.Update(connectedMsg{
		snapshot: &client.Snapshot{ChannelID: "ch1", Version: 10, State: state},
		events:   events,
		cancel:   func() {},
		epoch:    7,
	})
	got := res.(Model)
	if got.channelID != "ch1" {
		t.Fatalf("channelID = %q, want ch1", got.channelID)
	}
	if got.events == nil {
		t.Fatal("events channel was not stored")
	}
	if got.lastEventVersion != 10 {
		t.Fatalf("lastEventVersion = %d, want 10", got.lastEventVersion)
	}
	if len(got.messages) != 2 || got.messages[1].Content != "hi" {
		t.Fatalf("unexpected connected messages: %#v", got.messages)
	}
	if got.status != "connected: ch1" {
		t.Fatalf("status = %q, want connected: ch1", got.status)
	}
	if cmd == nil {
		t.Fatal("expected a wait-event command after connectedMsg")
	}

	// A normal event on the active channel updates state and schedules the next read.
	eventState := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"role": "assistant", "content": "hi there"},
		},
	}
	res, cmd = got.Update(eventMsg{
		channelID: "ch1",
		event: client.Event{
			ChannelID: "ch1",
			Version:   11,
			State:     eventState,
		},
	})
	got = res.(Model)
	if got.status != "streaming..." {
		t.Fatalf("status = %q, want streaming...", got.status)
	}
	if len(got.messages) != 2 || got.messages[1].Content != "hi there" {
		t.Fatalf("event did not update messages: %#v", got.messages)
	}
	if got.lastEventVersion != 11 {
		t.Fatalf("lastEventVersion = %d, want 11", got.lastEventVersion)
	}
	if cmd == nil {
		t.Fatal("expected a wait-event command after eventMsg")
	}

	// Disconnect only applies when epoch matches.
	res, cmd = got.Update(disconnectedMsg{epoch: 7})
	got = res.(Model)
	if got.err == nil || !strings.Contains(got.err.Error(), "closed") {
		t.Fatalf("err = %v, want event stream closed", got.err)
	}
	if !strings.Contains(got.status, "disconnected") {
		t.Fatalf("status = %q, want disconnected text", got.status)
	}
	if got.eventCancel != nil {
		t.Fatal("eventCancel should be cleared after disconnect")
	}
	if cmd == nil {
		t.Fatal("expected a reconnect tick after disconnectedMsg")
	}

	// Reconnect schedules a fresh connect command.
	res, cmd = got.Update(reconnectMsg{})
	got = res.(Model)
	if !strings.Contains(got.status, "reconnecting") {
		t.Fatalf("status = %q, want reconnecting text", got.status)
	}
	if cmd == nil {
		t.Fatal("expected a connect command after reconnectMsg")
	}

	// errMsg records both the field and status.
	res, _ = got.Update(errMsg{err: errors.New("boom")})
	got = res.(Model)
	if got.err == nil || got.err.Error() != "boom" {
		t.Fatalf("err = %v, want boom", got.err)
	}
	if !strings.Contains(got.status, "error: boom") {
		t.Fatalf("status = %q, want error: boom", got.status)
	}
}

func TestStaleEpochConnectedAndDisconnectedAreIgnored(t *testing.T) {
	m := newViewStateModel(t)
	m.sessionEpoch = 3
	cancelCalled := false

	res, _ := m.Update(connectedMsg{
		snapshot: &client.Snapshot{ChannelID: "old", State: map[string]any{
			"messages": []any{map[string]any{"role": "assistant", "content": "stale"}},
		}},
		events: make(chan client.Event),
		cancel: func() { cancelCalled = true },
		epoch:  2,
	})
	got := res.(Model)
	if got.channelID != "" {
		t.Fatalf("stale connectedMsg changed channelID to %q", got.channelID)
	}
	if len(got.messages) != 0 {
		t.Fatalf("stale connectedMsg changed messages: %#v", got.messages)
	}
	if !cancelCalled {
		t.Fatal("stale connectedMsg should cancel its own subscription")
	}

	res, _ = m.Update(disconnectedMsg{epoch: 2})
	got = res.(Model)
	if got.err != nil {
		t.Fatalf("stale disconnectedMsg set err: %v", got.err)
	}
}

func TestEventMsgDuplicateLateAndOutOfOrder(t *testing.T) {
	m := newViewStateModel(t)
	m.sessionEpoch = 1
	res, _ := m.Update(connectedMsg{
		snapshot: &client.Snapshot{ChannelID: "ch1", Version: 100, State: map[string]any{
			"messages": []any{map[string]any{"role": "assistant", "content": "base"}},
		}},
		events: make(chan client.Event),
		cancel: func() {},
		epoch:  1,
	})
	m = res.(Model)

	newerState := map[string]any{
		"messages": []any{map[string]any{"role": "assistant", "content": "newer"}},
	}
	olderState := map[string]any{
		"messages": []any{map[string]any{"role": "assistant", "content": "older"}},
	}

	// Apply a newer event first.
	res, _ = m.Update(eventMsg{
		channelID: "ch1",
		event:     client.Event{ChannelID: "ch1", Version: 102, State: newerState},
	})
	got := res.(Model)
	if len(got.messages) != 1 || got.messages[0].Content != "newer" {
		t.Fatalf("newer event not applied: %#v", got.messages)
	}

	// A duplicate of the same version must not duplicate or regress state.
	res, _ = got.Update(eventMsg{
		channelID: "ch1",
		event:     client.Event{ChannelID: "ch1", Version: 102, State: newerState},
	})
	got = res.(Model)
	if len(got.messages) != 1 || got.messages[0].Content != "newer" {
		t.Fatalf("duplicate event changed messages: %#v", got.messages)
	}

	// An older event arriving late on the same channel must not overwrite the newer state.
	res, _ = got.Update(eventMsg{
		channelID: "ch1",
		event:     client.Event{ChannelID: "ch1", Version: 101, State: olderState},
	})
	got = res.(Model)
	if len(got.messages) != 1 || got.messages[0].Content != "newer" {
		t.Fatalf("out-of-order event regressed state: %#v", got.messages)
	}

	// A late event from an old channel must be ignored entirely.
	res, _ = got.Update(eventMsg{
		channelID: "old",
		event: client.Event{ChannelID: "old", Version: 999, State: map[string]any{
			"messages": []any{map[string]any{"role": "assistant", "content": "intruder"}},
		}},
	})
	got = res.(Model)
	if len(got.messages) != 1 || got.messages[0].Content != "newer" {
		t.Fatalf("late old-channel event changed messages: %#v", got.messages)
	}

	// Unversioned events still flow through (backwards compatibility with tests/mocks).
	res, _ = got.Update(eventMsg{
		channelID: "ch1",
		event: client.Event{ChannelID: "ch1", State: map[string]any{
			"messages": []any{map[string]any{"role": "assistant", "content": "unversioned"}},
		}},
	})
	got = res.(Model)
	if len(got.messages) != 1 || got.messages[0].Content != "unversioned" {
		t.Fatalf("unversioned event was not applied: %#v", got.messages)
	}
}

func TestViewRendersKeyOverlayStates(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *Model)
		wantFrag string
	}{
		{
			name: "help",
			prepare: func(_ *testing.T, m *Model) {
				m.openHelp()
			},
			wantFrag: "Help",
		},
		{
			name: "command palette",
			prepare: func(_ *testing.T, m *Model) {
				m.openCommandPalette()
			},
			wantFrag: "/context",
		},
		{
			name: "mention",
			prepare: func(_ *testing.T, m *Model) {
				m.openMention()
			},
			wantFrag: "File reference",
		},
		{
			name: "rewind",
			prepare: func(_ *testing.T, m *Model) {
				m.messages = []client.Message{
					{Role: "user", Content: "please refactor"},
					{Role: "assistant", Content: "done"},
				}
				m.openRewind()
			},
			wantFrag: "Rewind",
		},
		{
			name: "mcp approval",
			prepare: func(_ *testing.T, m *Model) {
				m.mcpCallCmd("files.read", map[string]any{"path": "/tmp/example"})
			},
			wantFrag: "files.read",
		},
		{
			name: "sessions panel",
			prepare: func(_ *testing.T, m *Model) {
				m.panel = panelSessions
				m.sessionCursor = 0
				m.sessions = []session.Entry{{ID: "s1", Title: "Alpha"}}
			},
			wantFrag: "Alpha",
		},
		{
			name: "diff panel",
			prepare: func(_ *testing.T, m *Model) {
				m.panel = panelDiff
				m.diffCursor = 0
				m.diffDecisions = map[int]string{}
				m.diffs = []any{map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"}}
			},
			wantFrag: "a.txt",
		},
		{
			name: "split enabled",
			prepare: func(_ *testing.T, m *Model) {
				m.splitEnabled = true
				m.splitSessionID = "s2"
			},
			wantFrag: "Split: s2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newViewStateModel(t)
			tt.prepare(t, &m)

			// Must not panic for any of these overlay/panel states.
			out := m.View()
			if !strings.Contains(out, tt.wantFrag) {
				t.Fatalf("View() output missing %q; got:\n%s", tt.wantFrag, out)
			}
		})
	}
}

func TestViewRendersCombinedPanelsWithoutPanic(t *testing.T) {
	m := newViewStateModel(t)
	m.openHelp()
	m.openCommandPalette()
	m.openMention()
	m.messages = []client.Message{{Role: "user", Content: "hello"}}
	m.openRewind()
	m.mcpCallCmd("files.read", map[string]any{"path": "/tmp/example"})
	m.panel = panelDiff
	m.diffDecisions = map[int]string{}
	m.diffs = []any{map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"}}
	m.splitEnabled = true
	m.splitSessionID = "s2"

	out := m.View()
	if !strings.Contains(out, "Help") {
		t.Fatalf("combined view missing help content:\n%s", out)
	}
	if !strings.Contains(out, "files.read") {
		t.Fatalf("combined view missing MCP approval content:\n%s", out)
	}
}
