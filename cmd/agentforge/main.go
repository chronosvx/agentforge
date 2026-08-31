// Command agentforge 是 agentforge 的入口。
//
// 它负责：
//   - 解析命令行配置；
//   - 执行会话导入/导出/同步子命令；
//   - 创建会话存储；
//   - 连接真实 DSH 或内置 mock server；
//   - 启动 Bubble Tea UI。
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/agentflow"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/chronosvx/agentforge/internal/enterprise"
	"github.com/chronosvx/agentforge/internal/executor"
	"github.com/chronosvx/agentforge/internal/mock"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/repomode"
	"github.com/chronosvx/agentforge/internal/secret"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/share"
	"github.com/chronosvx/agentforge/internal/telemetry"
	"github.com/chronosvx/agentforge/internal/ui"
	"github.com/chronosvx/agentforge/internal/update"
)

func main() {
	if err := run(); err != nil {
		// --help/-h 由 flag 包输出用法后正常退出。
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "agentforge:", err)
		os.Exit(1)
	}
}

const version = "0.3.0"

func run() error {
	user, roles, args, err := extractGlobalIdentity(os.Args[1:])
	if err != nil {
		return err
	}

	maybeCheckUpdate()

	if len(args) > 0 {
		switch args[0] {
		case "export":
			return runExport(args[1:])
		case "import":
			return runImport(args[1:])
		case "sync":
			return runSync(args[1:])
		case "sync-remote":
			return runSyncRemote(args[1:])
		case "share":
			return runShare(args[1:])
		case "open":
			return runOpen(args[1:])
		case "exec":
			return runExecWithIdentity(args[1:], user, roles)
		case "agent":
			return runAgent(args[1:])
		}
	}

	headless, prompt, rest, err := extractHeadlessOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Parse(rest)
	if err != nil {
		return err
	}
	if cfg.User == "" {
		cfg.User = user
	}
	if cfg.Roles == "" {
		cfg.Roles = roles
	}
	if cfg.Version {
		fmt.Println("agentforge", version)
		return nil
	}
	if !headless && prompt != "" {
		return errors.New("--prompt requires --headless")
	}
	if headless && strings.TrimSpace(prompt) == "" {
		return errors.New("--prompt is required with --headless")
	}

	telemetry.SetEnabled(cfg.Telemetry)
	if telemetry.Enabled() {
		if err := telemetry.SetDataDir(cfg.DataDir); err != nil {
			return fmt.Errorf("init telemetry: %w", err)
		}
		_ = telemetry.Record("app.started", map[string]any{"version": version})
	}

	warnIfInsecureRemote(cfg)
	warnIfDataExport(cfg)

	auditLog := newAuditLogger(cfg.DataDir, !cfg.NoAudit)
	audit(auditLog, "app.started", map[string]any{
		"version": version,
		"user":    cfg.User,
		"roles":   cfg.Roles,
	})

	siemLog := newSIEMSink(cfg.DataDir)
	emitSIEM(siemLog, cfg.User, "app.start", "agentforge", "ok")

	if err := resolveProviderKey(cfg); err != nil {
		return err
	}

	store, err := session.NewStore(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("init session store: %w", err)
	}

	cl, cleanup, err := dial(cfg, store)
	if err != nil {
		return err
	}
	defer cleanup()
	defer cl.CloseConn()

	if headless {
		return runHeadless(cl, cfg, prompt, siemLog)
	}

	model := ui.New(cfg, cl, store, func(event string, fields map[string]any) {
		audit(auditLog, event, fields)
	})
	program := tea.NewProgram(model)
	final, err := program.Run()
	if m, ok := final.(ui.Model); ok {
		m.FlushPersist()
		_ = m.ClosePersist()
	}
	if err != nil {
		return fmt.Errorf("run ui: %w", err)
	}

	return nil
}

