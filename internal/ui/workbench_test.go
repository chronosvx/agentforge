package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestNewDefaultsToWorkbenchMode(t *testing.T) {
	dir := t.TempDir()
	m := New(&config.Config{DataDir: dir, Lang: "zh"}, &fakeBackend{}, nil)
	if !m.workbenchMode {
		t.Fatal("expected New to default to workbenchMode=true")
	}
	if !strings.Contains(m.input.Placeholder, "输入任务或命令") {
		t.Fatalf("workbench placeholder = %q, want task/command placeholder", m.input.Placeholder)
	}
}

func TestChatAndWorkbenchCommandsToggleMode(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)
	i18n.SetLang(i18n.LangZH)

	m := Model{workbenchMode: true, input: textarea.New()}

	m.handleCommand("/chat")
	if m.workbenchMode {
		t.Fatal("expected /chat to set workbenchMode=false")
	}
	if !strings.Contains(m.input.Placeholder, "输入消息") {
		t.Fatalf("chat placeholder = %q, want classic chat placeholder", m.input.Placeholder)
	}

	m.handleCommand("/workbench")
	if !m.workbenchMode {
		t.Fatal("expected /workbench to set workbenchMode=true")
	}
	if !strings.Contains(m.input.Placeholder, "输入任务或命令") {
		t.Fatalf("workbench placeholder = %q, want task/command placeholder", m.input.Placeholder)
	}
}

func TestWorkbenchRenderDoesNotShowAssistantBubble(t *testing.T) {
	m := Model{
		workbenchMode: true,
		messages: []client.Message{
			{Role: "user", Content: "do the task"},
			{Role: "assistant", Content: "done"},
		},
		viewport: viewport.New(80, 10),
	}
	m.dirty = true
	m.renderMessages()

	content := m.viewport.View()
	if !strings.Contains(content, "Task") {
		t.Fatalf("workbench render missing Task row: %q", content)
	}
	if !strings.Contains(content, "Result") {
		t.Fatalf("workbench render missing Result row: %q", content)
	}
	if strings.Contains(content, "assistant") {
		t.Fatalf("workbench render should not show assistant chat bubble: %q", content)
	}
}

func TestMouseWheelChangesMainScrollOffset(t *testing.T) {
	messages := make([]client.Message, 50)
	for i := range messages {
		messages[i] = client.Message{Role: "user", Content: "msg"}
	}
	m := Model{
		workbenchMode: true,
		messages:      messages,
		viewport:      viewport.New(80, 10),
		scrollOffset:  40,
		followBottom:  false,
	}

	res, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	up := res.(Model)
	if up.scrollOffset >= 40 {
		t.Fatalf("wheel up should decrease scrollOffset, got %d", up.scrollOffset)
	}

	res, _ = up.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	down := res.(Model)
	if down.scrollOffset <= up.scrollOffset {
		t.Fatalf("wheel down should increase scrollOffset, got %d after up=%d", down.scrollOffset, up.scrollOffset)
	}
}

func TestAltEnterInsertsNewlineInInput(t *testing.T) {
	m := Model{input: textarea.New()}
	m.input.SetValue("line one")

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	got := res.(Model)

	if !strings.Contains(got.input.Value(), "\n") {
		t.Fatalf("alt+enter should insert newline, got %q", got.input.Value())
	}
	if got.input.Value() != "line one\n" {
		t.Fatalf("alt+enter value = %q, want %q", got.input.Value(), "line one\n")
	}
}
