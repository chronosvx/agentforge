package ui

import (
	"strings"
	"testing"
)

func TestHandleDiffKeyAcceptAll(t *testing.T) {
	m := Model{
		diffs: []any{
			map[string]any{"file": "a.txt", "diff": "diff a"},
			map[string]any{"file": "b.txt", "diff": "diff b"},
		},
	}

	if !m.handleDiffKey("A") {
		t.Fatal("expected A to be consumed")
	}
	if len(m.diffDecisions) != 2 {
		t.Fatalf("expected decisions for 2 diffs, got %#v", m.diffDecisions)
	}
	for i := 0; i < len(m.diffs); i++ {
		if m.diffDecisions[i] != "accepted" {
			t.Fatalf("expected diff %d accepted, got %#v", i, m.diffDecisions)
		}
	}
	if m.status != "accepted all 2 diffs" {
		t.Fatalf("unexpected status: %q", m.status)
	}
}

func TestHandleDiffKeyRejectAll(t *testing.T) {
	m := Model{
		diffs: []any{
			map[string]any{"file": "a.txt", "diff": "diff a"},
			map[string]any{"file": "b.txt", "diff": "diff b"},
		},
	}

	if !m.handleDiffKey("R") {
		t.Fatal("expected R to be consumed")
	}
	if len(m.diffDecisions) != 2 {
		t.Fatalf("expected decisions for 2 diffs, got %#v", m.diffDecisions)
	}
	for i := 0; i < len(m.diffs); i++ {
		if m.diffDecisions[i] != "rejected" {
			t.Fatalf("expected diff %d rejected, got %#v", i, m.diffDecisions)
		}
	}
	if m.status != "rejected all 2 diffs" {
		t.Fatalf("unexpected status: %q", m.status)
	}
}

func TestRenderDiffPanelIncludesSummary(t *testing.T) {
	m := Model{
		diffs: []any{
			map[string]any{"file": "a.txt", "diff": "diff a"},
			map[string]any{"file": "b.txt", "diff": "diff b"},
			map[string]any{"file": "c.txt", "diff": "diff c"},
		},
		diffDecisions: map[int]string{0: "accepted", 1: "rejected"},
	}

	out := m.renderDiffPanel()
	if !strings.Contains(out, "accepted: 1, rejected: 1, pending: 1") {
		t.Fatalf("expected summary in render output, got:\n%s", out)
	}

	m.handleDiffKey("A")
	out = m.renderDiffPanel()
	if !strings.Contains(out, "accepted: 3, rejected: 0, pending: 0") {
		t.Fatalf("expected all-accepted summary, got:\n%s", out)
	}

	m.handleDiffKey("R")
	out = m.renderDiffPanel()
	if !strings.Contains(out, "accepted: 0, rejected: 3, pending: 0") {
		t.Fatalf("expected all-rejected summary, got:\n%s", out)
	}
}
