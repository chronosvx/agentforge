package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/server"
	"github.com/chronosvx/agentforge/internal/session"
)

// runServe 实现 `agentforge serve`：启动本地 daemon，供桌面 GUI 连接。
func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", server.DefaultAddr, "监听地址（仅建议 loopback）")
	token := fs.String("token", "", "访问令牌；为空时自动生成并保存到 <data-dir>/serve.token")
	dataDir := fs.String("data-dir", config.DefaultDataDir, "本地数据目录")
	providerName := fs.String("provider", "", "默认 LLM 提供商（GUI 未指定时使用）")
	providerKey := fs.String("provider-key", "", "默认 Provider API Key")
	baseURL := fs.String("base-url", "", "默认 OpenAI 兼容 Base URL")
	model := fs.String("model", "", "默认模型")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("serve: unexpected arguments: %v", fs.Args())
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("serve: create data dir: %w", err)
	}

	accessToken, err := server.LoadOrCreateToken(dir, *token)
	if err != nil {
		return err
	}

	store, err := session.NewStore(dir)
	if err != nil {
		return fmt.Errorf("serve: session store: %w", err)
	}

	cfg := &config.Config{
		DataDir:         dir,
		Provider:        *providerName,
		ProviderKey:     *providerKey,
		ProviderBaseURL: *baseURL,
		Model:           *model,
	}
	srv, err := server.New(server.Options{
		Config:  cfg,
		Store:   store,
		Token:   accessToken,
		Version: version,
	})
	if err != nil {
		return err
	}

	if *token == "" {
		fmt.Printf("agentforge daemon token: %s\n", accessToken)
		fmt.Printf("token file: %s\n", dir+"/serve.token")
	}
	fmt.Printf("agentforge daemon listening on http://%s\n", *addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return srv.ListenAndServe(ctx, *addr)
}
