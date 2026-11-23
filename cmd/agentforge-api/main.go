// Command agentforge-api 启动开放 HTTP API 服务。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/api"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/chronosvx/agentforge/internal/mock"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/session"
)

// promptBackend 是 agentforge-api 需要的聊天后端子集。
// internal/provider.Backend、internal/dshapi.Backend 与 internal/client.Client
// 都实现该接口，因此 API server 可以复用 TUI 的同一套真实后端。
type promptBackend interface {
	Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*client.Snapshot, error)
	Subscribe(ctx context.Context, channelID string, after int64) (<-chan client.Event, error)
	Invoke(ctx context.Context, channelID, method string, args []any) (any, error)
	Close(ctx context.Context, channelID string) error
	CloseConn() error
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8787", "HTTP listen address")
	dataDir := flag.String("data-dir", config.DefaultDataDir, "data directory")
	token := flag.String("api-token", "", "Bearer token for API auth")
	webhookURL := flag.String("webhook-url", "", "webhook URL to notify")
	webhookSecret := flag.String("webhook-secret", "", "webhook secret")

	providerName := flag.String("provider", "", "direct provider name: openai/deepseek/moonshot/kimi/zhipu/qwen/ollama")
	providerKey := flag.String("provider-key", "", "direct provider API key (comma separated for multiple keys)")
	providerBaseURL := flag.String("provider-base-url", "", "direct provider OpenAI-compatible base URL")
	model := flag.String("model", "", "model name for direct provider")
	apiURL := flag.String("api-url", "", "real DSH Web API base URL")
	dshAPIToken := flag.String("dsh-api-token", "", "upstream DSH Web API token")
	endpoint := flag.String("endpoint", "", "DSH TUI Channel WebSocket endpoint (optional)")
	mockMode := flag.Bool("mock", false, "use built-in mock server")
	workspace := flag.String("workspace", ".", "default workspace for prompt sessions")
	flag.Parse()

	store, err := session.NewStore(*dataDir)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	backend, cleanup, err := buildBackend(*providerName, *providerKey, *providerBaseURL, *model, *apiURL, *dshAPIToken, *endpoint, *mockMode, store)
	if err != nil {
		log.Fatalf("init backend: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if backend == nil {
		log.Printf("warning: no real backend configured (--provider, --api-url, --mock, or --endpoint); using echo fallback")
	}

	prompt := func(ctx context.Context, sessionID, text string) (string, error) {
		if backend == nil {
			return "echo: " + text, nil
		}
		return runPrompt(ctx, backend, *workspace, sessionID, text)
	}

	srv := api.NewServer(store, *token, prompt)
	notifier := &api.Notifier{Webhook: &api.Webhook{URL: *webhookURL, Secret: *webhookSecret}}
	srv.OnAgentApproval = func(approval any) {
		notifier.Notify("agent.approval.required", approval)
	}

	httpSrv := &http.Server{Addr: *listen, Handler: srv}
	log.Printf("agentforge-api listening on %s", *listen)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// buildBackend 根据命令行参数创建真实聊天后端。
// 优先级：--provider > --api-url > --mock > --endpoint > nil(echo)。
func buildBackend(providerName, providerKey, providerBaseURL, model, apiURL, dshAPIToken, endpoint string, mockMode bool, store session.LocalStore) (promptBackend, func(), error) {
	if providerName != "" {
		p, err := provider.NewProvider(provider.ProviderOptions{
			Name:    providerName,
			APIKey:  providerKey,
			BaseURL: providerBaseURL,
			Model:   model,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("init provider: %w", err)
		}
		return provider.NewBackendWithModel(p, store, model), nil, nil
	}

	if apiURL != "" {
		apiClient := dshapi.NewWithToken(apiURL, dshAPIToken)
		return dshapi.NewBackend(apiClient), nil, nil
	}

	if mockMode {
		srv, err := mock.Start(context.Background(), "127.0.0.1:0")
		if err != nil {
			return nil, nil, fmt.Errorf("start mock server: %w", err)
		}
		cl, err := client.Dial(context.Background(), "ws://"+srv.Addr()+mock.DefaultPath)
		if err != nil {
			_ = srv.Close()
			return nil, nil, fmt.Errorf("dial mock server: %w", err)
		}
		return cl, func() {
			_ = cl.CloseConn()
			_ = srv.Close()
		}, nil
	}

	if endpoint != "" {
		cl, err := client.Dial(context.Background(), endpoint)
		if err != nil {
			return nil, nil, fmt.Errorf("dial endpoint: %w", err)
		}
		return cl, func() { _ = cl.CloseConn() }, nil
	}

	return nil, nil, nil
}

// runPrompt 通过真实后端发送 prompt，并等待最终 assistant 结果。
func runPrompt(ctx context.Context, backend promptBackend, workspace, sessionID, text string) (string, error) {
	snap, err := backend.Open(ctx, workspace, sessionID, nil)
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	defer func() {
		_ = backend.Close(context.Background(), snap.ChannelID)
	}()

	events, err := backend.Subscribe(ctx, snap.ChannelID, snap.Version)
	if err != nil {
		return "", fmt.Errorf("subscribe session: %w", err)
	}
	if _, err := backend.Invoke(ctx, snap.ChannelID, "chat", []any{text}); err != nil {
		return "", fmt.Errorf("invoke prompt: %w", err)
	}

	content, err := waitForPromptResult(ctx, events, snap.ChannelID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("prompt: backend returned empty result")
	}
	return content, nil
}

// waitForPromptResult 消费事件流直到完成。Provider 后端会发送 done 事件；
// DSH API 等后端没有 done 事件时，在最后一条增量后短暂静默即视为完成。
func waitForPromptResult(ctx context.Context, events <-chan client.Event, channelID string) (string, error) {
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
				return "", fmt.Errorf("prompt: event stream for %s closed before completion", channelID)
			}
			if ev.Operation == "error" {
				return "", fmt.Errorf("prompt: backend error: %s", promptErrorFromState(ev.State))
			}
			content = promptContentFromState(ev.State)
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
			idle.Reset(idleTimeout)
		}
	}
}

// promptContentFromState 从后端事件状态中提取最后一条 assistant 消息。
func promptContentFromState(state map[string]any) string {
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

// promptErrorFromState 从后端事件状态中提取错误信息。
func promptErrorFromState(state map[string]any) string {
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
