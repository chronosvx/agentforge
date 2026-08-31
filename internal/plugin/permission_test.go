package plugin

import (
	"errors"
	"testing"
)

func TestCheckPermissionAllows(t *testing.T) {
	perm := Permissions{
		AllowNetwork:     true,
		AllowExec:        true,
		AllowFileWrite:   true,
		AllowReadAnyPath: true,
	}
	for _, op := range []string{"network", "net", "exec", "execute", "file_write", "write", "read_any_path", "read"} {
		if err := CheckPermission(perm, op); err != nil {
			t.Fatalf("CheckPermission(%q) should pass: %v", op, err)
		}
	}
}

func TestCheckPermissionDenies(t *testing.T) {
	cases := []struct {
		perm Permissions
		op   string
	}{
		{Permissions{}, "network"},
		{Permissions{AllowNetwork: true}, "exec"},
		{Permissions{AllowExec: true}, "file_write"},
		{Permissions{AllowFileWrite: true}, "read_any_path"},
		{Permissions{AllowReadAnyPath: true}, "write"},
	}
	for _, tc := range cases {
		err := CheckPermission(tc.perm, tc.op)
		if err == nil {
			t.Fatalf("CheckPermission(%#v, %q) should fail", tc.perm, tc.op)
		}
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied for %q, got %v", tc.op, err)
		}
	}
}

func TestCheckPermissionUnknownOp(t *testing.T) {
	err := CheckPermission(Permissions{}, "unknown")
	if err == nil {
		t.Fatal("expected error for unknown operation")
	}
	if errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("unknown op should not be a permission denial, got %v", err)
	}
}
