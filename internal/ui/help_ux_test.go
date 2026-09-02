package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestHelpLinesIncludesGlobalShortcuts(t *testing.T) {
	prevLang := i18n.CurrentLang()
	defer i18n.SetLang(prevLang)

	for _, lang := range []i18n.Lang{i18n.LangZH, i18n.LangEN} {
		i18n.SetLang(lang)

		lines := (&Model{}).helpLines()
		joined := strings.Join(lines, "\n")
		if !strings.Contains(joined, "Ctrl+P") {
			t.Fatalf("helpLines (%s) missing Ctrl+P: %q", lang, joined)
		}
		if !strings.Contains(joined, "Enter") {
			t.Fatalf("helpLines (%s) missing Enter: %q", lang, joined)
		}
	}
}
