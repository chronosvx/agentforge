package mcpperm

import (
	"testing"
	"time"
)

func TestPolicyExactRule(t *testing.T) {
	p := NewPolicy(ModeDeny)
	p.Set("files.read", ModeAuto)
	p.Set("files.write", ModeConfirm)

	if got := p.Mode("files.read"); got != ModeAuto {
		t.Errorf("Mode(files.read) = %q, want %q", got, ModeAuto)
	}
	if got := p.Mode("files.write"); got != ModeConfirm {
		t.Errorf("Mode(files.write) = %q, want %q", got, ModeConfirm)
	}
}

func TestPolicyPrefixRule(t *testing.T) {
	p := NewPolicy(ModeDeny)
	p.Set("group.*", ModeConfirm)
	p.Set("group.admin.*", ModeAuto)

	if got := p.Mode("group.users.list"); got != ModeConfirm {
		t.Errorf("Mode(group.users.list) = %q, want %q", got, ModeConfirm)
	}
	if got := p.Mode("group.admin.delete"); got != ModeAuto {
		t.Errorf("Mode(group.admin.delete) = %q, want %q", got, ModeAuto)
	}
	// Exact match must win over a prefix rule.
	p.Set("group.users.list", ModeAuto)
	if got := p.Mode("group.users.list"); got != ModeAuto {
		t.Errorf("exact Mode(group.users.list) = %q, want %q", got, ModeAuto)
	}
}

func TestPolicyDefault(t *testing.T) {
	p := NewPolicy(ModeAuto)
	p.Set("files.read", ModeConfirm)

	if got := p.Mode("db.query"); got != ModeAuto {
		t.Errorf("Mode(db.query) = %q, want %q", got, ModeAuto)
	}
	if got := p.Mode(""); got != ModeAuto {
		t.Errorf("Mode(empty) = %q, want %q", got, ModeAuto)
	}
}

func TestPolicyDeny(t *testing.T) {
	p := NewPolicy(ModeAuto)
	p.Set("secrets.read", ModeDeny)

	d := p.Decide("secrets.read")
	if d.Mode != ModeDeny {
		t.Errorf("Decision.Mode = %q, want %q", d.Mode, ModeDeny)
	}
	if d.Allowed {
		t.Error("Decision.Allowed = true, want false for deny")
	}
	if d.RequiresApproval {
		t.Error("Decision.RequiresApproval = true, want false for deny")
	}
}

func TestPolicyDecideModes(t *testing.T) {
	p := NewPolicy(ModeAuto)
	p.Set("confirm.tool", ModeConfirm)
	p.Set("deny.tool", ModeDeny)

	tests := []struct {
		tool             string
		wantMode         Mode
		wantAllowed      bool
		wantRequiresAppr bool
	}{
		{tool: "auto.tool", wantMode: ModeAuto, wantAllowed: true, wantRequiresAppr: false},
		{tool: "confirm.tool", wantMode: ModeConfirm, wantAllowed: true, wantRequiresAppr: true},
		{tool: "deny.tool", wantMode: ModeDeny, wantAllowed: false, wantRequiresAppr: false},
	}
	for _, tt := range tests {
		d := p.Decide(tt.tool)
		if d.Mode != tt.wantMode {
			t.Errorf("Decide(%q).Mode = %q, want %q", tt.tool, d.Mode, tt.wantMode)
		}
		if d.Allowed != tt.wantAllowed {
			t.Errorf("Decide(%q).Allowed = %v, want %v", tt.tool, d.Allowed, tt.wantAllowed)
		}
		if d.RequiresApproval != tt.wantRequiresAppr {
			t.Errorf("Decide(%q).RequiresApproval = %v, want %v", tt.tool, d.RequiresApproval, tt.wantRequiresAppr)
		}
	}
}

func TestPolicySetReplacesExistingRule(t *testing.T) {
	p := NewPolicy(ModeDeny)
	p.Set("files.read", ModeConfirm)
	p.Set("files.read", ModeAuto)

	if got := p.Mode("files.read"); got != ModeAuto {
		t.Errorf("Mode(files.read) after Set = %q, want %q", got, ModeAuto)
	}
	if len(p.Rules) != 1 {
		t.Errorf("len(p.Rules) = %d, want 1", len(p.Rules))
	}
}

func TestAuditRecordAndList(t *testing.T) {
	log := NewAuditLog()
	p := NewPolicy(ModeDeny)
	p.Set("files.read", ModeAuto)
	p.Set("admin.delete", ModeConfirm)

	before := time.Now()
	log.Record("files.read", p.Decide("files.read"))
	log.Record("admin.delete", p.Decide("admin.delete"))
	log.Record("secrets.read", p.Decide("secrets.read"))
	after := time.Now()

	entries := log.List()
	if len(entries) != 3 {
		t.Fatalf("len(List()) = %d, want 3", len(entries))
	}

	want := []struct {
		tool             string
		mode             Mode
		allowed          bool
		requiresApproval bool
	}{
		{tool: "files.read", mode: ModeAuto, allowed: true, requiresApproval: false},
		{tool: "admin.delete", mode: ModeConfirm, allowed: true, requiresApproval: true},
		{tool: "secrets.read", mode: ModeDeny, allowed: false, requiresApproval: false},
	}
	for i, w := range want {
		e := entries[i]
		if e.Tool != w.tool {
			t.Errorf("entry %d Tool = %q, want %q", i, e.Tool, w.tool)
		}
		if e.Decision.Mode != w.mode {
			t.Errorf("entry %d Decision.Mode = %q, want %q", i, e.Decision.Mode, w.mode)
		}
		if e.Decision.Allowed != w.allowed {
			t.Errorf("entry %d Decision.Allowed = %v, want %v", i, e.Decision.Allowed, w.allowed)
		}
		if e.Decision.RequiresApproval != w.requiresApproval {
			t.Errorf("entry %d Decision.RequiresApproval = %v, want %v", i, e.Decision.RequiresApproval, w.requiresApproval)
		}
		if e.At.Before(before) || e.At.After(after) {
			t.Errorf("entry %d At = %v, want within [%v, %v]", i, e.At, before, after)
		}
	}

	// List should return a copy, not the live slice.
	entries[0].Tool = "mutated"
	if got := log.List()[0].Tool; got != "files.read" {
		t.Errorf("List copy was mutated: got %q, want files.read", got)
	}
}

func TestAuditLogConcurrentRecord(t *testing.T) {
	log := NewAuditLog()
	const n = 50
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			log.Record("tool", Decision{Tool: "tool", Mode: ModeAuto, Allowed: true})
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if got := len(log.List()); got != n {
		t.Errorf("len(List()) = %d, want %d", got, n)
	}
}
