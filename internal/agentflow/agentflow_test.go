package agentflow

import (
	"context"
	"os"
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
