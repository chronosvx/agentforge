package plugin

import (
	"testing"
	"time"
)

func TestModerationStoreAddReportAndPendingStatus(t *testing.T) {
	store := NewModerationStore()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	if err := store.AddReport(Report{
		Plugin:    "evil",
		Reason:    "contains malware",
		Reporter:  "alice",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("add report: %v", err)
	}

	reports := store.Reports("evil")
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Plugin != "evil" || reports[0].Reason != "contains malware" || reports[0].Reporter != "alice" {
		t.Fatalf("unexpected report: %#v", reports[0])
	}
	if !reports[0].CreatedAt.Equal(now) {
		t.Fatalf("unexpected createdAt: %v", reports[0].CreatedAt)
	}
	if store.Status("evil") != StatusPending {
		t.Fatalf("expected pending status, got %q", store.Status("evil"))
	}
	if store.IsBlacklisted("evil") {
		t.Fatal("new report should not be blacklisted")
	}
}

func TestModerationStoreInvalidReport(t *testing.T) {
	store := NewModerationStore()
	if err := store.AddReport(Report{Plugin: "", Reason: "x", Reporter: "alice"}); err == nil {
		t.Fatal("expected error for empty plugin")
	}
	if err := store.AddReport(Report{Plugin: "evil", Reason: "", Reporter: "alice"}); err == nil {
		t.Fatal("expected error for empty reason")
	}
	if err := store.AddReport(Report{Plugin: "evil", Reason: "x", Reporter: ""}); err == nil {
		t.Fatal("expected error for empty reporter")
	}
	if len(store.Reports("")) != 0 {
		t.Fatal("invalid reports should not be stored")
	}
}

func TestModerationStoreReportsFiltersByPlugin(t *testing.T) {
	store := NewModerationStore()
	_ = store.AddReport(Report{Plugin: "alpha", Reason: "spam", Reporter: "alice"})
	_ = store.AddReport(Report{Plugin: "alpha", Reason: "abuse", Reporter: "bob"})
	_ = store.AddReport(Report{Plugin: "beta", Reason: "malware", Reporter: "carol"})

	if got := store.Reports("alpha"); len(got) != 2 {
		t.Fatalf("expected 2 alpha reports, got %d", len(got))
	}
	if got := store.Reports("beta"); len(got) != 1 {
		t.Fatalf("expected 1 beta report, got %d", len(got))
	}
	if got := store.Reports(""); len(got) != 3 {
		t.Fatalf("expected 3 total reports, got %d", len(got))
	}
}

func TestModerationStoreAuditFlow(t *testing.T) {
	store := NewModerationStore()
	_ = store.AddReport(Report{Plugin: "plugin-a", Reason: "malware", Reporter: "alice"})

	if store.Status("plugin-a") != StatusPending {
		t.Fatalf("expected pending, got %q", store.Status("plugin-a"))
	}

	if err := store.SetStatus("plugin-a", StatusApproved); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if store.Status("plugin-a") != StatusApproved {
		t.Fatalf("expected approved, got %q", store.Status("plugin-a"))
	}
	if store.IsBlacklisted("plugin-a") {
		t.Fatal("approved plugin should not be blacklisted")
	}

	if err := store.SetStatus("plugin-a", StatusRejected); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if store.Status("plugin-a") != StatusRejected {
		t.Fatalf("expected rejected, got %q", store.Status("plugin-a"))
	}

	if err := store.SetStatus("plugin-a", StatusBlacklisted); err != nil {
		t.Fatalf("blacklist: %v", err)
	}
	if !store.IsBlacklisted("plugin-a") {
		t.Fatal("expected blacklisted plugin")
	}

	if err := store.Unblacklist("plugin-a"); err != nil {
		t.Fatalf("unblacklist: %v", err)
	}
	if store.IsBlacklisted("plugin-a") {
		t.Fatal("expected plugin to be removed from blacklist")
	}
	if store.Status("plugin-a") != StatusApproved {
		t.Fatalf("expected approved after unblacklist, got %q", store.Status("plugin-a"))
	}
}

func TestModerationStoreSetStatusValidation(t *testing.T) {
	store := NewModerationStore()
	if err := store.SetStatus("", StatusApproved); err == nil {
		t.Fatal("expected error for empty plugin")
	}
	if err := store.SetStatus("plugin-a", ReviewStatus("bogus")); err == nil {
		t.Fatal("expected error for unknown status")
	}
}
