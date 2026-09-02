package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestEmptyMessagesRenderWelcome(t *testing.T) {
	i18n.SetLang(i18n.LangZH)
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })

	m := Model{viewport: viewport.New(80, 10)}
	m.dirty = true
	m.renderMessages()

	content := m.viewport.View()
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected non-empty welcome viewport, got empty")
	}
	if !strings.Contains(content, "agentforge") && !strings.Contains(content, "欢迎") {
		t.Fatalf("expected welcome content to mention agentforge or 欢迎, got %q", content)
	}

	i18n.SetLang(i18n.LangEN)
	m = Model{viewport: viewport.New(80, 10)}
	m.dirty = true
	m.renderMessages()

	content = m.viewport.View()
	if strings.TrimSpace(content) == "" {
		t.Fatalf("expected non-empty English welcome viewport, got empty")
	}
	if !strings.Contains(content, "agentforge") && !strings.Contains(content, "Welcome") {
		t.Fatalf("expected English welcome content to mention agentforge or Welcome, got %q", content)
	}
}
