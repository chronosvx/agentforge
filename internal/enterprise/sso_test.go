package enterprise_test

import (
	"context"
	"errors"
	"testing"

	"github.com/chronosvx/agentforge/internal/enterprise"
)

func TestStaticProviderAuthenticate(t *testing.T) {
	p := enterprise.NewStaticProvider()

	alice := &enterprise.User{
		ID:    "u-alice",
		Name:  "Alice",
		Email: "alice@example.com",
		Roles: []enterprise.Role{enterprise.RoleAdmin},
	}
	if err := p.AddUser("token-alice", alice); err != nil {
		t.Fatalf("add user: %v", err)
	}

	got, err := p.Authenticate(context.Background(), "token-alice")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != alice.ID || got.Name != alice.Name || got.Email != alice.Email {
		t.Fatalf("got %+v, want %+v", got, alice)
	}
	if len(got.Roles) != 1 || got.Roles[0] != enterprise.RoleAdmin {
		t.Fatalf("got roles %v, want [admin]", got.Roles)
	}
}

func TestStaticProviderWithInitialUsers(t *testing.T) {
	users := map[string]*enterprise.User{
		"token-a": {ID: "u-a", Name: "Alice", Email: "a@example.com", Roles: []enterprise.Role{enterprise.RoleMember}},
		"token-b": {ID: "u-b", Name: "Bob", Email: "b@example.com", Roles: []enterprise.Role{enterprise.RoleGuest}},
	}
	p := enterprise.NewStaticProvider(users)

	got, err := p.Authenticate(context.Background(), "token-b")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if got.ID != "u-b" || got.Name != "Bob" {
		t.Fatalf("got %+v, want Bob", got)
	}
}

func TestStaticProviderReturnsCopy(t *testing.T) {
	p := enterprise.NewStaticProvider()
	user := &enterprise.User{ID: "u1", Name: "Before", Email: "u@example.com", Roles: []enterprise.Role{enterprise.RoleGuest}}
	if err := p.AddUser("token", user); err != nil {
		t.Fatalf("add user: %v", err)
	}

	got, err := p.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	got.Name = "Mutated"
	got.Roles[0] = enterprise.RoleAdmin

	again, err := p.Authenticate(context.Background(), "token")
	if err != nil {
		t.Fatalf("authenticate again: %v", err)
	}
	if again.Name != "Before" {
		t.Fatalf("provider should return a copy, got name %q", again.Name)
	}
	if again.Roles[0] != enterprise.RoleGuest {
		t.Fatalf("provider should return a copy, got roles %v", again.Roles)
	}
}

func TestStaticProviderUnknownToken(t *testing.T) {
	p := enterprise.NewStaticProvider()
	_, err := p.Authenticate(context.Background(), "missing")
	if !errors.Is(err, enterprise.ErrIdentityNotFound) {
		t.Fatalf("got error %v, want ErrIdentityNotFound", err)
	}
}

func TestStaticProviderAddUserValidation(t *testing.T) {
	p := enterprise.NewStaticProvider()
	if err := p.AddUser("", &enterprise.User{ID: "u1"}); err == nil {
		t.Fatalf("expected error for empty token")
	}
	if err := p.AddUser("t", nil); err == nil {
		t.Fatalf("expected error for nil user")
	}
	if err := p.AddUser("t", &enterprise.User{Name: "No ID"}); err == nil {
		t.Fatalf("expected error for empty user id")
	}
}

func TestStaticProviderContextCancel(t *testing.T) {
	p := enterprise.NewStaticProvider()
	if err := p.AddUser("t", &enterprise.User{ID: "u1"}); err != nil {
		t.Fatalf("add user: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Authenticate(ctx, "t"); err == nil {
		t.Fatalf("expected error for cancelled context")
	}
}

func TestUserHasRoleAndAddRole(t *testing.T) {
	u := &enterprise.User{ID: "u1", Roles: []enterprise.Role{enterprise.RoleMember}}
	if !u.HasRole(enterprise.RoleMember) {
		t.Fatalf("user should have member role")
	}
	if u.HasRole(enterprise.RoleAdmin) {
		t.Fatalf("user should not have admin role")
	}

	u.AddRole(enterprise.RoleAdmin)
	if !u.HasRole(enterprise.RoleAdmin) {
		t.Fatalf("user should have admin role after AddRole")
	}
	if len(u.Roles) != 2 {
		t.Fatalf("roles length = %d, want 2", len(u.Roles))
	}

	u.AddRole(enterprise.RoleAdmin)
	if len(u.Roles) != 2 {
		t.Fatalf("AddRole should not duplicate existing role, got %d roles", len(u.Roles))
	}
}
