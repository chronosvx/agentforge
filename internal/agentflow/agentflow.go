// Package agentflow 提供 Agent 主流程的高层编排：计划 -> 审批 -> 执行 ->
// 测试 -> Diff 评审 -> 提交/回滚。
//
// 它把 internal/agent 的状态机、internal/executor 的执行器以及 Git 操作
// 组合成一个可被 CLI、TUI 或自动化复用的 Runner。
package agentflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/executor"
)

// Repo 描述 Agent 当前工作的代码仓库。
type Repo struct {
	WorkDir string
	IsGit   bool
	Name    string
}

// Request 是运行一次 Agent 主流程的输入。
type Request struct {
	Prompt      string
	WorkDir     string
	AutoApprove bool
	Timeout     time.Duration
	TestCmd     string
}

// Result 是一次 Agent 主流程运行的输出。
type Result struct {
	Session    *agent.AgentSession
	Actions    []agent.Action
	Diff       string
	TestOutput string
	TestOK     bool
}

// Planner 根据自然语言请求生成 Agent 计划。
type Planner interface {
	Plan(ctx context.Context, prompt string, repo *Repo) ([]agent.Action, error)
}

// Approver 在 RequireApproval 模式下决定是否批准一个动作。
type Approver func(action agent.Action) (bool, error)

// Runner 驱动 AgentSession 完成整个闭环。
type Runner struct {
	Planner Planner
	Exec    executor.Executor
	Approve Approver
}

// NewRunner 创建一个 Runner。exec 为 nil 时使用本地执行器。
func NewRunner(p Planner, e executor.Executor) *Runner {
	if e == nil {
		e = executor.NewLocalExecutor()
	}
	return &Runner{Planner: p, Exec: e}
}

// Run 执行完整主流程并返回结果。
func (r *Runner) Run(ctx context.Context, req Request) (*Result, error) {
	if r == nil || r.Planner == nil {
		return nil, errors.New("agentflow: runner or planner is nil")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("agentflow: prompt is required")
	}
	if req.WorkDir == "" {
		req.WorkDir = "."
	}
	abs, err := filepath.Abs(req.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("agentflow: resolve workdir: %w", err)
	}
	repo := detectRepo(ctx, abs)

	actions, err := r.Planner.Plan(ctx, req.Prompt, repo)
	if err != nil {
		return nil, fmt.Errorf("agentflow: plan: %w", err)
	}
	if len(actions) == 0 {
		return nil, errors.New("agentflow: planner returned an empty plan")
	}

	policy := agent.AllowAll
	if !req.AutoApprove {
		policy = agent.RequireApproval
	}
	session := agent.NewAgentSession(policy)
	session.Plan(actions)

	result := &Result{Session: session, Actions: append([]agent.Action(nil), actions...)}

	for {
		a := session.NextPendingAction()
		if a == nil {
			break
		}

		if session.Phase == agent.PhaseAwaitingApproval {
			ok := req.AutoApprove
			if !ok && r.Approve != nil {
				ok, err = r.Approve(*a)
				if err != nil {
					return result, fmt.Errorf("agentflow: approve: %w", err)
				}
			}
			if !ok {
				_ = session.Reject(a.ID)
				return result, fmt.Errorf("agentflow: action %s rejected", a.ID)
			}
			if err := session.Approve(a.ID); err != nil {
				return result, fmt.Errorf("agentflow: approve: %w", err)
			}
			a = session.NextPendingAction()
			if a == nil {
				break
			}
		}

		output, execErr := r.executeAction(ctx, *a, abs, req.Timeout)
		if execErr != nil {
			_ = session.MarkExecuted(a.ID, output+"\n"+execErr.Error())
			return result, fmt.Errorf("agentflow: execute %s: %w", a.ID, execErr)
		}
		if err := session.MarkExecuted(a.ID, output); err != nil {
			return result, fmt.Errorf("agentflow: mark executed: %w", err)
		}
	}

	// 有动作执行成功且配置了测试命令时，统一跑一次测试。
	// 未配置测试命令时，视为测试通过，让闭环进入 Diff 评审阶段。
	if len(actions) > 0 && req.TestCmd != "" {
		cmd := req.TestCmd
		res, testErr := r.Exec.Run(ctx, executor.Spec{
			Command: "sh",
			Args:    []string{"-c", cmd},
			WorkDir: abs,
			Timeout: req.Timeout,
		})
		result.TestOutput = strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
		result.TestOK = testErr == nil
		for i := range session.Actions {
			a := &session.Actions[i]
			if a.Status == agent.ActionStatusExecuted {
				_ = session.MarkTested(a.ID, result.TestOK)
				if !result.TestOK {
					break
				}
			}
		}
	} else if len(actions) > 0 {
		result.TestOK = true
		for i := range session.Actions {
			a := &session.Actions[i]
			if a.Status == agent.ActionStatusExecuted {
				_ = session.MarkTested(a.ID, true)
			}
		}
	}

	if repo.IsGit {
		diff, diffErr := gitDiff(ctx, abs)
		if diffErr == nil {
			result.Diff = diff
			session.SetDiff(diff)
		}
	}

	if req.TestCmd != "" && !result.TestOK {
		return result, fmt.Errorf("agentflow: tests failed")
	}

	return result, nil
}

