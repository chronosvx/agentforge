package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/agentflow"
	"github.com/chronosvx/agentforge/internal/gitflow"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/repomode"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// buildRepoContext 将仓库扫描结果格式化为简短的上下文块。
func buildRepoContext(info *repomode.Info) string {
	if info == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Repo context:\n")
	fmt.Fprintf(&b, "Root: %s\n", info.Root)
	fmt.Fprintf(&b, "Name: %s\n", info.Name)
	fmt.Fprintf(&b, "TestDirs: %s\n", strings.Join(info.TestDirs, ","))
	fmt.Fprintf(&b, "Recent changes:\n")
	for _, change := range info.RecentChanges {
		fmt.Fprintln(&b, change)
	}
	return strings.TrimRight(b.String(), "\n")
}

// staticPlannerWithOriginalPrompt 让静态规划器在请求携带仓库上下文时，
// 仍使用原始用户请求进行确定性解析。
type staticPlannerWithOriginalPrompt struct {
	planner *agentflow.StaticPlanner
	prompt  string
}

func (p staticPlannerWithOriginalPrompt) Plan(ctx context.Context, _ string, repo *agentflow.Repo) ([]agent.Action, error) {
	return p.planner.Plan(ctx, p.prompt, repo)
}

// runAgent 实现 `agentforge agent` 子命令：Agent 主流程端到端 Demo。
func runAgent(args []string) error {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	planner := fs.String("planner", "static", "规划器: static 或 llm")
	repoMode := fs.Bool("repo", false, "扫描仓库并添加上下文")
	providerName := fs.String("provider", "", "LLM 提供商")
	providerKey := fs.String("provider-key", "", "LLM API Key")
	model := fs.String("model", "", "LLM 模型")
	baseURL := fs.String("base-url", "", "LLM API Base URL")
	autoApprove := fs.Bool("yes", false, "自动批准所有动作")
	prompt := fs.String("prompt", "", "自然语言需求")
	workdir := fs.String("workdir", ".", "工作目录")
	testCmd := fs.String("test", "go test ./...", "测试命令；传空字符串可禁用测试")
	timeout := fs.Duration("timeout", 5*time.Minute, "单条命令超时")
	branch := fs.String("branch", "", "gitflow 分支名（默认自动生成）")
	doCommit := fs.Bool("commit", false, "成功后自动创建 git 分支并提交")
	doPush := fs.Bool("push", false, "提交后推送到 origin")
	doPR := fs.Bool("pr", false, "提交后创建 PR（需要 gh CLI）")
	prTitle := fs.String("pr-title", "", "PR 标题（默认使用提交信息）")
	prBody := fs.String("pr-body", "", "PR 描述")
	jsonOut := fs.Bool("json", false, "以 JSON 格式输出结果")
	dryRun := fs.Bool("dry-run", false, "只生成计划并预览，不实际执行动作")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*prompt) == "" && fs.NArg() > 0 {
		*prompt = strings.Join(fs.Args(), " ")
	}
	if strings.TrimSpace(*prompt) == "" {
		return errors.New("agent: --prompt is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	planPrompt := *prompt
	if *repoMode {
		info, err := repomode.Scan(ctx, *workdir)
		if err != nil {
			return fmt.Errorf("agent: scan repo: %w", err)
		}
		planPrompt = buildRepoContext(info) + "\n\n" + *prompt
	}

	var p agentflow.Planner
	switch strings.ToLower(strings.TrimSpace(*planner)) {
	case "", "static":
		sp := agentflow.NewStaticPlanner()
		if *repoMode {
			// 静态规划器只识别以 run/edit/test 开头的原始请求，
			// 因此请求中保留仓库上下文，但规划时使用原始用户请求。
			p = staticPlannerWithOriginalPrompt{planner: sp, prompt: *prompt}
		} else {
			p = sp
		}
	case "llm":
		if strings.TrimSpace(*providerName) == "" {
			return errors.New("agent: --provider is required with --planner llm")
		}
		prov, err := provider.NewProvider(provider.ProviderOptions{
			Name:    *providerName,
			APIKey:  *providerKey,
			BaseURL: *baseURL,
			Model:   *model,
		})
		if err != nil {
			return fmt.Errorf("agent: provider: %w", err)
		}
		plannerModel := strings.TrimSpace(*model)
		if plannerModel == "" {
			plannerModel = prov.Model()
		}
		if plannerModel == "" {
			return errors.New("agent: provider did not provide a default model")
		}
		p = agentflow.NewLLMPlanner(prov, plannerModel)
	default:
		return fmt.Errorf("agent: unknown planner %q", *planner)
	}

	runner := agentflow.NewRunner(p, nil)
	if !*autoApprove {
		runner.Approve = func(a agent.Action) (bool, error) {
			fmt.Printf("需要批准动作:\n  %s [%s] %s\n", a.ID, a.Type, a.Description)
			if a.Command != "" {
				fmt.Printf("  command: %s\n", a.Command)
			}
			if a.Path != "" {
				fmt.Printf("  path: %s\n", a.Path)
			}
			fmt.Print("批准? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return false, err
			}
			answer := strings.ToLower(strings.TrimSpace(line))
			return answer == "y" || answer == "yes", nil
		}
	}

	res, err := runner.Run(ctx, agentflow.Request{
		Prompt:      planPrompt,
		WorkDir:     *workdir,
		AutoApprove: *autoApprove,
		Timeout:     *timeout,
		TestCmd:     *testCmd,
		DryRun:      *dryRun,
	})
	if err != nil {
		if res != nil && !res.TestOK && (*doCommit || *doPush || *doPR) {
			return fmt.Errorf("agent: tests failed; refusing to commit")
		}
		return err
	}

	if !*jsonOut {
		fmt.Println("\n===== Agent 主流程结果 =====")
		fmt.Printf("Phase: %s\n", res.Session.Phase)
		for _, a := range res.Session.Actions {
			fmt.Printf("Action: %s [%s] %s\n", a.ID, a.Status, a.Description)
			if a.Output != "" {
				fmt.Printf("  Output: %s\n", a.Output)
			}
		}
		if res.TestOutput != "" {
			fmt.Printf("Test output:\n%s\n", res.TestOutput)
		}
		if *testCmd != "" {
			fmt.Printf("Test OK: %v\n", res.TestOK)
		}
		if res.Diff != "" {
			fmt.Printf("Diff:\n%s\n", res.Diff)
		} else {
			fmt.Println("Diff: (none)")
		}
	}

	if !res.TestOK && (*doCommit || *doPush || *doPR) {
		return fmt.Errorf("agent: tests failed; refusing to commit")
	}

	if *doCommit {
		cfg := gitflow.Config{
			WorkDir:   *workdir,
			Branch:    *branch,
			CommitMsg: "agentflow: " + *prompt,
			PRTitle:   *prTitle,
			PRBody:    *prBody,
			Push:      *doPush,
		}
		if err := gitflow.EnsureBranch(ctx, cfg); err != nil {
			return fmt.Errorf("agent: ensure branch: %w", err)
		}
		if cfg.Branch == "" {
			cfg.Branch = currentGitBranch(*workdir)
			if cfg.Branch == "" {
				return errors.New("agent: unable to determine current branch after ensure branch")
			}
		}
		actualBranch := cfg.Branch
		sha, err := gitflow.Commit(ctx, cfg)
		if err != nil {
			if strings.Contains(err.Error(), "nothing to commit") || strings.Contains(err.Error(), "no changes added") {
				if !*jsonOut {
					fmt.Println("Commit: (nothing to commit)")
				}
			} else {
				return fmt.Errorf("agent: commit: %w", err)
			}
		} else {
			if !*jsonOut {
				fmt.Printf("Commit: %s on %s\n", sha, actualBranch)
			}
		}
		if *doPush {
			if err := gitflow.Push(ctx, cfg); err != nil {
				return fmt.Errorf("agent: push: %w", err)
			}
			if !*jsonOut {
				fmt.Printf("Pushed: %s\n", actualBranch)
			}
		}
		if *doPR {
			url, err := gitflow.CreatePR(ctx, cfg)
			if err != nil {
				return fmt.Errorf("agent: create PR: %w", err)
			}
			if !*jsonOut {
				fmt.Printf("PR: %s\n", url)
			}
		}
	}

	if *jsonOut {
		type jsonAction struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Status      string `json:"status"`
			Description string `json:"description"`
			Output      string `json:"output,omitempty"`
		}
		actions := make([]jsonAction, 0, len(res.Session.Actions))
		for _, a := range res.Session.Actions {
			actions = append(actions, jsonAction{
				ID:          a.ID,
				Type:        string(a.Type),
				Status:      string(a.Status),
				Description: a.Description,
				Output:      a.Output,
			})
		}
		out := map[string]any{
			"phase":       res.Session.Phase,
			"actions":     actions,
			"test_output": res.TestOutput,
			"test_ok":     res.TestOK,
			"diff":        res.Diff,
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}

	return nil
}

// currentGitBranch 返回指定目录当前 Git 分支名；失败时返回空字符串。
func currentGitBranch(dir string) string {
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
