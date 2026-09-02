package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestRenderStatusBarDefaultShortcuts(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)

	m := Model{}

	for _, lang := range []i18n.Lang{i18n.LangZH, i18n.LangEN} {
		i18n.SetLang(lang)

		got := m.renderStatusBar()
		if !strings.Contains(got, "Ctrl+P") {
			t.Fatalf("renderStatusBar (%s) missing Ctrl+P: %q", lang, got)
		}
		if !strings.Contains(got, "Enter") {
			t.Fatalf("renderStatusBar (%s) missing Enter: %q", lang, got)
		}
	}
}
