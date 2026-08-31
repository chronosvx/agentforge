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
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/telemetry"
	"github.com/chronosvx/agentforge/internal/ui"
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
		case "multiagent":
			return runMultiAgent(args[1:])
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

	setupOfflineQueue(cfg, cl, headless)

	if headless {
		return runHeadless(cl, cfg, prompt, siemLog)
	}

	maybeCheckUpdate()

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
