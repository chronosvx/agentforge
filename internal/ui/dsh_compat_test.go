package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestDshThemeCommandShowsCurrentThemeWithoutArgs(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang(i18n.LangEN)

	m := Model{cfg: &config.Config{Theme: "dracula"}}
	m.handleCommand("/theme")

	want := i18n.T("status.current_theme", "dracula")
	if m.status != want {
		t.Fatalf("status = %q, want %q", m.status, want)
	}
}

func TestDshThemeCommandSwitchesThemeWithArg(t *testing.T) {
	m := Model{cfg: &config.Config{Theme: "default"}}
	m.handleCommand("/theme tokyo-night")

	if m.cfg == nil || m.cfg.Theme != "tokyo-night" {
		t.Fatalf("cfg.Theme = %#v, want tokyo-night", m.cfg)
	}
	if !m.dirty {
		t.Fatal("expected dirty=true after switching theme")
	}
	if !m.redraw {
		t.Fatal("expected redraw=true after switching theme")
	}
	want := i18n.T("status.theme_switched", "tokyo-night")
	if m.status != want {
		t.Fatalf("status = %q, want %q", m.status, want)
	}
}

func TestDshExitAndQuitCommandsSetStopState(t *testing.T) {
	for _, cmd := range []string{"/exit", "/quit"} {
		t.Run(cmd, func(t *testing.T) {
			m := Model{cfg: &config.Config{}}
			got := m.handleCommand(cmd)

			if !m.stopRequested {
				t.Fatal("expected stopRequested=true after " + cmd)
			}
			if got == nil {
				t.Fatal("expected non-nil tea.Cmd after " + cmd)
			}
			if _, ok := got().(tea.QuitMsg); !ok {
				t.Fatalf("expected tea.QuitMsg after %s, got %T", cmd, got())
			}
		})
	}
}

func TestDshAltUpRecallsLastUserMessage(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang(i18n.LangEN)

	input := textarea.New()
	input.Focus()
	m := Model{
		input: input,
		messages: []client.Message{
			{Role: "user", Content: "first user message"},
			{Role: "assistant", Content: "assistant reply"},
			{Role: "user", Content: "last user message"},
		},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	updated := res.(Model)

	if got := updated.input.Value(); got != "last user message" {
		t.Fatalf("input = %q, want %q", got, "last user message")
	}
	if updated.status != i18n.T("status.user_message_recalled") {
		t.Fatalf("status = %q, want %q", updated.status, i18n.T("status.user_message_recalled"))
	}
}

func TestDshAltUpWithoutUserMessageShowsPrompt(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang(i18n.LangEN)

	input := textarea.New()
	input.Focus()
	m := Model{
		input:    input,
		messages: []client.Message{{Role: "assistant", Content: "hello"}},
	}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp, Alt: true})
	updated := res.(Model)

	if updated.status != i18n.T("status.no_user_message") {
		t.Fatalf("status = %q, want %q", updated.status, i18n.T("status.no_user_message"))
	}
}

func TestDshCtrlLRequestsRedraw(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang(i18n.LangEN)

	m := Model{input: textarea.New()}
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	updated := res.(Model)

	if !updated.redraw {
		t.Fatal("expected redraw=true after Ctrl+L")
	}
	if !updated.redrawn {
		t.Fatal("expected redrawn=true after Ctrl+L")
	}
	if !updated.dirty {
		t.Fatal("expected dirty=true after Ctrl+L")
	}
	if updated.status != i18n.T("status.redrawn") {
		t.Fatalf("status = %q, want %q", updated.status, i18n.T("status.redrawn"))
	}
}

func TestDshThemeInCommandPalette(t *testing.T) {
	found := false
	for _, cmd := range commandPaletteCommands {
		if cmd == "/theme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected /theme in commandPaletteCommands")
	}
	found = false
	for _, cmd := range basicCommandPaletteCommands {
		if cmd == "/theme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected /theme in basicCommandPaletteCommands")
	}
}

func TestDshHelpIncludesCompatShortcuts(t *testing.T) {
	prevLang := i18n.CurrentLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })

	for _, lang := range []i18n.Lang{i18n.LangZH, i18n.LangEN} {
		i18n.SetLang(lang)
		joined := strings.Join((&Model{}).helpLines(), "\n")
		if !strings.Contains(joined, "Ctrl+L") {
			t.Fatalf("helpLines (%s) missing Ctrl+L: %q", lang, joined)
		}
		if !strings.Contains(joined, "Alt+Up") {
			t.Fatalf("helpLines (%s) missing Alt+Up: %q", lang, joined)
		}
	}
}
