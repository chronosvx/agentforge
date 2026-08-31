// Package api 提供开放 HTTP API 服务，便于外部程序/CI 调用。
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/session"
)

// Server 是开放 API 的 HTTP handler。
type Server struct {
	store           session.LocalStore
	token           string
	PromptFunc      func(ctx context.Context, sessionID, text string) (string, error)
	OnAgentApproval func(approval any)
	mux             *http.ServeMux

	// prompt 是 PromptFunc 的兼容别名，供旧代码/同包测试直接赋值。
	prompt func(ctx context.Context, sessionID, text string) (string, error)
}

// NewServer 创建 API Server。
func NewServer(store session.LocalStore, token string, prompt func(ctx context.Context, sessionID, text string) (string, error)) *Server {
	s := &Server{
		store:      store,
		token:      token,
		PromptFunc: prompt,
		prompt:     prompt,
		mux:        http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /v1/healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("POST /v1/sessions/{id}/prompt", s.handlePrompt)
	return s
}

// FireAgentApproval 触发 Agent 审批事件；上层可把它接到 Webhook Notifier。
func (s *Server) FireAgentApproval(approval any) {
	if s != nil && s.OnAgentApproval != nil {
		s.OnAgentApproval(approval)
	}
}

func (s *Server) promptFunc() func(ctx context.Context, sessionID, text string) (string, error) {
	if s.PromptFunc != nil {
		return s.PromptFunc
	}
	return s.prompt
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.token != "" && r.Header.Get("Authorization") != "Bearer "+s.token {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.ID == "" {
		req.ID = "session-" + time.Now().Format("20060102150405.000000000")
	}
	if s.store != nil {
		if err := s.store.Save(session.Entry{ID: req.ID}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": req.ID})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.store == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "session store disabled"})
		return
	}
	entry, err := s.store.Load(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "text is required"})
		return
	}
	prompt := s.promptFunc()
	if prompt == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{"error": "prompt handler not configured"})
		return
	}
	result, err := prompt(r.Context(), id, req.Text)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "result": result})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
