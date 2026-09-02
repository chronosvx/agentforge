package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func TestAppendHistoryTrimsSkipsEmptyAndDeduplicates(t *testing.T) {
	m := Model{}
	m.appendHistory("  hello  ")
	m.appendHistory("hello")
	m.appendHistory("")
	m.appendHistory("world")

	if len(m.history) != 2 {
		t.Fatalf("expected 2 history entries, got %d: %#v", len(m.history), m.history)
	}
	if m.history[0] != "hello" || m.history[1] != "world" {
		t.Fatalf("unexpected history: %#v", m.history)
	}

	// Non-consecutive duplicates are still kept.
	m.appendHistory("hello")
	if len(m.history) != 3 {
		t.Fatalf("expected non-consecutive duplicate to be kept, got %d: %#v", len(m.history), m.history)
	}
}

func TestAppendHistoryKeepsAtMost200(t *testing.T) {
	m := Model{}
	for i := 0; i < 250; i++ {
		m.appendHistory(fmt.Sprintf("msg-%03d", i))
	}

	if len(m.history) != 200 {
		t.Fatalf("expected 200 history entries, got %d", len(m.history))
	}
	if m.history[0] != "msg-050" {
		t.Fatalf("expected oldest kept entry msg-050, got %q", m.history[0])
	}
	if m.history[len(m.history)-1] != "msg-249" {
		t.Fatalf("expected newest entry msg-249, got %q", m.history[len(m.history)-1])
	}
}

func TestOpenCloseHistorySearch(t *testing.T) {
	m := Model{history: []string{"hello", "world", "history search"}}
	m.openHistorySearch()

	if !m.historySearchOpen {
		t.Fatal("expected history search to be open")
	}
	if m.historySearchQuery != "" {
		t.Fatalf("expected empty query, got %q", m.historySearchQuery)
	}
	if m.historySearchCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.historySearchCursor)
	}
	if len(m.historySearchResults) != len(m.history) {
		t.Fatalf("expected all history entries, got %d", len(m.historySearchResults))
	}
	for i, want := range m.history {
		if m.historySearchResults[i] != want {
			t.Fatalf("result %d = %q, want %q", i, m.historySearchResults[i], want)
		}
	}

	m.historySearchCursor = 2
	m.historySearchQuery = "x"
	m.closeHistorySearch()
	if m.historySearchOpen {
		t.Fatal("expected history search to be closed")
	}
	if m.historySearchQuery != "" || m.historySearchCursor != 0 || m.historySearchResults != nil {
		t.Fatalf("expected search state cleared, got query=%q cursor=%d results=%#v", m.historySearchQuery, m.historySearchCursor, m.historySearchResults)
	}
}

func TestHistoryFiltering(t *testing.T) {
	m := Model{history: []string{"Hello World", "goodbye", "help", "ALPHA"}}
	m.openHistorySearch()

	if !m.handleHistoryKey("h") {
		t.Fatal("expected printable h to be consumed")
	}
	if m.historySearchQuery != "h" {
		t.Fatalf("expected query h, got %q", m.historySearchQuery)
	}
	if len(m.historySearchResults) != 3 {
		t.Fatalf("expected 3 matches for h, got %d: %#v", len(m.historySearchResults), m.historySearchResults)
	}

	if !m.handleHistoryKey("e") {
		t.Fatal("expected printable e to be consumed")
	}
	if len(m.historySearchResults) != 2 {
		t.Fatalf("expected 2 matches for he, got %d: %#v", len(m.historySearchResults), m.historySearchResults)
	}
	for _, item := range m.historySearchResults {
		if !strings.Contains(strings.ToLower(item), "he") {
			t.Fatalf("unexpected non-matching result %q", item)
		}
	}

	// Backspace removes the last query rune and re-filters.
	if !m.handleHistoryKey("backspace") {
		t.Fatal("expected backspace to be consumed")
	}
	if m.historySearchQuery != "h" {
		t.Fatalf("expected query h after backspace, got %q", m.historySearchQuery)
	}
	if len(m.historySearchResults) != 3 {
		t.Fatalf("expected 3 matches after backspace, got %d: %#v", len(m.historySearchResults), m.historySearchResults)
	}
}

func TestHistoryFilteringCaseInsensitive(t *testing.T) {
	m := Model{history: []string{"Hello World", "goodbye", "help", "ALPHA"}}
	m.openHistorySearch()

	if !m.handleHistoryKey("a") {
		t.Fatal("expected printable a to be consumed")
	}
	if len(m.historySearchResults) != 1 || m.historySearchResults[0] != "ALPHA" {
		t.Fatalf("expected case-insensitive match ALPHA, got %#v", m.historySearchResults)
	}
}

func TestHistoryNavigation(t *testing.T) {
	m := Model{history: []string{"a", "b", "c", "d"}}
	m.openHistorySearch()

	cases := []struct {
		key  string
		want int
	}{
		{"down", 1},
		{"j", 2},
		{"ctrl+n", 3},
		{"up", 2},
		{"k", 1},
		{"home", 0},
		{"end", 3},
	}
	for _, tc := range cases {
		if !m.handleHistoryKey(tc.key) {
			t.Fatalf("expected key %q to be consumed", tc.key)
		}
		if m.historySearchCursor != tc.want {
			t.Fatalf("after %q expected cursor %d, got %d", tc.key, tc.want, m.historySearchCursor)
		}
	}

	// Cursor does not move past boundaries.
	m.handleHistoryKey("home")
	m.handleHistoryKey("up")
	if m.historySearchCursor != 0 {
		t.Fatalf("expected cursor to stay at 0, got %d", m.historySearchCursor)
	}
	m.handleHistoryKey("end")
	m.handleHistoryKey("down")
	if m.historySearchCursor != len(m.history)-1 {
		t.Fatalf("expected cursor to stay at last, got %d", m.historySearchCursor)
	}
}

func TestHistoryEnterFillsInputAndCloses(t *testing.T) {
	input := textarea.New()
	m := Model{history: []string{"hello", "world"}, input: input}
	m.openHistorySearch()

	m.handleHistoryKey("down") // select "world"
	if !m.handleHistoryKey("enter") {
		t.Fatal("expected enter to be consumed")
	}

	if m.historySearchOpen {
		t.Fatal("expected history search to close after enter")
	}
	if got := m.input.Value(); got != "world" {
		t.Fatalf("expected input filled with world, got %q", got)
	}
}

func TestHistoryCloseKeys(t *testing.T) {
	for _, key := range []string{"esc", "q"} {
		m := Model{history: []string{"hello"}}
		m.openHistorySearch()
		if !m.handleHistoryKey(key) {
			t.Fatalf("expected %q to close history search", key)
		}
		if m.historySearchOpen {
			t.Fatalf("expected %q to close history search", key)
		}
	}
}

func TestHistoryUnrecognizedKeyReturnsFalse(t *testing.T) {
	m := Model{history: []string{"hello"}}
	m.openHistorySearch()
	if m.handleHistoryKey("ctrl+x") {
		t.Fatal("expected unrecognized key to return false")
	}
	if !m.historySearchOpen {
		t.Fatal("unrecognized key should not close history search")
	}
}

func TestRenderHistorySearchIncludesTitleItemsAndHint(t *testing.T) {
	m := Model{history: []string{"hello", "world"}}
	m.openHistorySearch()
	got := m.renderHistorySearch()

	for _, want := range []string{i18n.T("history.title"), "hello", "world", i18n.T("history.hint")} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderHistorySearch missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "> hello") {
		t.Fatalf("expected current item marker > hello:\n%s", got)
	}
}