// executeAction 执行单个 Agent 动作。
func (r *Runner) executeAction(ctx context.Context, a agent.Action, workDir string, timeout time.Duration) (string, error) {
	switch a.Type {
	case agent.ActionTypeRunCommand:
		if strings.TrimSpace(a.Command) == "" {
			return "", errors.New("agentflow: run_command has empty command")
		}
		res, err := r.Exec.Run(ctx, executor.Spec{
			Command: "sh",
			Args:    []string{"-c", a.Command},
			WorkDir: workDir,
			Timeout: timeout,
		})
		out := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
		if err != nil {
			return out, err
		}
		if out == "" {
			out = "ok"
		}
		return out, nil
	case agent.ActionTypeEditFile:
		if a.Path == "" {
			return "", errors.New("agentflow: edit_file has empty path")
		}
		path, err := safePath(workDir, a.Path)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("agentflow: mkdir: %w", err)
		}
		content := a.Content
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("agentflow: write file: %w", err)
		}
		return "wrote " + filepath.ToSlash(path), nil
	case agent.ActionTypeApplyDiff:
		if a.Content == "" {
			return "", errors.New("agentflow: apply_diff has empty diff")
		}
		return applyDiff(ctx, workDir, a.Content)
	default:
		return "", fmt.Errorf("agentflow: unsupported action type %s", a.Type)
	}
}

// safePath 确保目标路径不会逃逸出 workDir。
func safePath(workDir, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("agentflow: empty path")
	}
	clean := filepath.Clean(p)
	if filepath.IsAbs(clean) {
		clean = strings.TrimPrefix(clean, string(filepath.Separator))
	}
	full := filepath.Join(workDir, clean)
	rel, err := filepath.Rel(workDir, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("agentflow: path %q escapes workdir", p)
	}

	// Reject any path component that is a .git directory, regardless of case.
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if strings.EqualFold(part, ".git") {
			return "", fmt.Errorf("agentflow: path %q targets .git", p)
		}
	}

	absWork, err := filepath.Abs(workDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}

	resolvedWork := absWork
	if resolved, err := filepath.EvalSymlinks(absWork); err == nil {
		resolvedWork = resolved
	}

	resolvedParent, err := resolveExistingParent(absFull)
	if err != nil {
		return "", fmt.Errorf("agentflow: resolve path %q: %w", p, err)
	}
	resolved := filepath.Join(resolvedParent, filepath.Base(absFull))

	// If the final path already exists, resolve it too so an existing symlink
	// cannot redirect the write outside the workdir.
	if fi, statErr := os.Lstat(absFull); statErr == nil {
		if resolvedFinal, evalErr := filepath.EvalSymlinks(absFull); evalErr == nil {
			resolved = resolvedFinal
		} else if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("agentflow: path %q escapes workdir via symlink", p)
		}
	}

	if !pathWithin(resolvedWork, resolved) {
		return "", fmt.Errorf("agentflow: path %q escapes workdir via symlink", p)
	}

	// Also reject .git components introduced through symlinks.
	if resolvedRel, err := filepath.Rel(resolvedWork, resolved); err == nil {
		for _, part := range strings.Split(resolvedRel, string(filepath.Separator)) {
			if strings.EqualFold(part, ".git") {
				return "", fmt.Errorf("agentflow: path %q targets .git", p)
			}
		}
	}

	return full, nil
}

// resolveExistingParent resolves the deepest existing ancestor of path and
// re-appends any not-yet-existing suffix. This lets safePath catch symlinked
// directories even when the final file or some intermediate directory does not
// exist yet.
func resolveExistingParent(path string) (string, error) {
	dir := filepath.Dir(path)
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(dir)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		// If dir itself is a symlink whose target is missing, resolve the link
		// target and keep walking from there so dangling symlinks cannot hide an
		// escape.
		if fi, lerr := os.Lstat(dir); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
			target, rerr := os.Readlink(dir)
			if rerr != nil {
				return "", rerr
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(dir), target)
			}
			dir = target
			continue
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", err
		}
		missing = append(missing, filepath.Base(dir))
		dir = parent
	}
}

// pathWithin reports whether child is equal to root or located under it.
func pathWithin(root, child string) bool {
	rel, err := filepath.Rel(root, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// applyDiff 使用 git apply 应用 diff 内容。
func applyDiff(ctx context.Context, workDir, diff string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "apply", "-")
	cmd.Stdin = strings.NewReader(diff)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("agentflow: git apply: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return "applied diff", nil
}

// gitDiff 返回工作区未暂存的 diff。
func gitDiff(ctx context.Context, workDir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", workDir, "diff")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// detectRepo 检测目录是否在 Git 仓库内。
func detectRepo(ctx context.Context, dir string) *Repo {
	repo := &Repo{WorkDir: dir}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return repo
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return repo
	}
	repo.IsGit = true
	repo.Name = filepath.Base(root)
	repo.WorkDir = root
	return repo
}
