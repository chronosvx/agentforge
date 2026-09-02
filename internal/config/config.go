// Package config 负责命令行参数解析与运行配置。
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// DefaultEndpoint 是 DSH Channel 服务的默认 WebSocket 地址。
	DefaultEndpoint = "ws://127.0.0.1:18891/agentforge/v1/connection"
	// DefaultDataDir 是默认数据目录，Parse 时会展开其中的 ~。
	DefaultDataDir = "~/.agentforge"
)

// supportedThemes 是 Theme 字段支持的取值。
var supportedThemes = map[string]bool{
	"default":     true,
	"catppuccin":  true,
	"dracula":     true,
	"tokyo-night": true,
	"nord":        true,
	"solarized":   true,
}

// Config 保存 agentforge 运行所需的配置。
type Config struct {
	Endpoint          string  // DSH Channel WebSocket 地址
	APIURL            string  // DSH Web API 地址（http://127.0.0.1:port）
	APIToken          string  // DSH Web API 访问令牌（Authorization: Bearer <token>）
	Remote            bool    // 为 true 时启用远程模式
	Workspace         string  // 工作区名称
	SessionID         string  // 通过 --resume 指定的会话 ID
	DataDir           string  // 本地数据目录
	Mock              bool    // 为 true 时连接内置 mock server
	CostLimit         float64 // 成本告警阈值（元），0 表示关闭
	Theme             string  // 主题：default / catppuccin / dracula / tokyo-night / nord / solarized
	Lang              string  // 界面语言：zh / en
	Telemetry         bool    // 为 true 时启用本地遥测（默认关闭）
	Version           bool    // 仅打印版本并退出
	Provider          string  // 直连厂商名：openai/deepseek/moonshot/kimi/zhipu/qwen/ollama
	ProviderKey       string  // 直连厂商 API Key，多个用逗号分隔
	ProviderKeyStdin  bool    // 为 true 时从 stdin 读取 Provider Key，避免出现在命令行
	ProviderBaseURL   string  // 直连厂商 OpenAI 兼容 Base URL（可选）
	Model             string  // 直连厂商模型名（可选）
	Keychain          bool    // 为 true 时优先使用系统 Keychain 保存/读取 --provider-key
	EncryptSessions   bool    // 为 true 时本地会话使用 AES-GCM 加密存储
	SecretKey         string  // EncryptedFileStore 主密钥（hex/base64），也可用 DSH_SECRET_KEY
	ConfirmDataExport bool    // 为 true 时确认允许数据出境
	NoAudit           bool    // 为 true 时禁用审计日志
	NoColor           bool    // 为 true 时禁用彩色输出
	User              string  // 模拟企业用户身份（可选）
	Roles             string  // 模拟用户角色，逗号分隔（可选）
}

