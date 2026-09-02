package main

import (
	"context"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/client"
)

func TestExtractHeadlessOptionsDefaultFormat(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless", "--prompt", "hello",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "text" {
		t.Fatalf("format = %q, want %q", format, "text")
	}
	if len(rest) != 0 {
		t.Fatalf("rest = %v, want empty", rest)
	}
}

func TestExtractHeadlessOptionsFormatSpaceForm(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless", "--format", "json", "--prompt", "hello", "--mock",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "json" {
		t.Fatalf("format = %q, want %q", format, "json")
	}
	want := []string{"--mock"}
	if strings.Join(rest, " ") != strings.Join(want, " ") {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

func TestExtractHeadlessOptionsFormatEqualsForm(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless=true", "--format=json", "--prompt=hello", "--provider", "deepseek",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "json" {
		t.Fatalf("format = %q, want %q", format, "json")
	}
	if len(rest) != 2 || rest[0] != "--provider" || rest[1] != "deepseek" {
		t.Fatalf("unexpected rest: %v", rest)
	}
}

func TestExtractHeadlessOptionsFormatText(t *testing.T) {
	_, _, format, _, err := extractHeadlessOptions([]string{
		"--headless", "--format", "text", "--prompt", "hello",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want %q", format, "text")
	}
}

func TestExtractHeadlessOptionsFormatStreamJSONSpaceForm(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless", "--format", "stream-json", "--prompt", "hello", "--mock",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "stream-json" {
		t.Fatalf("format = %q, want %q", format, "stream-json")
	}
	want := []string{"--mock"}
	if strings.Join(rest, " ") != strings.Join(want, " ") {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

func TestExtractHeadlessOptionsFormatStreamJSONEqualsForm(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless=true", "--format=stream-json", "--prompt=hello", "--provider", "deepseek",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "stream-json" {
		t.Fatalf("format = %q, want %q", format, "stream-json")
	}
	if len(rest) != 2 || rest[0] != "--provider" || rest[1] != "deepseek" {
		t.Fatalf("unexpected rest: %v", rest)
	}
}

func TestWaitForHeadlessResultStreamEmitsDeltas(t *testing.T) {
	events := make(chan client.Event, 4)
	stateWithContent := func(content string) map[string]any {
		return map[string]any{
			"messages": []any{
				map[string]any{"role": "assistant", "content": content},
			},
		}
	}

	events <- client.Event{Operation: "event", State: stateWithContent("Hel")}
	events <- client.Event{Operation: "event", State: stateWithContent("Hello")}
	events <- client.Event{Operation: "event", State: stateWithContent("Hello")}
	events <- client.Event{Operation: "done", State: stateWithContent("Hello")}
	close(events)

	var deltas []string
	content, err := waitForHeadlessResultStream(context.Background(), events, "ch-1", func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatalf("waitForHeadlessResultStream: %v", err)
	}
	if content != "Hello" {
		t.Fatalf("content = %q, want %q", content, "Hello")
	}
	if len(deltas) != 2 || deltas[0] != "Hel" || deltas[1] != "Hello" {
		t.Fatalf("deltas = %v, want [Hel Hello]", deltas)
	}
}

func TestExtractHeadlessOptionsInvalidFormat(t *testing.T) {
	for _, args := range [][]string{
		{"--headless", "--format", "yaml", "--prompt", "hello"},
		{"--headless", "--format=yaml", "--prompt", "hello"},
	} {
		if _, _, _, _, err := extractHeadlessOptions(args); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

func TestExtractHeadlessOptionsMissingFormatValue(t *testing.T) {
	if _, _, _, _, err := extractHeadlessOptions([]string{"--headless", "--format"}); err == nil {
		t.Fatal("expected error for missing --format value")
	}
}
