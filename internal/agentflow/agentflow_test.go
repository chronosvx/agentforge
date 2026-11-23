package agentflow

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/executor"
)

func TestStaticPlannerRun(t *testing.T) {
	p := NewStaticPlanner()
	actions, err := p.Plan(context.Background(), "run echo hello", nil)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	if actions[0].Type != agent.ActionTypeRunCommand || actions[0].Command != "echo hello" {
		t.Fatalf("unexpected action: %+v", actions[0])
	}
}

func TestStaticPlannerEdit(t *testing.T) {
	p := NewStaticPlanner()
	actions, err := p.Plan(context.Background(), "edit README.md hello", nil)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("len(actions) = %d, want 1", len(actions))
	}
	if actions[0].Type != agent.ActionTypeEditFile || actions[0].Path != "README.md" || actions[0].Content != "hello" {
		t.Fatalf("unexpected action: %+v", actions[0])
	}
}

func TestStaticPlannerTest(t *testing.T) {
	p := NewStaticPlanner()
	actions, err := p.Plan(context.Background(), "帮我跑一下测试", nil)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != agent.ActionTypeRunCommand {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

type fakeExecutor struct {
	calls []string
}

func (f *fakeExecutor) Run(_ context.Context, spec executor.Spec) (executor.Result, error) {
	f.calls = append(f.calls, strings.Join(spec.Args, " "))
	return executor.Result{Stdout: "ok\n"}, nil
}

func TestRunnerAutoApproveRunCommand(t *testing.T) {
	fe := &fakeExecutor{}
	r := NewRunner(NewStaticPlanner(), fe)
	dir := t.TempDir()
	res, err := r.Run(context.Background(), Request{
		Prompt:      "run echo hello",
		WorkDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.Session.Phase != agent.PhaseDone && res.Session.Phase != agent.PhaseReviewingDiff {
		t.Fatalf("unexpected phase: %s", res.Session.Phase)
	}
	if len(fe.calls) == 0 {
		t.Fatal("expected executor call")
	}
}

func TestRunnerRejectWithoutApprover(t *testing.T) {
	fe := &fakeExecutor{}
	r := NewRunner(NewStaticPlanner(), fe)
	dir := t.TempDir()
	_, err := r.Run(context.Background(), Request{
		Prompt:      "run echo hello",
		WorkDir:     dir,
		AutoApprove: false,
	})
	if err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("expected rejection error, got %v", err)
	}
}

func TestSafePathRejectsGitPaths(t *testing.T) {
	r := &Runner{}
	_, err := r.executeAction(context.Background(), agent.Action{
		Type:    agent.ActionTypeEditFile,
		Path:    ".git/config",
		Content: "pwned",
	}, t.TempDir(), 0)
	if err == nil || !strings.Contains(err.Error(), ".git") {
		t.Fatalf("expected .git path rejection, got %v", err)
	}
}

func TestSafePathRejectsSymlinkEscape(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(work, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	r := &Runner{}
	_, err := r.executeAction(context.Background(), agent.Action{
		Type:    agent.ActionTypeEditFile,
		Path:    "link/pwned",
		Content: "pwned",
	}, work, 0)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestRunnerTestFailureReturnsError(t *testing.T) {
	r := NewRunner(NewStaticPlanner(), nil)
	dir := t.TempDir()
	res, err := r.Run(context.Background(), Request{
		Prompt:      "run echo ok",
		WorkDir:     dir,
		AutoApprove: true,
		TestCmd:     "false",
	})
	if err == nil || !strings.Contains(err.Error(), "tests failed") {
		t.Fatalf("expected tests failed error, got %v", err)
	}
	if res == nil {
		t.Fatal("expected a result even when tests fail")
	}
	if res.TestOK {
		t.Fatal("expected TestOK to be false")
	}
}

type fixedPlanner struct {
	actions []agent.Action
}

func (p fixedPlanner) Plan(context.Context, string, *Repo) ([]agent.Action, error) {
	return p.actions, nil
}

func TestRunnerRequiresApprovalForEveryAction(t *testing.T) {
	fe := &fakeExecutor{}
	r := NewRunner(fixedPlanner{actions: []agent.Action{
		{ID: "a1", Type: agent.ActionTypeRunCommand, Command: "echo one"},
		{ID: "a2", Type: agent.ActionTypeRunCommand, Command: "echo two"},
	}}, fe)
	approvals := 0
	r.Approve = func(agent.Action) (bool, error) {
		approvals++
		return true, nil
	}

	res, err := r.Run(context.Background(), Request{
		Prompt:  "two commands",
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if approvals != 2 {
		t.Fatalf("approvals = %d, want 2 (one per action)", approvals)
	}
	if len(fe.calls) != 2 {
		t.Fatalf("executor calls = %d, want 2", len(fe.calls))
	}
	if res.Session.Actions[0].Status != agent.ActionStatusExecuted || res.Session.Actions[1].Status != agent.ActionStatusExecuted {
		t.Fatalf("actions not executed: %+v", res.Session.Actions)
	}
}

func TestRunnerDryRunDoesNotExecute(t *testing.T) {
	fe := &fakeExecutor{}
	r := NewRunner(NewStaticPlanner(), fe)
	r.Approve = func(agent.Action) (bool, error) {
		t.Fatal("dry-run must not request approval")
		return false, nil
	}
	dir := t.TempDir()
	res, err := r.Run(context.Background(), Request{
		Prompt:      "run echo hello",
		WorkDir:     dir,
		AutoApprove: false,
		TestCmd:     "false",
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(fe.calls) != 0 {
		t.Fatalf("expected no executor calls in dry-run, got %d", len(fe.calls))
	}
	if len(res.Session.Actions) != 1 || res.Session.Actions[0].Status != agent.ActionStatusPending {
		t.Fatalf("unexpected dry-run actions: %+v", res.Session.Actions)
	}
	if res.Session.Phase != agent.PhasePreview {
		t.Fatalf("dry-run phase = %s, want %s", res.Session.Phase, agent.PhasePreview)
	}
	if res.TestStatus != TestStatusNotRun || res.TestRan || res.TestOK {
		t.Fatalf("dry-run test status = %s ran=%v ok=%v, want not_run", res.TestStatus, res.TestRan, res.TestOK)
	}
}

func TestRunnerSkippedTestIsNotReportedAsPassed(t *testing.T) {
	r := NewRunner(NewStaticPlanner(), &fakeExecutor{})
	res, err := r.Run(context.Background(), Request{
		Prompt:      "run echo hello",
		WorkDir:     t.TempDir(),
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TestStatus != TestStatusSkipped || res.TestRan || res.TestOK {
		t.Fatalf("test status = %s ran=%v ok=%v, want skipped", res.TestStatus, res.TestRan, res.TestOK)
	}
	if len(res.Session.Actions) != 1 || res.Session.Actions[0].Status != agent.ActionStatusExecuted {
		t.Fatalf("unexpected actions after skipped tests: %+v", res.Session.Actions)
	}
	if res.Session.Phase != agent.PhaseReviewingDiff {
		t.Fatalf("phase = %s, want %s", res.Session.Phase, agent.PhaseReviewingDiff)
	}
}

func TestRunnerExecutionFailureMarksActionFailed(t *testing.T) {
	r := NewRunner(NewStaticPlanner(), nil)
	res, err := r.Run(context.Background(), Request{
		Prompt:      "run false",
		WorkDir:     t.TempDir(),
		AutoApprove: true,
	})
	if err == nil {
		t.Fatal("expected execution failure")
	}
	if res == nil || len(res.Session.Actions) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Session.Actions[0].Status != agent.ActionStatusFailed {
		t.Fatalf("action status = %s, want failed", res.Session.Actions[0].Status)
	}
	if res.Session.Phase != agent.PhaseFailed {
		t.Fatalf("phase = %s, want failed", res.Session.Phase)
	}
	if res.TestStatus != TestStatusNotRun {
		t.Fatalf("test status = %s, want not_run", res.TestStatus)
	}
}

func TestRunnerDiffScopesTaskChangesFromPreExisting(t *testing.T) {
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	runGitTest(t, dir, "config", "user.name", "Test User")
	runGitTest(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "add", "base.txt")
	runGitTest(t, dir, "commit", "-m", "initial")

	// 预先存在的改动：一个未暂存、一个已暂存。
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "staged.txt"), []byte("staged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "add", "staged.txt")

	r := NewRunner(NewStaticPlanner(), nil)
	res, err := r.Run(context.Background(), Request{
		Prompt:      "edit new.txt hello",
		WorkDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(res.TaskPaths) != 1 || res.TaskPaths[0] != "new.txt" {
		t.Fatalf("task paths = %v, want [new.txt]", res.TaskPaths)
	}
	if strings.Join(res.PreExistingPaths, ",") != "dirty.txt,staged.txt" {
		t.Fatalf("pre-existing paths = %v, want dirty.txt,staged.txt", res.PreExistingPaths)
	}
	if len(res.ConflictedPaths) != 0 {
		t.Fatalf("conflicted paths = %v, want none", res.ConflictedPaths)
	}
	if !strings.Contains(res.Diff, "new.txt") || !strings.Contains(res.Diff, "+hello") {
		t.Fatalf("task diff missing new file content:\n%s", res.Diff)
	}
	if strings.Contains(res.Diff, "dirty.txt") || strings.Contains(res.Diff, "staged.txt") {
		t.Fatalf("task diff leaked pre-existing changes:\n%s", res.Diff)
	}
}

func TestRunnerMarksConflictWhenTaskTouchesDirtyFile(t *testing.T) {
	dir := t.TempDir()
	runGitTest(t, dir, "init")
	runGitTest(t, dir, "config", "user.name", "Test User")
	runGitTest(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, dir, "add", "dirty.txt")
	runGitTest(t, dir, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(NewStaticPlanner(), nil)
	res, err := r.Run(context.Background(), Request{
		Prompt:      "edit dirty.txt task\n",
		WorkDir:     dir,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if strings.Join(res.ConflictedPaths, ",") != "dirty.txt" {
		t.Fatalf("conflicted paths = %v, want [dirty.txt]", res.ConflictedPaths)
	}
}

func TestRunnerWorkdirSubdirectoryUsesRepoRelativePaths(t *testing.T) {
	root := t.TempDir()
	runGitTest(t, root, "init")
	runGitTest(t, root, "config", "user.name", "Test User")
	runGitTest(t, root, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(root, "base.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, root, "add", "base.txt")
	runGitTest(t, root, "commit", "-m", "initial")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(NewStaticPlanner(), nil)
	res, err := r.Run(context.Background(), Request{
		Prompt:      "edit new.txt hello",
		WorkDir:     sub,
		AutoApprove: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.RepoRoot == "" {
		t.Fatal("RepoRoot is empty, want Git root")
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotRoot, err := filepath.EvalSymlinks(res.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(gotRoot) != filepath.Clean(wantRoot) {
		t.Fatalf("RepoRoot = %q, want %q", res.RepoRoot, root)
	}
	if strings.Join(res.TaskPaths, ",") != "sub/new.txt" {
		t.Fatalf("task paths = %v, want [sub/new.txt]", res.TaskPaths)
	}
	if !strings.Contains(res.Diff, "sub/new.txt") {
		t.Fatalf("diff missing repo-relative path:\n%s", res.Diff)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
