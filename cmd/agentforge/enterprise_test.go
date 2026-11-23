package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckExecRBAC(t *testing.T) {
	if err := checkExecRBAC("", ""); err != nil {
		t.Fatalf("empty identity should be allowed: %v", err)
	}
	if err := checkExecRBAC("alice", ""); err != nil {
		t.Fatalf("user without roles should be allowed by default: %v", err)
	}
	if err := checkExecRBAC("alice", "member"); err != nil {
		t.Fatalf("member should be allowed to exec: %v", err)
	}
	if err := checkExecRBAC("alice", "admin"); err != nil {
		t.Fatalf("admin should be allowed to exec: %v", err)
	}
	if err := checkExecRBAC("bob", "guest"); err == nil {
		t.Fatal("guest should be denied exec")
	}
	if err := checkExecRBAC("bob", "auditor"); err == nil {
		t.Fatal("auditor should be denied exec")
	}
	if err := checkExecRBAC("bob", "unknown"); err == nil {
		t.Fatal("unknown role should be rejected")
	}
}

func TestEmitSIEMWritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	sink := newSIEMSink(dir)
	if sink == nil {
		t.Fatal("newSIEMSink returned nil")
	}

	emitSIEM(sink, "alice", "app.start", "agentforge", "ok")

	data, err := os.ReadFile(filepath.Join(dir, "siem.log"))
	if err != nil {
		t.Fatalf("read siem.log: %v", err)
	}
	line := string(data)
	for _, want := range []string{`"user":"alice"`, `"action":"app.start"`, `"resource":"agentforge"`, `"result":"ok"`} {
		if !strings.Contains(line, want) {
			t.Fatalf("siem line %q does not contain %q", line, want)
		}
	}
}

func TestRunExecWithIdentityDeniedWritesSIEM(t *testing.T) {
	dir := t.TempDir()
	err := runExecWithIdentity([]string{
		"--local", "--cmd", "echo hi", "--data-dir", dir,
	}, "bob", "guest")
	if err == nil {
		t.Fatal("expected RBAC denial")
	}
	if !strings.Contains(err.Error(), "RBAC denied") {
		t.Fatalf("unexpected error: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "siem.log"))
	if readErr != nil {
		t.Fatalf("read siem.log: %v", readErr)
	}
	if !strings.Contains(string(data), `"result":"denied"`) || !strings.Contains(string(data), `"user":"bob"`) {
		t.Fatalf("siem log missing denied event: %s", data)
	}
}

func TestRunExecWithIdentitySuccessWritesSIEM(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := fmt.Sprintf("%q -test.run=TestHelperProcess -- echo", exe)
	dir := t.TempDir()
	err = runExecWithIdentity([]string{
		"--local", "--cmd", cmd,
		"--data-dir", dir,
		"--env", "GO_WANT_HELPER_PROCESS=1",
		"--env", "GO_HELPER_CMD=echo",
		"--env", "GO_HELPER_ARG=hello",
	}, "alice", "member")
	if err != nil {
		t.Fatalf("run exec with member identity: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "siem.log"))
	if readErr != nil {
		t.Fatalf("read siem.log: %v", readErr)
	}
	if !strings.Contains(string(data), `"result":"ok"`) || !strings.Contains(string(data), `"user":"alice"`) {
		t.Fatalf("siem log missing ok event: %s", data)
	}
}