// maybeCheckUpdate 在后台/启动路径静默检查 GitHub 上的新版本。
// 检查失败或网络不可用时忽略错误，不阻塞主流程。
func maybeCheckUpdate() {
	if os.Getenv("AGENTFORGE_NO_UPDATE_CHECK") != "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	latest, err := update.LatestFromGitHubWithClient(ctx, "chronosvx", "agentforge", update.GitHubAPIBase, &http.Client{Timeout: 3 * time.Second})
	if err != nil {
		return
	}

	newer, err := update.Check(version, latest)
	if err != nil {
		return
	}
	if newer {
		fmt.Fprintf(os.Stderr, "agentforge: new version %s available (current %s)\n", latest, version)
	}
}

// extractGlobalIdentity 在子命令之前提取可选的 --user 与 --roles 全局参数。
//
// 一旦遇到第一个非 flag 参数（通常是子命令名），就停止提取，避免误删
// `exec --user <ssh-user>` 中属于 SSH 的 --user。
func extractGlobalIdentity(args []string) (user, roles string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			rest = append(rest, args[i:]...)
			return user, roles, rest, nil
		}
		switch {
		case arg == "--user":
			if i+1 >= len(args) {
				return "", "", nil, errors.New("--user requires a value")
			}
			user = args[i+1]
			i++
		case strings.HasPrefix(arg, "--user="):
			user = strings.TrimPrefix(arg, "--user=")
		case arg == "--roles":
			if i+1 >= len(args) {
				return "", "", nil, errors.New("--roles requires a value")
			}
			roles = args[i+1]
			i++
		case strings.HasPrefix(arg, "--roles="):
			roles = strings.TrimPrefix(arg, "--roles=")
		default:
			rest = append(rest, arg)
		}
	}
	return user, roles, rest, nil
}

// extractHeadlessOptions 从参数中提取 --headless 与 --prompt，返回剩余参数。
// 它避免修改 internal/config 的 flag 集合，同时让 headless 参数可被主流程识别。
func extractHeadlessOptions(args []string) (headless bool, prompt string, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--headless":
			headless = true
		case strings.HasPrefix(arg, "--headless="):
			switch strings.TrimPrefix(arg, "--headless=") {
			case "true":
				headless = true
			case "false":
				headless = false
			default:
				return false, "", nil, fmt.Errorf("invalid --headless value %q", strings.TrimPrefix(arg, "--headless="))
			}
		case arg == "--prompt":
			if i+1 >= len(args) {
				return false, "", nil, errors.New("--prompt requires a value")
			}
			prompt = args[i+1]
			i++
		case strings.HasPrefix(arg, "--prompt="):
			prompt = strings.TrimPrefix(arg, "--prompt=")
		default:
			rest = append(rest, arg)
		}
	}
	return headless, prompt, rest, nil
}

// runHeadless 在非交互模式下通过现有 ChatBackend 发送 prompt，等待流式完成后打印最终结果。
func runHeadless(cl ui.ChatBackend, cfg *config.Config, prompt string, siemLog *enterprise.LogSink) (err error) {
	if strings.TrimSpace(prompt) == "" {
		return errors.New("headless: --prompt is required")
	}
	defer func() {
		result := "ok"
		if err != nil {
			result = "error"
		}
		emitSIEM(siemLog, cfg.User, "headless.complete", "headless", result)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	snap, err := cl.Open(ctx, cfg.Workspace, cfg.SessionID, nil)
	if err != nil {
		return fmt.Errorf("headless: open session: %w", err)
	}
	events, err := cl.Subscribe(ctx, snap.ChannelID, snap.Version)
	if err != nil {
		return fmt.Errorf("headless: subscribe: %w", err)
	}
	if _, err := cl.Invoke(ctx, snap.ChannelID, "chat", []any{prompt}); err != nil {
		return fmt.Errorf("headless: invoke: %w", err)
	}

	content, err := waitForHeadlessResult(ctx, events, snap.ChannelID)
	if err != nil {
		return err
	}
	fmt.Println(content)
	return nil
}

// waitForHeadlessResult 消费事件流直到完成。Provider 后端会发送 done 事件；
// 其他后端（如 DSH API）没有 done 事件时，在最后一条增量后短暂静默即视为完成。
func waitForHeadlessResult(ctx context.Context, events <-chan client.Event, channelID string) (string, error) {
	const idleTimeout = 3 * time.Second

	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	content := ""
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-events:
			if !ok {
				if content != "" {
					return content, nil
				}
				return "", fmt.Errorf("headless: event stream for %s closed before completion", channelID)
			}
			if ev.Operation == "error" {
				return "", fmt.Errorf("headless: backend error: %s", headlessErrorFromState(ev.State))
			}
			content = headlessContentFromState(ev.State)
			if ev.Operation == "done" {
				return content, nil
			}
			if content != "" {
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(idleTimeout)
			}
		case <-idle.C:
			if content != "" {
				return content, nil
			}
		}
	}
}

