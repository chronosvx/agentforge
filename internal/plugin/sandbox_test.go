package plugin

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRunSandboxedExecutes(t *testing.T) {
	out, err := RunSandboxed(context.Background(), helperBin, []string{"world"}, t.TempDir(), time.Second, Permissions{AllowExec: true})
	if err != nil {
		t.Fatalf("RunSandboxed: %v", err)
	}
	if out != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRunSandboxedUsesWorkDir(t *testing.T) {
	workDir := t.TempDir()
	out, err := RunSandboxed(context.Background(), helperBin, []string{"cwd"}, workDir, time.Second, Permissions{AllowExec: true})
	if err != nil {
		t.Fatalf("RunSandboxed: %v", err)
	}
	if filepath.Clean(out) != filepath.Clean(workDir) {
		t.Fatalf("expected work dir %q, got %q", workDir, out)
	}
}

func TestRunSandboxedTimeout(t *testing.T) {
	start := time.Now()
	_, err := RunSandboxed(context.Background(), helperBin, []string{"slow"}, t.TempDir(), 100*time.Millisecond, Permissions{AllowExec: true})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestRunSandboxedDeniedWithoutExec(t *testing.T) {
	_, err := RunSandboxed(context.Background(), helperBin, nil, t.TempDir(), time.Second, Permissions{})
	if err == nil {
		t.Fatal("expected permission denied error")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestRunSandboxedDeniesNetworkEvenWhenRequested(t *testing.T) {
	_, err := RunSandboxed(context.Background(), helperBin, nil, t.TempDir(), time.Second, Permissions{AllowExec: true, AllowNetwork: true})
	if err == nil {
		t.Fatal("expected network sandbox unsupported error")
	}
	if !errors.Is(err, ErrNetworkSandboxUnsupported) {
		t.Fatalf("expected ErrNetworkSandboxUnsupported, got %v", err)
	}
}

func TestRunSandboxedRejectsEmptyExecutable(t *testing.T) {
	_, err := RunSandboxed(context.Background(), "", nil, t.TempDir(), time.Second, Permissions{AllowExec: true})
	if err == nil {
		t.Fatal("expected error for empty executable")
	}
}
