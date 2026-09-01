// Package server 提供 agentforge 本地 daemon 的 HTTP API。
//
// 它只监听 loopback 地址，供 Flutter 桌面 GUI（以及未来的 Windows/Linux/
// 手机前端）通过本地 HTTP/SSE 调用 Go 核心能力。
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/memory"
	"github.com/chronosvx/agentforge/internal/session"
)

// DefaultAddr 是 daemon 默认监听地址。
const DefaultAddr = "127.0.0.1:48321"

// Server 是本地 daemon 的 HTTP 服务。
type Server struct {
	cfg       *config.Config
	store     session.LocalStore
	memory    *memory.MemoryStore
	token     string
	version   string
	runs      *RunManager
	startedAt time.Time
}

// Options 是创建 Server 的参数。
type Options struct {
	Config  *config.Config
	Store   session.LocalStore
	Memory  *memory.MemoryStore
	Token   string
	Version string
}

// New 创建一个 Server。
func New(opts Options) (*Server, error) {
	if opts.Config == nil {
		return nil, errors.New("server: config is required")
	}
	if opts.Store == nil {
		return nil, errors.New("server: session store is required")
	}
	if opts.Memory == nil {
		ms, err := memory.NewStore(opts.Config.DataDir)
		if err != nil {
			return nil, fmt.Errorf("server: memory store: %w", err)
		}
		opts.Memory = ms
	}
	version := opts.Version
	if version == "" {
		version = "dev"
	}
	return &Server{
		cfg:       opts.Config,
		store:     opts.Store,
		memory:    opts.Memory,
		token:     opts.Token,
		version:   version,
		runs:      NewRunManager(),
		startedAt: time.Now(),
	}, nil
}

// Handler 返回带鉴权和路由的 HTTP Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/agent/runs", s.handleAgentRuns)
	mux.HandleFunc("/api/agent/runs/", s.handleAgentRunByID)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/memory", s.handleMemory)
	return s.auth(mux)
}

// ListenAndServe 启动 daemon，阻塞直到 ctx 取消或服务出错。
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	if strings.TrimSpace(addr) == "" {
		addr = DefaultAddr
	}
	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.Handler(),
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("agentforge daemon listening on http://%s", addr)
		errCh <- httpServer.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

// Token 返回当前 daemon 的访问令牌。
func (s *Server) Token() string {
	return s.token
}

// LoadOrCreateToken 从 dataDir 加载或生成 daemon 访问令牌。
func LoadOrCreateToken(dataDir, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	path := filepath.Join(dataDir, "serve.token")
	if data, err := os.ReadFile(path); err == nil {
		token := strings.TrimSpace(string(data))
		if token != "" {
			return token, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("server: generate token: %w", err)
	}
	token := hex.EncodeToString(buf)
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("server: create data dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("server: write token: %w", err)
	}
	return token, nil
}

// auth 包装所有 /api 路由，校验 Bearer token。
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.EqualFold(auth, "Bearer "+s.token) {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"error": "unauthorized",
				})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSON(w, status, map[string]any{"error": fmt.Sprintf(format, args...)})
}
