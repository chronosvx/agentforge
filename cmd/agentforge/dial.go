package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/chronosvx/agentforge/internal/mock"
	"github.com/chronosvx/agentforge/internal/offline"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/secret"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/ui"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// offlineQueueBackend 是支持离线队列的聊天后端可选接口。
type offlineQueueBackend interface {
	SetOfflineQueue(*offline.Queue)
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
	q := offline.NewQueue(filepath.Join(cfg.DataDir, "offline.json"))
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
