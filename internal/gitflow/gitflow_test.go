package gitflow_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/gitflow"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.name", "Test User")
	run(t, dir, "git", "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial commit")

	return dir
}

func run(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestEnsureBranchCreatesBranchAndDiffEmpty(t *testing.T) {
	dir := initRepo(t)
	branch := "feature/test-branch"

	err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir, Branch: branch})
	if err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	current := strings.TrimSpace(run(t, dir, "git", "branch", "--show-current"))
	if current != branch {
		t.Fatalf("current branch = %q, want %q", current, branch)
	}

	diff, err := gitflow.Diff(context.Background(), dir)
	if err != nil {
		t.Fatalf("Diff() error = %v", err)
	}
	if diff != "" {
		t.Fatalf("Diff() = %q, want empty before changes", diff)
	}
}

func TestEnsureBranchUsesDefaultName(t *testing.T) {
	dir := initRepo(t)

	err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir})
	if err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	current := strings.TrimSpace(run(t, dir, "git", "branch", "--show-current"))
	if !strings.HasPrefix(current, "agentflow/") {
		t.Fatalf("current branch = %q, want prefix %q", current, "agentflow/")
	}
}

func TestEnsureBranchChecksOutExistingBranch(t *testing.T) {
	dir := initRepo(t)
	branch := "feature/existing"

	run(t, dir, "git", "checkout", "-b", branch)
	// Move back to the original branch so EnsureBranch has to check out the
	// existing branch rather than simply being on it already.
	run(t, dir, "git", "checkout", "-")

	err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir, Branch: branch})
	if err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	current := strings.TrimSpace(run(t, dir, "git", "branch", "--show-current"))
	if current != branch {
		t.Fatalf("current branch = %q, want %q", current, branch)
	}
}

func TestCommitAddsAndCommitsFile(t *testing.T) {
	dir := initRepo(t)
	branch := "feature/commit-test"

	if err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir, Branch: branch}); err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	sha, err := gitflow.Commit(context.Background(), gitflow.Config{
		WorkDir:   dir,
		CommitMsg: "test commit",
	})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	head := strings.TrimSpace(run(t, dir, "git", "rev-parse", "HEAD"))
	if sha != head {
		t.Fatalf("Commit() SHA = %q, HEAD = %q", sha, head)
	}

	msg := strings.TrimSpace(run(t, dir, "git", "log", "-1", "--pretty=%s"))
	if msg != "test commit" {
		t.Fatalf("commit message = %q, want %q", msg, "test commit")
	}

	status := strings.TrimSpace(run(t, dir, "git", "status", "--porcelain"))
	if status != "" {
		t.Fatalf("working tree not clean after Commit(): %q", status)
	}
}

func TestCreatePRWithFakeGH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh shell script is not portable on Windows")
	}

	dir := initRepo(t)
	branch := "feature/pr-test"

	if err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir, Branch: branch}); err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$GH_ARGS_FILE"
echo "https://example.com/pr/123"
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsFile := filepath.Join(t.TempDir(), "gh-args.txt")
	t.Setenv("GH_ARGS_FILE", argsFile)

	url, err := gitflow.CreatePR(context.Background(), gitflow.Config{
		WorkDir: dir,
		Branch:  branch,
		PRTitle: "My PR",
		PRBody:  "PR body",
	})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}

	if url != "https://example.com/pr/123" {
		t.Fatalf("CreatePR() URL = %q, want %q", url, "https://example.com/pr/123")
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake gh args: %v", err)
	}
	got := string(args)
	for _, want := range []string{"pr", "create", "--title", "My PR", "--body", "PR body", "--head", branch} {
		if !strings.Contains(got, want) {
			t.Fatalf("fake gh args do not contain %q:\n%s", want, got)
		}
	}
}

func TestCreatePRDefaultsTitleToCommitMsg(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh shell script is not portable on Windows")
	}

	dir := initRepo(t)
	branch := "feature/pr-default-title"

	if err := gitflow.EnsureBranch(context.Background(), gitflow.Config{WorkDir: dir, Branch: branch}); err != nil {
		t.Fatalf("EnsureBranch() error = %v", err)
	}

	bin := t.TempDir()
	gh := filepath.Join(bin, "gh")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$GH_ARGS_FILE"
echo "https://example.com/pr/456"
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	argsFile := filepath.Join(t.TempDir(), "gh-args.txt")
	t.Setenv("GH_ARGS_FILE", argsFile)

	_, err := gitflow.CreatePR(context.Background(), gitflow.Config{
		WorkDir:   dir,
		Branch:    branch,
		CommitMsg: "agentflow: automated change",
	})
	if err != nil {
		t.Fatalf("CreatePR() error = %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read fake gh args: %v", err)
	}
	got := string(args)
	if !strings.Contains(got, "--title") || !strings.Contains(got, "agentflow: automated change") {
		t.Fatalf("fake gh args do not contain default title:\n%s", got)
	}
}