// Parse 使用标准库 flag 解析命令行参数，并展开 DataDir 中的 ~。
func Parse(args []string) (*Config, error) {
	cfg := &Config{
		Endpoint: DefaultEndpoint,
		DataDir:  DefaultDataDir,
		Lang:     "zh",
	}

	var configPath string

	fs := flag.NewFlagSet("agentforge", flag.ContinueOnError)
	fs.StringVar(&configPath, "config", "", "配置文件路径（JSON 或 key=value）")
	fs.StringVar(&cfg.Endpoint, "endpoint", cfg.Endpoint, "DSH Channel WebSocket 地址")
	fs.StringVar(&cfg.APIURL, "api-url", "", "DSH Web API 地址（例如 http://127.0.0.1:54695）")
	fs.StringVar(&cfg.APIToken, "api-token", "", "DSH Web API 访问令牌（附加 Authorization: Bearer <token>）")
	fs.BoolVar(&cfg.Remote, "remote", false, "启用远程模式（连接非本机 DSH 时使用）")
	fs.StringVar(&cfg.Workspace, "workspace", "", "工作区名称")
	fs.StringVar(&cfg.SessionID, "resume", "", "恢复指定会话 ID")
	fs.StringVar(&cfg.DataDir, "data-dir", cfg.DataDir, "本地数据目录（支持 ~ 展开）")
	fs.BoolVar(&cfg.Mock, "mock", false, "连接内置 mock server 而不是真实 DSH")
	fs.Float64Var(&cfg.CostLimit, "cost-limit", 0, "成本告警阈值（元），0 表示关闭")
	fs.StringVar(&cfg.Theme, "theme", "default", "主题：default / catppuccin / dracula / tokyo-night / nord / solarized")
	fs.StringVar(&cfg.Lang, "lang", cfg.Lang, "界面语言：zh / en")
	fs.BoolVar(&cfg.Telemetry, "telemetry", false, "启用本地遥测（写入 <data-dir>/telemetry.log，默认关闭）")
	fs.BoolVar(&cfg.Version, "version", false, "打印版本并退出")
	fs.StringVar(&cfg.Provider, "provider", "", "直连厂商名：openai/deepseek/moonshot/kimi/zhipu/qwen/ollama")
	fs.StringVar(&cfg.ProviderKey, "provider-key", "", "直连厂商 API Key，多个用逗号分隔")
	fs.BoolVar(&cfg.ProviderKeyStdin, "provider-key-stdin", false, "从 stdin 读取 Provider Key，避免出现在命令行")
	fs.StringVar(&cfg.ProviderBaseURL, "provider-base-url", "", "直连厂商 OpenAI 兼容 Base URL（可选）")
	fs.StringVar(&cfg.Model, "model", "", "直连厂商模型名（可选）")
	fs.BoolVar(&cfg.Keychain, "keychain", false, "优先使用系统 Keychain/凭据管理器保存/读取 --provider-key")
	fs.BoolVar(&cfg.EncryptSessions, "encrypt-sessions", false, "使用 AES-GCM 加密本地会话存储（需要 --secret-key 或 DSH_SECRET_KEY）")
	fs.StringVar(&cfg.SecretKey, "secret-key", "", "EncryptedFileStore 主密钥（hex/base64；为空时读取 DSH_SECRET_KEY）")
	fs.BoolVar(&cfg.ConfirmDataExport, "confirm-data-export", false, "确认允许数据出境（连接国外厂商时提示）")
	fs.BoolVar(&cfg.NoAudit, "no-audit", false, "禁用审计日志（默认开启）")
	fs.BoolVar(&cfg.NoColor, "no-color", false, "禁用彩色输出（默认 false）")
	fs.StringVar(&cfg.User, "user", "", "模拟企业用户身份（可选）")
	fs.StringVar(&cfg.Roles, "roles", "", "模拟用户角色，逗号分隔（可选）")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	// 分层配置：默认值 < 用户配置 < 项目配置 < 环境变量 < CLI。
	paths := []string{}
	if configPath != "" {
		paths = append(paths, configPath)
	} else {
		paths = append(paths, DefaultPaths()...)
	}
	for _, p := range paths {
		kv, err := LoadFile(p)
		if err != nil {
			return nil, err
		}
		applyConfig(cfg, kv, setFlags)
	}
	envKV := map[string]string{}
	for _, kv := range os.Environ() {
		if idx := strings.IndexByte(kv, '='); idx > 0 && strings.HasPrefix(kv, "DSH_") {
			k := strings.ToLower(strings.ReplaceAll(kv[4:idx], "_", "-"))
			envKV[k] = kv[idx+1:]
		}
	}
	applyConfig(cfg, envKV, setFlags)

	if fs.NArg() > 0 {
		return nil, fmt.Errorf("config: unexpected arguments: %v", fs.Args())
	}

	if cfg.Lang != "zh" && cfg.Lang != "en" {
		return nil, fmt.Errorf("config: invalid --lang %q (supported: zh, en)", cfg.Lang)
	}

	if !supportedThemes[cfg.Theme] {
		return nil, fmt.Errorf("config: invalid --theme %q (supported: default, catppuccin, dracula, tokyo-night, nord, solarized)", cfg.Theme)
	}

	dataDir, err := expandHome(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	cfg.DataDir = dataDir

	return cfg, nil
}

// expandHome 将路径开头的 ~ 展开为用户主目录。
func expandHome(path string) (string, error) {
	if path == "" || path == "~" {
		if path == "" {
			return path, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: resolve home directory: %w", err)
		}
		return home, nil
	}

	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: resolve home directory: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}
