package collab_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/collab"
)

func TestCollabStoreAddAndListComments(t *testing.T) {
	store, err := collab.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	first := collab.Comment{
		ID:        "c1",
		SessionID: "s1",
		Author:    "alice",
		Content:   "LGTM",
		CreatedAt: time.Unix(100, 0).UTC(),
	}
	second := collab.Comment{
		ID:        "c2",
		SessionID: "s1",
		Author:    "bob",
		Content:   "please add tests",
		CreatedAt: time.Unix(101, 0).UTC(),
	}
	if err := store.AddComment(first); err != nil {
		t.Fatalf("add first comment: %v", err)
	}
	if err := store.AddComment(second); err != nil {
		t.Fatalf("add second comment: %v", err)
	}

	got, err := store.ListComments("s1")
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d comments, want 2", len(got))
	}
	if got[0].ID != first.ID || got[0].Author != first.Author || got[0].Content != first.Content {
		t.Fatalf("first comment = %+v, want %+v", got[0], first)
	}
	if got[1].ID != second.ID || got[1].Author != second.Author || got[1].Content != second.Content {
		t.Fatalf("second comment = %+v, want %+v", got[1], second)
	}
	if !got[0].CreatedAt.Equal(first.CreatedAt) || !got[1].CreatedAt.Equal(second.CreatedAt) {
		t.Fatalf("comment timestamps not preserved: %+v", got)
	}

	// 不同会话之间互不影响。
	other, err := store.ListComments("s2")
	if err != nil {
		t.Fatalf("list other comments: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("got %d comments for other session, want 0", len(other))
	}
}

func TestCollabStoreAddAndListApprovals(t *testing.T) {
	store, err := collab.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	first := collab.Approval{
		ID:        "a1",
		SessionID: "s1",
		ActionID:  "act-1",
		Approver:  "alice",
		Decision:  "approved",
		CreatedAt: time.Unix(200, 0).UTC(),
	}
	second := collab.Approval{
		ID:        "a2",
		SessionID: "s1",
		ActionID:  "act-2",
		Approver:  "bob",
		Decision:  "rejected",
		CreatedAt: time.Unix(201, 0).UTC(),
	}
	if err := store.AddApproval(first); err != nil {
		t.Fatalf("add first approval: %v", err)
	}
	if err := store.AddApproval(second); err != nil {
		t.Fatalf("add second approval: %v", err)
	}

	got, err := store.ListApprovals("s1")
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d approvals, want 2", len(got))
	}
	if got[0].ID != first.ID || got[0].ActionID != first.ActionID || got[0].Decision != first.Decision {
		t.Fatalf("first approval = %+v, want %+v", got[0], first)
	}
	if got[1].ID != second.ID || got[1].ActionID != second.ActionID || got[1].Decision != second.Decision {
		t.Fatalf("second approval = %+v, want %+v", got[1], second)
	}
}

func TestCollabStoreReplayReturnsSortedBySeq(t *testing.T) {
	store, err := collab.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	events := []collab.ReplayEvent{
		{Seq: 3, Type: "apply_diff", Data: map[string]any{"path": "internal/collab/collab.go"}, CreatedAt: time.Unix(303, 0).UTC()},
		{Seq: 1, Type: "plan", Data: map[string]any{"actions": 2}, CreatedAt: time.Unix(301, 0).UTC()},
		{Seq: 2, Type: "approve", Data: map[string]any{"actionId": "act-1"}, CreatedAt: time.Unix(302, 0).UTC()},
	}
	for _, event := range events {
		if err := store.AddReplayEvent("s1", event); err != nil {
			t.Fatalf("add replay event %d: %v", event.Seq, err)
		}
	}

	got, err := store.Replay("s1")
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d replay events, want 3", len(got))
	}
	wantSeq := []int{1, 2, 3}
	wantType := []string{"plan", "approve", "apply_diff"}
	for i, event := range got {
		if event.Seq != wantSeq[i] {
			t.Fatalf("event %d seq = %d, want %d", i, event.Seq, wantSeq[i])
		}
		if event.Type != wantType[i] {
			t.Fatalf("event %d type = %q, want %q", i, event.Type, wantType[i])
		}
	}
}

func TestCollabStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	store, err := collab.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.AddComment(collab.Comment{ID: "c1", SessionID: "s1", Author: "alice", Content: "keep"}); err != nil {
		t.Fatalf("add comment: %v", err)
	}
	if err := store.AddApproval(collab.Approval{ID: "a1", SessionID: "s1", ActionID: "act-1", Approver: "bob", Decision: "approved"}); err != nil {
		t.Fatalf("add approval: %v", err)
	}
	if err := store.AddReplayEvent("s1", collab.ReplayEvent{Seq: 2, Type: "done"}); err != nil {
		t.Fatalf("add replay event: %v", err)
	}

	// 新实例从同一 data-dir 读取，应能看到之前写入的数据。
	again, err := collab.NewStore(dir)
	if err != nil {
		t.Fatalf("new store again: %v", err)
	}

	comments, err := again.ListComments("s1")
	if err != nil {
		t.Fatalf("list comments from second store: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != "c1" {
		t.Fatalf("unexpected comments from second store: %+v", comments)
	}

	approvals, err := again.ListApprovals("s1")
	if err != nil {
		t.Fatalf("list approvals from second store: %v", err)
	}
	if len(approvals) != 1 || approvals[0].ID != "a1" {
		t.Fatalf("unexpected approvals from second store: %+v", approvals)
	}

	events, err := again.Replay("s1")
	if err != nil {
		t.Fatalf("replay from second store: %v", err)
	}
	if len(events) != 1 || events[0].Seq != 2 || events[0].Type != "done" {
		t.Fatalf("unexpected replay events from second store: %+v", events)
	}

	if _, err := filepath.Abs(filepath.Join(dir, "collab", "s1.json")); err != nil {
		t.Fatalf("collab file path should be valid: %v", err)
	}
}

func TestCollabStoreMissingSessionReturnsEmpty(t *testing.T) {
	store, err := collab.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	comments, err := store.ListComments("missing")
	if err != nil {
		t.Fatalf("list missing comments: %v", err)
	}
	if len(comments) != 0 {
		t.Fatalf("missing comments = %+v, want empty", comments)
	}

	approvals, err := store.ListApprovals("missing")
	if err != nil {
		t.Fatalf("list missing approvals: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("missing approvals = %+v, want empty", approvals)
	}

	events, err := store.Replay("missing")
	if err != nil {
		t.Fatalf("replay missing session: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("missing events = %+v, want empty", events)
	}
}

func TestCollabStoreRejectsInvalidSessionID(t *testing.T) {
	store, err := collab.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	if err := store.AddComment(collab.Comment{SessionID: "../escape"}); err == nil {
		t.Fatal("expected AddComment to reject invalid session id")
	}
	if err := store.AddApproval(collab.Approval{SessionID: "a/b"}); err == nil {
		t.Fatal("expected AddApproval to reject invalid session id")
	}
	if err := store.AddReplayEvent("../escape", collab.ReplayEvent{Seq: 1}); err == nil {
		t.Fatal("expected AddReplayEvent to reject invalid session id")
	}
	if _, err := store.ListComments(""); err == nil {
		t.Fatal("expected ListComments to reject empty session id")
	}
	if _, err := store.Replay(".."); err == nil {
		t.Fatal("expected Replay to reject invalid session id")
	}
}