// headlessContentFromState 从后端事件状态中提取最后一条 assistant 消息。
func headlessContentFromState(state map[string]any) string {
	switch msgs := state["messages"].(type) {
	case []any:
		for i := len(msgs) - 1; i >= 0; i-- {
			m, ok := msgs[i].(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role != "assistant" {
				continue
			}
			if s, ok := m["content"].(string); ok {
				return s
			}
		}
	case []map[string]any:
		for i := len(msgs) - 1; i >= 0; i-- {
			role, _ := msgs[i]["role"].(string)
			if role != "assistant" {
				continue
			}
			if s, ok := msgs[i]["content"].(string); ok {
				return s
			}
		}
	}
	return ""
}

// headlessErrorFromState 从后端事件状态中提取错误信息。
func headlessErrorFromState(state map[string]any) string {
	if state == nil {
		return "unknown error"
	}
	if cost, ok := state["cost"].(map[string]any); ok {
		if s, ok := cost["lastError"].(string); ok && s != "" {
			return s
		}
	}
	if s, ok := state["error"].(string); ok && s != "" {
		return s
	}
	return "unknown error"
}

// execOptions 保存 `exec` 子命令的解析结果。
type execOptions struct {
	local   bool
	docker  bool
	ssh     bool
	cmd     string
	image   string
	host    string
	user    string
	port    int
	workDir string
	env     []string
	timeout time.Duration
	dataDir string
}

// stringListValue 支持重复的 --env KEY=VALUE 参数。
type stringListValue []string

func (s *stringListValue) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListValue) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// parseExecArgs 解析 `agentforge exec` 子命令参数。
func parseExecArgs(args []string) (execOptions, error) {
	fs := flag.NewFlagSet("agentforge exec", flag.ContinueOnError)
	var opts execOptions
	fs.BoolVar(&opts.local, "local", false, "run locally")
	fs.BoolVar(&opts.docker, "docker", false, "run in a Docker container")
	fs.BoolVar(&opts.ssh, "ssh", false, "run over SSH")
	fs.StringVar(&opts.cmd, "cmd", "", "command line to execute")
	fs.StringVar(&opts.image, "image", "", "Docker image (required with --docker)")
	fs.StringVar(&opts.host, "host", "", "SSH host or user@host (required with --ssh)")
	fs.StringVar(&opts.user, "user", "", "SSH user (optional)")
	fs.IntVar(&opts.port, "port", 0, "SSH port (optional)")
	fs.StringVar(&opts.workDir, "workdir", "", "working directory")
	fs.Var((*stringListValue)(&opts.env), "env", "environment variable KEY=VALUE (repeatable)")
	fs.DurationVar(&opts.timeout, "timeout", 0, "execution timeout (e.g. 5s, 1m)")
	fs.StringVar(&opts.dataDir, "data-dir", config.DefaultDataDir, "本地数据目录（用于 SIEM/审计日志）")
	if err := fs.Parse(args); err != nil {
		return execOptions{}, err
	}
	if fs.NArg() > 0 {
		return execOptions{}, fmt.Errorf("exec: unexpected arguments: %v", fs.Args())
	}

	modes := 0
	if opts.local {
		modes++
	}
	if opts.docker {
		modes++
	}
	if opts.ssh {
		modes++
	}
	if modes != 1 {
		return execOptions{}, errors.New("exec: exactly one of --local, --docker, --ssh is required")
	}
	if strings.TrimSpace(opts.cmd) == "" {
		return execOptions{}, errors.New("exec: --cmd is required")
	}
	if opts.docker && strings.TrimSpace(opts.image) == "" {
		return execOptions{}, errors.New("exec: --image is required with --docker")
	}
	if opts.ssh && strings.TrimSpace(opts.host) == "" {
		return execOptions{}, errors.New("exec: --host is required with --ssh")
	}
	return opts, nil
}

