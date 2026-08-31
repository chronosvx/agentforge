package repomode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanNonGit(t *testing.T) {
	dir := t.TempDir()

	readme := "# Hello\n\nWorld\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(dir, "tests")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "example_test.go"), []byte("package tests\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if info.IsGit {
		t.Fatal("IsGit = true, want false for non-git directory")
	}
	if info.Root != dir {
		t.Fatalf("Root = %q, want %q", info.Root, dir)
	}
	if !contains(info.Files, "README.md") || !contains(info.Files, "tests") {
		t.Fatalf("Files = %v, want README.md and tests", info.Files)
	}
	if info.Readme != strings.TrimSpace(readme) {
		t.Fatalf("Readme = %q, want %q", info.Readme, strings.TrimSpace(readme))
	}
	if len(info.TestDirs) != 1 || info.TestDirs[0] != "tests" {
		t.Fatalf("TestDirs = %v, want [tests]", info.TestDirs)
	}
	if info.GoMod != "" {
		t.Fatalf("GoMod = %q, want empty", info.GoMod)
	}
	if len(info.RecentChanges) != 0 {
		t.Fatalf("RecentChanges = %v, want empty for non-git", info.RecentChanges)
	}
}

func TestScanGit(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "Initial commit")

	info, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if !info.IsGit {
		t.Fatal("IsGit = false, want true for git repository")
	}
	wantRoot, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Root != wantRoot {
		t.Fatalf("Root = %q, want %q", info.Root, wantRoot)
	}
	if len(info.RecentChanges) == 0 {
		t.Fatal("RecentChanges is empty, want at least one commit")
	}
	if !strings.Contains(info.RecentChanges[0], "Initial commit") {
		t.Fatalf("RecentChanges[0] = %q, want it to contain %q", info.RecentChanges[0], "Initial commit")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
