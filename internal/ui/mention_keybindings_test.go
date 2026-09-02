package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestDefaultKeybindings(t *testing.T) {
	m := Model{}
	if got := m.key("help"); got != "ctrl+/" {
		t.Fatalf("help key = %q, want ctrl+/", got)
	}
	if got := m.key("palette"); got != "ctrl+p" {
		t.Fatalf("palette key = %q, want ctrl+p", got)
	}
	if got := m.key("history"); got != "ctrl+r" {
		t.Fatalf("history key = %q, want ctrl+r", got)
	}
}

func TestLoadKeybindingsFromFile(t *testing.T) {
	dir := t.TempDir()
	content := `{"palette":"ctrl+e","history":"ctrl+u"}`
	if err := os.WriteFile(filepath.Join(dir, "keybindings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadKeybindings(dir)
	if got["help"] != "ctrl+/" {
		t.Fatalf("help should stay default, got %q", got["help"])
	}
	if got["palette"] != "ctrl+e" {
		t.Fatalf("palette = %q, want ctrl+e", got["palette"])
	}
	if got["history"] != "ctrl+u" {
		t.Fatalf("history = %q, want ctrl+u", got["history"])
	}
}

func TestLoadKeybindingsMissingFileReturnsDefaults(t *testing.T) {
	got := loadKeybindings(t.TempDir())
	if got["help"] != "ctrl+/" || got["palette"] != "ctrl+p" || got["history"] != "ctrl+r" {
		t.Fatalf("expected defaults, got %#v", got)
	}
}

func TestIsKeySupportsDefaultHelpAliases(t *testing.T) {
	m := Model{}
	for _, key := range []string{"ctrl+/", "ctrl+_", "ctrl+?"} {
		if !m.isKey("help", key) {
			t.Fatalf("expected %q to match default help key", key)
		}
	}
	if m.isKey("history", "ctrl+?") {
		t.Fatal("ctrl+? should not alias history")
	}
}

func TestIsKeyUsesCustomHelp(t *testing.T) {
	m := Model{keybindings: map[string]string{"help": "ctrl+h"}}
	if !m.isKey("help", "ctrl+h") {
		t.Fatal("expected custom help key to match")
	}
	for _, key := range []string{"ctrl+/", "ctrl+_", "ctrl+?"} {
		if m.isKey("help", key) {
			t.Fatalf("custom help should not match %q", key)
		}
	}
}

func TestLoadKeybindingsIgnoresEmptyOverrides(t *testing.T) {
	dir := t.TempDir()
	content := `{"help":"","palette":"ctrl+h"}`
	if err := os.WriteFile(filepath.Join(dir, "keybindings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadKeybindings(dir)
	if got["help"] != "ctrl+/" {
		t.Fatalf("empty help override should be ignored, got %q", got["help"])
	}
	if got["palette"] != "ctrl+h" {
		t.Fatalf("palette = %q, want ctrl+h", got["palette"])
	}
}

func TestMentionCandidatesSkipsIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	dirs := []string{".git", "node_modules", "dist", "build", "vendor", "src"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		".git/config":         "x",
		"node_modules/pkg.js": "x",
		"dist/bundle.js":      "x",
		"build/app":           "x",
		"vendor/dep.go":       "x",
		"src/app.go":          "x",
		"README.md":           "x",
		".DS_Store":           "x",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := &Model{cfg: &config.Config{Workspace: root}}
	got := m.mentionCandidates("")
	joined := strings.Join(got, "\n")
	for _, want := range []string{"src/app.go", "README.md"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("mentionCandidates missing %q:\n%s", want, joined)
		}
	}
	for _, banned := range []string{".git/config", "node_modules/pkg.js", "dist/bundle.js", "build/app", "vendor/dep.go", ".DS_Store"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("mentionCandidates should not include %q:\n%s", banned, joined)
		}
	}
}

func TestMentionCandidatesFiltersByQuery(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.go", "model.go", "README.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := &Model{cfg: &config.Config{Workspace: root}}
	got := m.mentionCandidates("model")
	if len(got) != 1 || got[0] != "model.go" {
		t.Fatalf("expected only model.go, got %#v", got)
	}
}

func TestMentionSyncOpenFilterAndAccept(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"file.txt", "folder.go", "README.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	m := Model{cfg: &config.Config{Workspace: root}, input: textarea.New()}
	m.input.SetValue("read @file")
	m.syncMentionFromInput()

	if !m.mentionOpen {
		t.Fatal("expected mention to open after typing @")
	}
	if m.mentionQuery != "file" {
		t.Fatalf("mention query = %q, want file", m.mentionQuery)
	}
	if len(m.mentionResults) != 1 || m.mentionResults[0] != "file.txt" {
		t.Fatalf("expected file.txt, got %#v", m.mentionResults)
	}

	if !m.handleMentionKey("enter") {
		t.Fatal("expected enter to be consumed")
	}
	if m.mentionOpen {
		t.Fatal("expected mention to close after accept")
	}
	if got := m.input.Value(); got != "read @file.txt" {
		t.Fatalf("input after accept = %q, want %q", got, "read @file.txt")
	}
}

func TestMentionOpensOnTypedAtAndFilters(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"file.txt", "model.go", "README.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	input := textarea.New()
	input.Focus()
	m := Model{cfg: &config.Config{Workspace: root}, input: input}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	updated := res.(Model)
	if !updated.mentionOpen {
		t.Fatal("expected @ to open mention")
	}

	res, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	updated = res.(Model)
	if !updated.mentionOpen {
		t.Fatal("expected mention to stay open while typing filter")
	}
	if updated.mentionQuery != "f" {
		t.Fatalf("mention query = %q, want f", updated.mentionQuery)
	}
	if len(updated.mentionResults) != 1 || updated.mentionResults[0] != "file.txt" {
		t.Fatalf("expected file.txt after filter f, got %#v", updated.mentionResults)
	}
}

func TestMentionCloseKeys(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"esc", "q"} {
		m := Model{cfg: &config.Config{Workspace: root}, input: textarea.New()}
		m.input.SetValue("@")
		m.syncMentionFromInput()
		if !m.mentionOpen {
			t.Fatalf("expected mention to be open for %q test", key)
		}
		if !m.handleMentionKey(key) {
			t.Fatalf("expected %q to be consumed", key)
		}
		if m.mentionOpen {
			t.Fatalf("expected %q to close mention", key)
		}
	}
}

func TestRenderMentionIncludesI18nTexts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Model{cfg: &config.Config{Workspace: root}, input: textarea.New()}
	m.input.SetValue("@")
	m.syncMentionFromInput()

	view := m.renderMention()
	for _, want := range []string{i18n.T("mention.title"), "a.go", i18n.T("mention.hint")} {
		if !strings.Contains(view, want) {
			t.Fatalf("renderMention missing %q:\n%s", want, view)
		}
	}
}