// splitCommandLine 把命令行字符串拆成程序名与参数，支持单引号、双引号和反斜杠转义。
func splitCommandLine(s string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	started := false
	flush := func() {
		if started {
			fields = append(fields, cur.String())
			cur.Reset()
			started = false
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
			started = true
		case inDouble:
			if r == '"' {
				inDouble = false
			} else if r == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\' || runes[i+1] == '$' || runes[i+1] == '`') {
				i++
				cur.WriteRune(runes[i])
			} else {
				cur.WriteRune(r)
			}
			started = true
		case r == '\'':
			inSingle = true
			started = true
		case r == '"':
			inDouble = true
			started = true
		case r == '\\':
			if i+1 < len(runes) {
				i++
				cur.WriteRune(runes[i])
			} else {
				cur.WriteRune(r)
			}
			started = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inSingle || inDouble {
		return nil, errors.New("exec: unterminated quote in command")
	}
	flush()
	if len(fields) == 0 {
		return nil, errors.New("exec: empty command")
	}
	return fields, nil
}

// runExec 实现 `agentforge exec` 子命令（不指定企业身份时保持原有行为）。
func runExec(args []string) error {
	return runExecWithIdentity(args, "", "")
}

// runExecWithIdentity 在 exec 子命令前执行 RBAC 校验，并记录 SIEM 事件。
func runExecWithIdentity(args []string, identityUser, identityRoles string) (retErr error) {
	opts, err := parseExecArgs(args)
	if err != nil {
		return err
	}

	if err := checkExecRBAC(identityUser, identityRoles); err != nil {
		dir, _ := expandHome(opts.dataDir)
		emitSIEM(newSIEMSink(dir), identityUser, "exec.run", "exec", "denied")
		return err
	}

	dir, err := expandHome(opts.dataDir)
	if err != nil {
		return err
	}
	siemLog := newSIEMSink(dir)
	defer func() {
		result := "ok"
		if retErr != nil {
			result = "error"
		}
		emitSIEM(siemLog, identityUser, "exec.run", "exec", result)
	}()

	parts, err := splitCommandLine(opts.cmd)
	if err != nil {
		return err
	}

	spec := executor.Spec{
		Command: parts[0],
		Args:    parts[1:],
		WorkDir: opts.workDir,
		Env:     opts.env,
		Timeout: opts.timeout,
	}

	var ex executor.Executor
	switch {
	case opts.local:
		ex = executor.NewLocalExecutor()
	case opts.docker:
		ex = executor.NewDockerExecutor(opts.image)
	case opts.ssh:
		ex = executor.NewSSHExecutor(opts.host, opts.user, opts.port)
	}

	res, err := ex.Run(context.Background(), spec)
	if res.Stdout != "" {
		fmt.Print(res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	if err != nil {
		if res.ExitCode != 0 {
			return fmt.Errorf("exec: command failed with exit code %d", res.ExitCode)
		}
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

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
	})
	if err != nil {
		return err
	}

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
	return nil
}

// checkExecRBAC 校验模拟企业用户是否有权执行 exec。
//
// 未指定 --user 时默认允许；指定了 --user 但未指定 --roles 时也默认允许；
// 一旦提供角色，就使用 DefaultPolicy（可视为已配置策略）进行校验。
func checkExecRBAC(user, roles string) error {
	if strings.TrimSpace(user) == "" {
		return nil
	}
	roleNames := splitRoles(roles)
	if len(roleNames) == 0 {
		return nil
	}

	parsed := make([]enterprise.Role, 0, len(roleNames))
	for _, name := range roleNames {
		role, err := enterprise.ParseRole(name)
		if err != nil {
			return fmt.Errorf("exec: %w", err)
		}
		parsed = append(parsed, role)
	}

	rbac := enterprise.NewRBAC(enterprise.DefaultPolicy())
	u := &enterprise.User{ID: user, Name: user, Roles: parsed}
	if !rbac.Check(u, "exec.run") {
		return fmt.Errorf("exec: RBAC denied user %q (roles %s) from running commands", user, strings.Join(roleNames, ","))
	}
	return nil
}

// splitRoles 将逗号分隔的角色字符串拆分为去空白的角色名列表。
func splitRoles(roles string) []string {
	var out []string
	for _, part := range strings.Split(roles, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// runExport 实现 `agentforge export --out-dir <dir>`。
func runExport(args []string) error {
	fs := flag.NewFlagSet("agentforge export", flag.ContinueOnError)
	outDir := fs.String("out-dir", "", "导出目录")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("export: unexpected arguments: %v", fs.Args())
	}
	if *outDir == "" {
		return errors.New("export: --out-dir is required")
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	out, err := expandHome(*outDir)
	if err != nil {
		return err
	}

	paths, err := session.ExportAll(dir, out)
	if err != nil {
		return err
	}

	fmt.Printf("exported %d session(s) to %s\n", len(paths), out)
	for _, path := range paths {
		fmt.Println(path)
	}
	return nil
}

// runImport 实现 `agentforge import --in-dir <dir>`。
func runImport(args []string) error {
	fs := flag.NewFlagSet("agentforge import", flag.ContinueOnError)
	inDir := fs.String("in-dir", "", "导入目录")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("import: unexpected arguments: %v", fs.Args())
	}
	if *inDir == "" {
		return errors.New("import: --in-dir is required")
	}

	in, err := expandHome(*inDir)
	if err != nil {
		return err
	}
	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}

	results, err := session.ImportAll(in, dir)
	if err != nil {
		return err
	}

	imported, skipped := 0, 0
	for _, result := range results {
		fmt.Println(result)
		if strings.HasPrefix(result, "imported ") {
			imported++
		} else {
			skipped++
		}
	}
	fmt.Printf("imported %d, skipped %d\n", imported, skipped)
	return nil
}

// runShare 实现 `agentforge share --session <id> [--key <key>]`。
func runShare(args []string) error {
	fs := flag.NewFlagSet("agentforge share", flag.ContinueOnError)
	sessionID := fs.String("session", "", "要分享的会话 ID")
	key := fs.String("key", "", "AES-GCM 分享密钥（hex/base64；为空时读取 DSH_SHARE_KEY）")
	shareKey := fs.String("share-key", "", "AES-GCM 分享密钥（--key 的别名）")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	noAudit := fs.Bool("no-audit", false, "禁用审计日志（默认开启）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("share: unexpected arguments: %v", fs.Args())
	}
	if *sessionID == "" {
		return errors.New("share: --session is required")
	}
	if *key != "" && *shareKey != "" && *key != *shareKey {
		return errors.New("share: --key and --share-key conflict")
	}
	if *key == "" {
		*key = *shareKey
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	store, err := session.NewStore(dir)
	if err != nil {
		return err
	}
	entry, err := store.Load(*sessionID)
	if err != nil {
		return fmt.Errorf("share: load session %s: %w", *sessionID, err)
	}

	secretKey, err := resolveShareKey(*key)
	if err != nil {
		return err
	}
	link, err := share.ShareSession(*entry, secretKey)
	if err != nil {
		return err
	}
	audit(newAuditLogger(dir, !*noAudit), "session.share", map[string]any{"sessionId": *sessionID})
	fmt.Println(link)
	return nil
}

// runOpen 实现 `agentforge open --url <dsh-share://...> [--key <key>]`。
func runOpen(args []string) error {
	fs := flag.NewFlagSet("agentforge open", flag.ContinueOnError)
	shareURL := fs.String("url", "", "分享链接，例如 dsh-share://...")
	key := fs.String("key", "", "AES-GCM 分享密钥（hex/base64；为空时读取 DSH_SHARE_KEY）")
	shareKey := fs.String("share-key", "", "AES-GCM 分享密钥（--key 的别名）")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	noAudit := fs.Bool("no-audit", false, "禁用审计日志（默认开启）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("open: unexpected arguments: %v", fs.Args())
	}
	if *shareURL == "" {
		return errors.New("open: --url is required")
	}
	if *key != "" && *shareKey != "" && *key != *shareKey {
		return errors.New("open: --key and --share-key conflict")
	}
	if *key == "" {
		*key = *shareKey
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	secretKey, err := resolveShareKey(*key)
	if err != nil {
		return err
	}
	entry, err := share.OpenShare(*shareURL, secretKey)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		return errors.New("open: shared session has empty id")
	}

	store, err := session.NewStore(dir)
	if err != nil {
		return err
	}
	if _, err := store.Load(entry.ID); err == nil {
		fmt.Printf("skipped %s: already exists locally\n", entry.ID)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("open: load existing %s: %w", entry.ID, err)
	}

	if err := store.Save(*entry); err != nil {
		return fmt.Errorf("open: save %s: %w", entry.ID, err)
	}
	audit(newAuditLogger(dir, !*noAudit), "session.import", map[string]any{"sessionId": entry.ID})
	fmt.Printf("imported %s\n", entry.ID)
	return nil
}

// resolveShareKey 解析分享密钥；未提供 key 且环境变量为空时返回 nil（明文模式）。
func resolveShareKey(key string) ([]byte, error) {
	if key == "" {
		key = os.Getenv(share.EnvKey)
	}
	if key == "" {
		return nil, nil
	}
	secretKey, err := security.LoadKey(key)
	if err != nil {
		return nil, fmt.Errorf("share: load key: %w", err)
	}
	return secretKey, nil
}

// runSync 实现 `agentforge sync --from <dir> --to <dir>`。
func runSync(args []string) error {
	fs := flag.NewFlagSet("agentforge sync", flag.ContinueOnError)
	from := fs.String("from", "", "源数据目录")
	to := fs.String("to", "", "目标数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("sync: unexpected arguments: %v", fs.Args())
	}
	if *from == "" || *to == "" {
		return errors.New("sync: --from and --to are required")
	}

	src, err := expandHome(*from)
	if err != nil {
		return err
	}
	dst, err := expandHome(*to)
	if err != nil {
		return err
	}

	ops, err := session.SyncDir(src, dst)
	if err != nil {
		return err
	}

	ids := make([]string, 0, len(ops))
	for id := range ops {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Printf("synced %d session(s)\n", len(ops))
	for _, id := range ids {
		fmt.Printf("%s %s\n", id, ops[id])
	}
	return nil
}

// syncRemoteOptions 保存 `sync-remote` 子命令的解析结果。
type syncRemoteOptions struct {
	baseURL  string
	dataDir  string
	key      string
	push     bool
	pull     bool
	watch    bool
	interval time.Duration
}

// parseSyncRemoteArgs 解析 `sync-remote` 子命令参数。
func parseSyncRemoteArgs(args []string) (syncRemoteOptions, error) {
	fs := flag.NewFlagSet("agentforge sync-remote", flag.ContinueOnError)
	baseURL := fs.String("url", "", "远程存储 base URL，例如 https://host/sessions")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	key := fs.String("key", "", "AES-GCM 同步密钥（hex/base64；为空时读取 DSH_SYNC_KEY）")
	push := fs.Bool("push", false, "上传所有本地会话到远程")
	pull := fs.Bool("pull", false, "下载远程会话到本地")
	watch := fs.Bool("watch", false, "持续同步模式，按 --interval 定时执行")
	interval := fs.Duration("interval", 60*time.Second, "watch 模式的同步间隔")
	if err := fs.Parse(args); err != nil {
		return syncRemoteOptions{}, err
	}
	if fs.NArg() > 0 {
		return syncRemoteOptions{}, fmt.Errorf("sync-remote: unexpected arguments: %v", fs.Args())
	}

	opts := syncRemoteOptions{
		baseURL:  *baseURL,
		dataDir:  *dataDir,
		key:      *key,
		push:     *push,
		pull:     *pull,
		watch:    *watch,
		interval: *interval,
	}
	if opts.baseURL == "" {
		return syncRemoteOptions{}, errors.New("sync-remote: --url is required")
	}
	if !opts.watch && opts.push == opts.pull {
		return syncRemoteOptions{}, errors.New("sync-remote: exactly one of --push or --pull is required")
	}
	if opts.watch && !opts.push && !opts.pull {
		opts.push = true
		opts.pull = true
	}
	if opts.watch && opts.interval <= 0 {
		return syncRemoteOptions{}, errors.New("sync-remote: --interval must be positive")
	}
	return opts, nil
}

// newSyncRemoteStore 根据 --key / DSH_SYNC_KEY 构造普通或加密的 RemoteStore。
func newSyncRemoteStore(baseURL, key string) (session.RemoteStore, error) {
	var remote session.RemoteStore = session.NewHTTPStore(baseURL)
	if key != "" || os.Getenv("DSH_SYNC_KEY") != "" {
		enc, err := session.NewRemoteEncryptedStore(remote, key)
		if err != nil {
			return nil, err
		}
		remote = enc
	}
	return remote, nil
}

// syncOnce 执行一次远程同步。pull 在前、push 在后，便于 watch 默认“先拉后推”。
func syncOnce(ctx context.Context, dataDir string, remote session.RemoteStore, push, pull bool) error {
	if pull {
		ids, err := session.SyncFromRemote(ctx, dataDir, remote)
		if err != nil {
			return err
		}
		fmt.Printf("downloaded %d session(s)\n", len(ids))
		for _, id := range ids {
			fmt.Println(id)
		}
	}
	if push {
		ids, err := session.SyncToRemote(ctx, dataDir, remote)
		if err != nil {
			return err
		}
		fmt.Printf("uploaded %d session(s)\n", len(ids))
		for _, id := range ids {
			fmt.Println(id)
		}
	}
	return nil
}

// runSyncRemote 实现 `agentforge sync-remote --url <base> --data-dir <dir> [--key <hex>] [--push|--pull] [--watch] [--interval <dur>]`。
func runSyncRemote(args []string) error {
	opts, err := parseSyncRemoteArgs(args)
	if err != nil {
		return err
	}

	dir, err := expandHome(opts.dataDir)
	if err != nil {
		return err
	}

	remote, err := newSyncRemoteStore(opts.baseURL, opts.key)
	if err != nil {
		return err
	}

	if !opts.watch {
		return syncOnce(context.Background(), dir, remote, opts.push, opts.pull)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("sync-remote: watching %s every %s (push=%v pull=%v); press Ctrl+C to stop\n", opts.baseURL, opts.interval, opts.push, opts.pull)
	for {
		if err := syncOnce(ctx, dir, remote, opts.push, opts.pull); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Fprintln(os.Stderr, "agentforge: sync-remote:", err)
		}

		select {
		case <-ctx.Done():
			fmt.Println("sync-remote: stopped")
			return nil
		case <-time.After(opts.interval):
		}
	}
}

// expandHome 将路径开头的 ~ 展开为用户主目录。
func expandHome(path string) (string, error) {
	if path == "" || path == "~" {
		if path == "" {
			return path, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return home, nil
	}

	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

// resolveProviderKey 在 Provider 直连模式下解析 API Key。
//
// 当启用 --keychain，或提供了 --secret-key / DSH_SECRET_KEY 时，会优先从
// 系统 Keychain 或本地加密文件读取 --provider-key；如果命令行同时提供了
// --provider-key，则先写入对应安全存储再使用。
func resolveProviderKey(cfg *config.Config) error {
	if cfg == nil || cfg.Provider == "" {
		return nil
	}

	useSecret := cfg.Keychain || cfg.SecretKey != "" || os.Getenv(secret.EnvSecretKey) != ""
	if !useSecret {
		return nil
	}

	var store secret.Store
	var err error
	switch {
	case cfg.Keychain:
		store, err = secret.NewStore(cfg.DataDir, true)
	case cfg.SecretKey != "":
		store, err = secret.NewEncryptedFileStore(cfg.DataDir, cfg.SecretKey)
	default:
		store, err = secret.NewStore(cfg.DataDir, false)
	}
	if err != nil {
		return fmt.Errorf("init secret store: %w", err)
	}

	const service = "agentforge"
	if cfg.ProviderKey != "" {
		if err := store.Set(service, cfg.Provider, cfg.ProviderKey); err != nil {
			return fmt.Errorf("save provider key to secret store: %w", err)
		}
		return nil
	}

	got, err := store.Get(service, cfg.Provider)
	if err != nil {
		if errors.Is(err, secret.ErrNotFound) {
			return errors.New("provider key not found in secret store; run again with --provider-key to store it")
		}
		return fmt.Errorf("read provider key from secret store: %w", err)
	}
	cfg.ProviderKey = got
	return nil
}

// dial 根据配置连接 Provider 直连、真实 DSH Web API、真实 Channel 或内置 mock server。
// 返回的 cleanup 用于关闭 mock server（真实/Provider 连接时为 no-op）。
func dial(cfg *config.Config, store *session.Store) (ui.ChatBackend, func(), error) {
	if cfg.Provider != "" {
		p, err := provider.NewProvider(provider.ProviderOptions{
			Name:    cfg.Provider,
			APIKey:  cfg.ProviderKey,
			BaseURL: cfg.ProviderBaseURL,
			Model:   cfg.Model,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("init provider: %w", err)
		}
		return provider.NewBackendWithModel(p, store, cfg.Model), func() {}, nil
	}

	if cfg.APIURL != "" {
		api := dshapi.NewWithToken(cfg.APIURL, cfg.APIToken)
		return dshapi.NewBackend(api), func() {}, nil
	}

	if cfg.Mock {
		srv, err := mock.Start(context.Background(), "127.0.0.1:0")
		if err != nil {
			return nil, nil, fmt.Errorf("start mock server: %w", err)
		}

		endpoint := "ws://" + srv.Addr() + mock.DefaultPath
		cl, err := client.Dial(context.Background(), endpoint)
		if err != nil {
			_ = srv.Close()
			return nil, nil, fmt.Errorf("dial mock server %s: %w", endpoint, err)
		}
		return cl, func() { _ = srv.Close() }, nil
	}

	cl, err := client.Dial(context.Background(), cfg.Endpoint)
	if err != nil {
		return nil, nil, fmt.Errorf("dial %s: %w", cfg.Endpoint, err)
	}
	return cl, func() {}, nil
}

// warnIfDataExport 在直连国外厂商且未确认数据出境时打印提示。
func warnIfDataExport(cfg *config.Config) {
	if cfg == nil || cfg.ConfirmDataExport {
		return
	}
	if cfg.Provider == "openai" || cfg.Provider == "anthropic" || cfg.Provider == "gemini" {
		fmt.Fprintf(os.Stderr,
			"agentforge: warning: provider %q may send data outside your region; use --confirm-data-export to acknowledge\n",
			cfg.Provider)
	}
}

// warnIfInsecureRemote 在远程模式下对非 loopback 且未使用 https/wss 的
// 连接地址打印安全警告，提醒用户数据可能明文传输。
func warnIfInsecureRemote(cfg *config.Config) {
	if cfg == nil || !cfg.Remote {
		return
	}
	for _, raw := range []string{cfg.APIURL, cfg.Endpoint} {
		if raw == "" {
			continue
		}
		if isInsecureRemoteURL(raw) {
			fmt.Fprintf(os.Stderr,
				"agentforge: warning: remote address %q is not using https/wss and is not loopback; token and data may be transmitted insecurely\n",
				raw)
		}
	}
}

// isInsecureRemoteURL 判断 URL 是否属于“非 loopback 且非加密”的远程地址。
func isInsecureRemoteURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https", "wss":
		return false
	case "http", "ws":
		// fallthrough 到 host 判断
	default:
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}
