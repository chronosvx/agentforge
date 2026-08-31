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
	"github.com/chronosvx/agentforge/internal/secret"
	"github.com/chronosvx/agentforge/internal/security"
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

	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printRootUsage()
		return nil
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
		case "doctor":
			return runDoctor(args[1:])
		case "backup":
			return runBackup(args[1:])
		case "restore":
			return runRestore(args[1:])
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

	if !hasConfiguredBackend(cfg) {
		return errors.New("no backend configured\n\nTry one of:\n  agentforge --mock\n  agentforge --provider deepseek --provider-key sk-xxx\n  agentforge --api-url http://127.0.0.1:54695\n\nSee docs/QUICKSTART.md")
	}

	warnPlaintextSecrets(cfg)

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

	store, err := newSessionStore(cfg)
	if err != nil {
		return err
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

	latest := maybeCheckUpdate()

	model := ui.New(cfg, cl, store, func(event string, fields map[string]any) {
		audit(auditLog, event, fields)
	})
	model.SetUpdateAvailable(latest)
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

// newSessionStore 根据配置创建本地会话存储；启用 --encrypt-sessions 时使用 AES-GCM 加密。
func newSessionStore(cfg *config.Config) (session.LocalStore, error) {
	if !cfg.EncryptSessions {
		return session.NewStore(cfg.DataDir)
	}

	keySource := cfg.SecretKey
	if keySource == "" {
		keySource = os.Getenv(secret.EnvSecretKey)
	}
	if keySource == "" {
		return nil, errors.New("--encrypt-sessions requires --secret-key or DSH_SECRET_KEY")
	}
	key, err := security.LoadKey(keySource)
	if err != nil {
		return nil, fmt.Errorf("parse session encryption key: %w", err)
	}
	return session.NewEncryptedStore(cfg.DataDir, key)
}

// hasConfiguredBackend 判断用户是否显式配置了可用的后端。
func hasConfiguredBackend(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if cfg.Mock || cfg.Provider != "" || cfg.APIURL != "" {
		return true
	}
	return cfg.Endpoint != "" && cfg.Endpoint != config.DefaultEndpoint
}

// printRootUsage 打印根命令帮助，列出所有子命令。
func printRootUsage() {
	fmt.Print(`agentforge - terminal AI development agent

Usage:
  agentforge [flags]
  agentforge <command> [flags]

Commands:
  agent        Run the Agent main flow (plan -> execute -> diff -> test)
  multiagent   Run multiple Agent tasks in parallel
  exec         Execute commands locally, in Docker, or over SSH
  export       Export sessions to a directory
  import       Import sessions from a directory
  sync         Sync sessions between two directories
  sync-remote  Sync sessions with a remote HTTP/object storage backend
  share        Share a session as an encrypted link
  open         Open a shared session link
  doctor       Check the local environment for requirements
  backup       Backup the local data directory to a zip file
  restore      Restore the local data directory from a zip file

Flags:
  --mock              Run with the built-in mock backend
  --provider NAME     Use a direct model provider (deepseek, openai, ...)
  --provider-key KEY  Provider API key
  --api-url URL       Connect to a real DSH Web API
  --data-dir DIR      Local data directory (default ~/.agentforge)
  --lang zh|en        UI language (default zh)
  --version           Print version and exit
  --help              Show this help

Examples:
  agentforge --mock
  agentforge --provider deepseek --provider-key sk-xxx --keychain
  agentforge agent --prompt "add --version to main.go" --planner llm --repo --yes
  agentforge doctor

See docs/QUICKSTART.md for more.
`)
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
