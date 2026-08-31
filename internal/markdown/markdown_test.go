package markdown

import (
	"strings"
	"testing"
)

func TestRenderHeading(t *testing.T) {
	got := Render("# Hello")
	if !strings.Contains(got, "Hello") {
		t.Fatalf("heading content missing: %q", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("heading should contain ANSI escape: %q", got)
	}
	if strings.Contains(got, "#") {
		t.Fatalf("heading marker should be rendered away: %q", got)
	}

	gotH2 := Render("## Sub")
	if !strings.Contains(gotH2, "Sub") || !strings.Contains(gotH2, "\x1b[") {
		t.Fatalf("h2 rendering failed: %q", gotH2)
	}
}

func TestRenderInlineCode(t *testing.T) {
	got := Render("run `go test` now")
	if !strings.Contains(got, "go test") {
		t.Fatalf("inline code content missing: %q", got)
	}
	if !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("inline code should use reverse video: %q", got)
	}
	if strings.Contains(got, "`") {
		t.Fatalf("inline code backticks should be removed: %q", got)
	}
}

func TestRenderCodeBlock(t *testing.T) {
	input := "```go\npackage main\n\nfunc main() {}\n```"
	got := Render(input)
	if !strings.Contains(got, "package main") {
		t.Fatalf("code block content missing: %q", got)
	}
	if !strings.Contains(got, "func main() {}") {
		t.Fatalf("code block content missing: %q", got)
	}
	if !strings.Contains(got, "│") || !strings.Contains(got, "┌") || !strings.Contains(got, "└") {
		t.Fatalf("code block border missing: %q", got)
	}
	if !strings.Contains(got, "1 │") {
		t.Fatalf("code block line number missing: %q", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("code fences should not appear in rendered output: %q", got)
	}
}

func TestRenderListAndQuote(t *testing.T) {
	got := Render("- item\n> quote")
	if !strings.Contains(got, "item") || !strings.Contains(got, "quote") {
		t.Fatalf("list/quote content missing: %q", got)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("list/quote should contain ANSI colors: %q", got)
	}
}

func TestSanitizeControlChars(t *testing.T) {
	in := "hello\x1b[31mred\x00world\x07\nnext\tend"
	got := Sanitize(in)
	for _, r := range got {
		if r == '\x1b' || r == 0 || r == 0x07 || r == 0x7f {
			t.Fatalf("control character not removed: %q", got)
		}
	}
	if !strings.Contains(got, "\n") || !strings.Contains(got, "\t") {
		t.Fatalf("newline/tab should be preserved: %q", got)
	}
	if strings.Contains(got, "\x1b[31m") {
		t.Fatalf("ANSI escape not removed: %q", got)
	}
}

func TestRenderSanitizesInput(t *testing.T) {
	in := "# Evil\x1b]0;own\x07"
	got := Render(in)
	if strings.Contains(got, "\x1b]0;own") {
		t.Fatalf("render should sanitize terminal injection: %q", got)
	}
	if !strings.Contains(got, "Evil") {
		t.Fatalf("safe text missing after sanitize: %q", got)
	}
}

func TestRenderEmptyText(t *testing.T) {
	if got := Render(""); got != "" {
		t.Fatalf("empty render should be empty, got %q", got)
	}
	if got := Sanitize(""); got != "" {
		t.Fatalf("empty sanitize should be empty, got %q", got)
	}
}
