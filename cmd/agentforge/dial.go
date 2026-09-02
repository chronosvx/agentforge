package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/chronosvx/agentforge/internal/mock"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/ui"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// offlineQueueBackend 是支持离线队列的聊天后端可选接口。
type offlineQueueBackend interface {
	SetOfflineQueue(*provider.Queue)
	FlushOffline(context.Context) error
}

// setupOfflineQueue 为 Provider 直连后端挂载离线消息队列，并周期重发。
func setupOfflineQueue(cfg *config.Config, cl ui.ChatBackend, headless bool) {
	if cfg == nil || cl == nil {
		return
	}
	ob, ok := cl.(offlineQueueBackend)
	if !ok {
		return
	}
	q := provider.NewQueue(filepath.Join(cfg.DataDir, "offline.json"))
	ob.SetOfflineQueue(q)
	if headless {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = ob.FlushOffline(ctx)
			}
		}
	}()
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

	if cfg.ProviderKeyStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read provider key from stdin: %w", err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return errors.New("provider key from stdin is empty")
		}
		cfg.ProviderKey = key
	}

	useSecret := cfg.Keychain || cfg.SecretKey != "" || os.Getenv(security.EnvSecretKey) != ""
	if !useSecret {
		return nil
	}

	var store security.Store
	var err error
	switch {
	case cfg.Keychain:
		store, err = security.NewStore(cfg.DataDir, true)
	case cfg.SecretKey != "":
		store, err = security.NewEncryptedFileStore(cfg.DataDir, cfg.SecretKey)
	default:
		store, err = security.NewStore(cfg.DataDir, false)
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
		if errors.Is(err, security.ErrNotFound) {
			return errors.New("provider key not found in secret store; run again with --provider-key to store it")
		}
		return fmt.Errorf("read provider key from secret store: %w", err)
	}
	cfg.ProviderKey = got
	return nil
}

// warnPlaintextSecrets 对通过命令行/明文配置提供的敏感信息输出安全提示。
func warnPlaintextSecrets(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if cfg.ProviderKey != "" && !cfg.ProviderKeyStdin && !cfg.Keychain {
		fmt.Fprintln(os.Stderr, "agentforge: warning: provider key provided as plaintext; prefer --provider-key-stdin or --keychain")
	}
	if cfg.APIToken != "" {
		fmt.Fprintln(os.Stderr, "agentforge: warning: api token provided as plaintext; prefer environment variable or keychain")
	}
	if cfg.SecretKey != "" {
		fmt.Fprintln(os.Stderr, "agentforge: warning: secret key provided as plaintext; prefer DSH_SECRET_KEY environment variable")
	}
}

// dial 根据配置连接 Provider 直连、真实 DSH Web API、真实 Channel 或内置 mock server。
// 返回的 cleanup 用于关闭 mock server（真实/Provider 连接时为 no-op）。
func dial(cfg *config.Config, store session.LocalStore) (ui.ChatBackend, func(), error) {
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

// insecureTarget 描述一个需要安全审查的连接地址。
type insecureTarget struct {
	kind          string
	raw           string
	hasCredential bool
}

// insecureRemoteTargets 返回配置中所有“非 loopback 且明文 HTTP/WS”的连接地址。
// 该检查不依赖 --remote：只要用户配置了远程明文地址就应提示/拦截。
func insecureRemoteTargets(cfg *config.Config) []insecureTarget {
	if cfg == nil {
		return nil
	}
	var out []insecureTarget
	if cfg.APIURL != "" && isInsecureRemoteURL(cfg.APIURL) {
		out = append(out, insecureTarget{kind: "api-url", raw: cfg.APIURL, hasCredential: cfg.APIToken != ""})
	}
	if cfg.Endpoint != "" && isInsecureRemoteURL(cfg.Endpoint) {
		out = append(out, insecureTarget{kind: "endpoint", raw: cfg.Endpoint})
	}
	if cfg.ProviderBaseURL != "" && isInsecureRemoteURL(cfg.ProviderBaseURL) {
		out = append(out, insecureTarget{kind: "provider-base-url", raw: cfg.ProviderBaseURL, hasCredential: cfg.ProviderKey != ""})
	}
	return out
}

// warnIfInsecureRemote 对非 loopback 且未使用 https/wss 的连接地址打印安全警告。
// 该函数只负责提示；强制拒绝由 checkInsecureRemote 处理。
func warnIfInsecureRemote(cfg *config.Config) {
	if cfg == nil {
		return
	}
	for _, t := range insecureRemoteTargets(cfg) {
		fmt.Fprintf(os.Stderr,
			"agentforge: warning: %s %q is not using https/wss and is not loopback; data may be transmitted insecurely\n",
			t.kind, t.raw)
	}
}

// checkInsecureRemote 对非 loopback 明文 HTTP/WS 执行安全检查。
//
// 无条件（不依赖 --remote）检测 api-url、endpoint 与 provider-base-url；
// 若连接会携带 token/key 且用户未显式设置 --allow-insecure-http，则返回错误，
// 默认拒绝该连接。没有凭据的明文远程连接仍输出高危警告，但不强制拒绝。
func checkInsecureRemote(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	targets := insecureRemoteTargets(cfg)
	if len(targets) == 0 {
		return nil
	}

	refused := false
	for _, t := range targets {
		switch {
		case cfg.AllowInsecureHTTP:
			fmt.Fprintf(os.Stderr,
				"agentforge: warning: %s %q is non-loopback plaintext; continuing because --allow-insecure-http is set\n",
				t.kind, t.raw)
		case t.hasCredential:
			fmt.Fprintf(os.Stderr,
				"agentforge: HIGH-RISK warning: refusing %s %q because credentials would be sent over non-loopback plaintext\n",
				t.kind, t.raw)
			refused = true
		default:
			fmt.Fprintf(os.Stderr,
				"agentforge: HIGH-RISK warning: %s %q is non-loopback plaintext; data may be transmitted insecurely\n",
				t.kind, t.raw)
		}
	}
	if refused {
		return fmt.Errorf("agentforge: refusing insecure non-loopback plaintext connection; use --allow-insecure-http only if you understand the risk")
	}
	return nil
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
