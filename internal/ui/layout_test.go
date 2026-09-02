package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestViewDoesNotRenderDefaultSidebarOrTabs(t *testing.T) {
	m := newViewStateModel(t)
	m.tabs = []session.Entry{{ID: "s1"}, {ID: "s2"}}
	m.sessionID = "s1"
	m.messages = []client.Message{{Role: "user", Content: "hello"}}

	out := m.View()
	if strings.Contains(out, "Sessions") {
		t.Fatalf("default View() should not render the Sessions sidebar; got:\n%s", out)
	}
	if strings.Contains(out, "▶") {
		t.Fatalf("default View() should not render sidebar session markers; got:\n%s", out)
	}
	if strings.Contains(out, "s2") {
		t.Fatalf("default View() should not render the tab bar with non-current sessions; got:\n%s", out)
	}
}

func TestNewInputStartsAtOneLine(t *testing.T) {
	m := newViewStateModel(t)
	if m.input.Height() != 1 {
		t.Fatalf("input.Height = %d, want 1", m.input.Height())
	}
}

func TestInputHeightGrowsAndShrinks(t *testing.T) {
	m := newViewStateModel(t)

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
	m = res.(Model)
	if m.input.Value() != "hello" || m.input.Height() != 1 {
		t.Fatalf("after one-line input: value=%q height=%d, want height 1", m.input.Value(), m.input.Height())
	}

	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = res.(Model)
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("world")})
	m = res.(Model)
	if m.input.Value() != "hello\nworld" {
		t.Fatalf("unexpected multi-line value: %q", m.input.Value())
	}
	if m.input.Height() != 2 {
		t.Fatalf("input.Height = %d, want 2 for two explicit lines", m.input.Height())
	}

	// Submitting with Enter should clear the textarea and shrink it back to one line.
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(Model)
	if m.input.Value() != "" || m.input.Height() != 1 {
		t.Fatalf("after submit value=%q height=%d, want empty one-line input", m.input.Value(), m.input.Height())
	}

	// Long content should also grow due to automatic wrapping (capped by sync).
	m.input.SetValue(strings.Repeat("x", 500))
	m.syncInputHeight()
	if m.input.Height() <= 1 {
		t.Fatalf("long input should grow above 1 line, got height %d", m.input.Height())
	}

	// Clearing should shrink back to one line.
	m.input.SetValue("")
	m.syncInputHeight()
	if m.input.Height() != 1 {
		t.Fatalf("input.Height after clearing = %d, want 1", m.input.Height())
	}
}

func TestApplyLayoutKeepsViewportWithTallInput(t *testing.T) {
	m := Model{
		width:  60,
		height: 8,
		input:  textarea.New(),
	}
	m.input.SetValue(strings.Repeat("x\n", 50))
	m.applyLayout()

	if m.input.Height() < 1 {
		t.Fatalf("input.Height = %d, want at least 1", m.input.Height())
	}
	if m.viewport.Height < 1 {
		t.Fatalf("viewport.Height = %d, want at least 1 even with a tall input", m.viewport.Height)
	}
	if m.input.Height() > 8 {
		t.Fatalf("input.Height = %d, want max 8", m.input.Height())
	}
}
