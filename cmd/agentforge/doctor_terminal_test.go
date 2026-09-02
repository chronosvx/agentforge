package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestTerminalTrueColor(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	if !terminalTrueColor() {
		t.Fatal("terminalTrueColor() = false for COLORTERM=truecolor")
	}

	t.Setenv("COLORTERM", "24bit")
	if !terminalTrueColor() {
		t.Fatal("terminalTrueColor() = false for COLORTERM=24bit")
	}

	t.Setenv("COLORTERM", "")
	if terminalTrueColor() {
		t.Fatal("terminalTrueColor() = true when COLORTERM is empty")
	}
}

func TestTerminalNeedsANSISuggestion(t *testing.T) {
	t.Setenv("TERM", "")
	if !terminalNeedsANSISuggestion() {
		t.Fatal("terminalNeedsANSISuggestion() = false for empty TERM")
	}

	t.Setenv("TERM", "dumb")
	if !terminalNeedsANSISuggestion() {
		t.Fatal("terminalNeedsANSISuggestion() = false for TERM=dumb")
	}

	t.Setenv("TERM", "xterm-256color")
	if terminalNeedsANSISuggestion() {
		t.Fatal("terminalNeedsANSISuggestion() = true for TERM=xterm-256color")
	}
}

func TestLocaleNeedsUTF8Suggestion(t *testing.T) {
	t.Setenv("LANG", "")
	if !localeNeedsUTF8Suggestion() {
		t.Fatal("localeNeedsUTF8Suggestion() = false for empty LANG")
	}

	t.Setenv("LANG", "C")
	if !localeNeedsUTF8Suggestion() {
		t.Fatal("localeNeedsUTF8Suggestion() = false for LANG=C")
	}

	t.Setenv("LANG", "en_US.UTF-8")
	if localeNeedsUTF8Suggestion() {
		t.Fatal("localeNeedsUTF8Suggestion() = true for UTF-8 locale")
	}
}

func TestReportTerminalOutputContainsHints(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("COLORTERM", "")
	t.Setenv("LANG", "C")

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	var hints []doctorHint
	reportTerminal(&hints)
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	text := string(out)
	for _, want := range []string{
		"Terminal capability",
		"TERM",
		"COLORTERM",
		"LANG",
		"建议使用支持 ANSI/真彩色的终端",
		"建议设置 UTF-8 locale",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reportTerminal output missing %q:\n%s", want, text)
		}
	}
	if len(hints) != 2 {
		t.Fatalf("len(hints) = %d, want 2 terminal hints", len(hints))
	}
}
