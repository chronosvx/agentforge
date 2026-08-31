package enterprise_test

import (
	"testing"

	"github.com/chronosvx/agentforge/internal/enterprise"
)

func TestDefaultPolicyRoles(t *testing.T) {
	p := enterprise.DefaultPolicy()

	tests := []struct {
		name   string
		role   enterprise.Role
		action string
		want   bool
	}{
		{"admin can everything", enterprise.RoleAdmin, "anything.at.all", true},
		{"member can read session", enterprise.RoleMember, "session.read", true},
		{"member can write session", enterprise.RoleMember, "session.write", true},
		{"member can approve", enterprise.RoleMember, "collab.approve", true},
		{"member cannot audit", enterprise.RoleMember, "audit.read", false},
		{"auditor can read audit", enterprise.RoleAuditor, "audit.read", true},
		{"auditor can read siem", enterprise.RoleAuditor, "siem.read", true},
		{"auditor cannot write session", enterprise.RoleAuditor, "session.write", false},
		{"guest can read session", enterprise.RoleGuest, "session.read", true},
		{"guest cannot write", enterprise.RoleGuest, "session.write", false},
		{"unknown role denied", enterprise.Role("nobody"), "session.read", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Can(tt.role, tt.action); got != tt.want {
				t.Fatalf("Can(%q, %q) = %v, want %v", tt.role, tt.action, got, tt.want)
			}
		})
	}
}

func TestPolicyAllowAndCustomActions(t *testing.T) {
	p := enterprise.NewPolicy()
	p.Allow(enterprise.RoleMember, "session.read", "session.write")

	if !p.Can(enterprise.RoleMember, "session.read") {
		t.Fatalf("member should be able to read session")
	}
	if !p.Can(enterprise.RoleMember, "session.write") {
		t.Fatalf("member should be able to write session")
	}
	if p.Can(enterprise.RoleMember, "admin.manage") {
		t.Fatalf("member should not be able to manage admin")
	}
	if p.Can(enterprise.RoleAdmin, "session.read") {
		t.Fatalf("empty admin policy should not grant access")
	}
}

func TestPolicyWildcard(t *testing.T) {
	p := enterprise.NewPolicy()
	p.Allow(enterprise.RoleAdmin, "*")

	if !p.Can(enterprise.RoleAdmin, "any.action") {
		t.Fatalf("wildcard should grant admin any action")
	}
	if p.Can(enterprise.RoleMember, "any.action") {
		t.Fatalf("member should not inherit admin wildcard")
	}
}

func TestRBACCheckUserRoles(t *testing.T) {
	rbac := enterprise.NewRBAC(enterprise.DefaultPolicy())

	admin := &enterprise.User{ID: "u1", Name: "Admin", Email: "admin@example.com", Roles: []enterprise.Role{enterprise.RoleAdmin}}
	if !rbac.Check(admin, "admin.manage") {
		t.Fatalf("admin should pass any action")
	}

	auditor := &enterprise.User{ID: "u2", Name: "Auditor", Email: "audit@example.com", Roles: []enterprise.Role{enterprise.RoleAuditor}}
	if !rbac.Check(auditor, "audit.read") {
		t.Fatalf("auditor should read audit")
	}
	if rbac.Check(auditor, "session.write") {
		t.Fatalf("auditor should not write session")
	}

	// 多角色任一命中即可。
	multi := &enterprise.User{ID: "u3", Name: "Multi", Email: "multi@example.com", Roles: []enterprise.Role{enterprise.RoleGuest, enterprise.RoleMember}}
	if !rbac.Check(multi, "session.write") {
		t.Fatalf("member role should allow session.write")
	}
	if rbac.Check(multi, "audit.read") {
		t.Fatalf("member+guest should not read audit")
	}
}

func TestRBACNilSafety(t *testing.T) {
	rbac := enterprise.NewRBAC(nil)
	if rbac.Check(nil, "session.read") {
		t.Fatalf("nil user should be denied")
	}
	if rbac.Check(&enterprise.User{ID: "u", Roles: nil}, "session.read") {
		t.Fatalf("user without roles should be denied")
	}
}

func TestParseRole(t *testing.T) {
	for _, role := range enterprise.KnownRoles() {
		got, err := enterprise.ParseRole(role.String())
		if err != nil {
			t.Fatalf("ParseRole(%q): %v", role, err)
		}
		if got != role {
			t.Fatalf("ParseRole(%q) = %q, want %q", role, got, role)
		}
	}

	if _, err := enterprise.ParseRole("superuser"); err == nil {
		t.Fatalf("expected error for unknown role")
	}
}
