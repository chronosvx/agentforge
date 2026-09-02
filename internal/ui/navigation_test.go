package ui

import (
	"testing"

	"github.com/chronosvx/agentforge/internal/multiagent"
)

func TestHandleMultiKeyMovesCursorInRunningTasks(t *testing.T) {
	m := Model{
		multiRunning: true,
		multiTasks: []multiagent.Task{
			{ID: "task-0", Prompt: "first"},
			{ID: "task-1", Prompt: "second"},
			{ID: "task-2", Prompt: "third"},
		},
	}

	if !m.handleMultiKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("down") {
		t.Fatal("expected down to be consumed")
	}
	if m.multiCursor != 2 {
		t.Fatalf("expected cursor 2 after down, got %d", m.multiCursor)
	}

	// At the bottom boundary: the key is consumed but the cursor must not move.
	if !m.handleMultiKey("j") {
		t.Fatal("expected j at bottom to be consumed")
	}
	if m.multiCursor != 2 {
		t.Fatalf("expected cursor to stay at bottom 2, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("k") {
		t.Fatal("expected k to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor 1 after k, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("up") {
		t.Fatal("expected up to be consumed")
	}
	if m.multiCursor != 0 {
		t.Fatalf("expected cursor 0 after up, got %d", m.multiCursor)
	}

	// At the top boundary: the key is consumed but the cursor must not move.
	if !m.handleMultiKey("k") {
		t.Fatal("expected k at top to be consumed")
	}
	if m.multiCursor != 0 {
		t.Fatalf("expected cursor to stay at top 0, got %d", m.multiCursor)
	}
}

func TestHandleMultiKeyMovesCursorInPendingTasks(t *testing.T) {
	m := Model{
		multiPending: []multiagent.Task{
			{ID: "pending-0", Prompt: "first"},
			{ID: "pending-1", Prompt: "second"},
			{ID: "pending-2", Prompt: "third"},
		},
		multiCursor: 1,
	}

	if !m.handleMultiKey("up") {
		t.Fatal("expected up to be consumed")
	}
	if m.multiCursor != 0 {
		t.Fatalf("expected cursor 0 after up, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("down") {
		t.Fatal("expected down to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor 1 after down, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.multiCursor != 2 {
		t.Fatalf("expected cursor 2 after j, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("down") {
		t.Fatal("expected down at bottom to be consumed")
	}
	if m.multiCursor != 2 {
		t.Fatalf("expected cursor to stay at bottom 2, got %d", m.multiCursor)
	}

	if !m.handleMultiKey("k") {
		t.Fatal("expected k to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor 1 after k, got %d", m.multiCursor)
	}
}

func TestHandleMultiKeyNoTasksReturnsFalse(t *testing.T) {
	tests := []struct {
		name string
		m    Model
	}{
		{name: "empty model", m: Model{}},
		{name: "running with no tasks", m: Model{multiRunning: true}},
		{name: "empty pending slice", m: Model{multiPending: []multiagent.Task{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"j", "down", "k", "up"} {
				if tt.m.handleMultiKey(key) {
					t.Fatalf("handleMultiKey(%q) = true, want false", key)
				}
			}
		})
	}
}

func TestHandleMultiKeyUsesTasksWhenNotRunningAndNoPending(t *testing.T) {
	m := Model{
		multiTasks: []multiagent.Task{
			{ID: "done-0", Prompt: "finished first"},
			{ID: "done-1", Prompt: "finished second"},
		},
	}

	if !m.handleMultiKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor 1 in completed task list, got %d", m.multiCursor)
	}
}

func TestHandleMultiKeyPrefersRunningTasks(t *testing.T) {
	m := Model{
		multiRunning: true,
		multiTasks: []multiagent.Task{
			{ID: "running-0", Prompt: "run first"},
			{ID: "running-1", Prompt: "run second"},
		},
		multiPending: []multiagent.Task{
			{ID: "pending-0", Prompt: "pending"},
		},
	}

	if !m.handleMultiKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor to move in running tasks to 1, got %d", m.multiCursor)
	}
}

func TestHandleMultiKeyClampsOutOfRangeCursor(t *testing.T) {
	m := Model{
		multiRunning: true,
		multiTasks: []multiagent.Task{
			{ID: "task-0", Prompt: "first"},
			{ID: "task-1", Prompt: "second"},
		},
		multiCursor: 10,
	}

	if !m.handleMultiKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.multiCursor != 1 {
		t.Fatalf("expected cursor clamped to last index 1, got %d", m.multiCursor)
	}

	m.multiCursor = -5
	if !m.handleMultiKey("k") {
		t.Fatal("expected k to be consumed")
	}
	if m.multiCursor != 0 {
		t.Fatalf("expected cursor clamped to first index 0, got %d", m.multiCursor)
	}
}
