package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/agentflow"
	"github.com/chronosvx/agentforge/internal/provider"
	"github.com/chronosvx/agentforge/internal/repomode"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"version":    s.version,
		"data_dir":   s.cfg.DataDir,
		"started_at": s.startedAt,
	})
}

func (s *Server) handleAgentRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"runs": s.runs.List()})
	case http.MethodPost:
		var req AgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON: %v", err)
			return
		}
		if strings.TrimSpace(req.WorkDir) == "" {
			req.WorkDir = "."
		}
		if strings.TrimSpace(req.Planner) == "" {
			req.Planner = "static"
		}
		if strings.TrimSpace(req.TestCmd) == "" {
			req.TestCmd = "go test ./..."
		}
		if req.Timeout <= 0 {
			req.Timeout = 5 * time.Minute
		}
		runner, err := s.newRunner(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "%v", err)
			return
		}
		// 使用 Background 而不是 r.Context()：POST 返回后请求上下文会取消，
		// 但 Agent 运行应继续在 daemon 中执行。
		run, err := s.runs.Start(context.Background(), runner, req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "%v", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         run.ID,
			"started_at": run.StartedAt,
		})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAgentRunByID(w http.ResponseWriter, r *http.Request) {
	// 路径形如 /api/agent/runs/<id> 或 /api/agent/runs/<id>/events
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/agent/runs/")
	id := trimmed
	events := false
	if strings.HasSuffix(trimmed, "/events") {
		events = true
		id = strings.TrimSuffix(trimmed, "/events")
	}
	if id == "" {
		writeError(w, http.StatusBadRequest, "run id is required")
		return
	}
	run := s.runs.Get(id)
	if run == nil {
		writeError(w, http.StatusNotFound, "run %s not found", id)
		return
	}
	if events {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.streamRunEvents(w, r, run)
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, run.Snapshot())
}

func (s *Server) streamRunEvents(w http.ResponseWriter, r *http.Request, run *Run) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel, err := s.runs.Subscribe(run.ID)
	if err != nil {
		return
	}
	defer cancel()

	// 先补发历史事件，避免订阅前已产生的事件丢失。
	run.mu.Lock()
	history := append([]agentflow.Event(nil), run.Events...)
	done := run.Done
	run.mu.Unlock()
	for _, ev := range history {
		if err := writeSSE(w, flusher, ev); err != nil {
			return
		}
	}
	if done {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, flusher, ev); err != nil {
				return
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, ev agentflow.Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	entries, err := s.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": entries})
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx := r.Context()
	project := strings.TrimSpace(r.URL.Query().Get("project"))
	if project != "" {
		entries, err := s.memory.List(ctx, project)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "%v", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"projects": []string{project}, "entries": entries})
		return
	}

	projects, err := s.memory.Projects(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "%v", err)
		return
	}
	all := make([]map[string]any, 0)
	for _, project := range projects {
		entries, err := s.memory.List(ctx, project)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			all = append(all, map[string]any{
				"id":         entry.ID,
				"project":    entry.Project,
				"key":        entry.Key,
				"content":    entry.Content,
				"tags":       entry.Tags,
				"updated_at": entry.UpdatedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects, "entries": all})
}

// newRunner 根据 AgentRequest 构建 agentflow.Runner。
// 为了保持 daemon 轻量，这里只支持 static 与 llm 两种规划器。
func (s *Server) newRunner(ctx context.Context, req AgentRequest) (*agentflow.Runner, error) {
	var planner agentflow.Planner
	switch strings.ToLower(strings.TrimSpace(req.Planner)) {
	case "", "static":
		planner = agentflow.NewStaticPlanner()
	case "llm":
		if strings.TrimSpace(req.Provider) == "" {
			req.Provider = s.cfg.Provider
		}
		if strings.TrimSpace(req.ProviderKey) == "" {
			req.ProviderKey = s.cfg.ProviderKey
		}
		if strings.TrimSpace(req.BaseURL) == "" {
			req.BaseURL = s.cfg.ProviderBaseURL
		}
		if strings.TrimSpace(req.Model) == "" {
			req.Model = s.cfg.Model
		}
		if strings.TrimSpace(req.Provider) == "" {
			return nil, fmt.Errorf("provider is required with planner llm")
		}
		prov, err := provider.NewProvider(provider.ProviderOptions{
			Name:    req.Provider,
			APIKey:  req.ProviderKey,
			BaseURL: req.BaseURL,
			Model:   req.Model,
		})
		if err != nil {
			return nil, fmt.Errorf("provider: %w", err)
		}
		model := strings.TrimSpace(req.Model)
		if model == "" {
			model = prov.Model()
		}
		if model == "" {
			return nil, fmt.Errorf("provider did not provide a default model")
		}
		planner = agentflow.NewLLMPlanner(prov, model)
	default:
		return nil, fmt.Errorf("unknown planner %q", req.Planner)
	}

	if req.RepoMode {
		info, err := repomode.Scan(ctx, req.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("scan repo: %w", err)
		}
		planner = repoAwarePlanner{base: planner, prompt: req.Prompt, repo: buildRepoContext(info)}
	}

	runner := agentflow.NewRunner(planner, nil)
	if !req.AutoApprove {
		// GUI 第一版不开放审批流；未开启自动批准时直接拒绝，避免 daemon 挂起。
		runner.Approve = func(agent.Action) (bool, error) {
			return false, fmt.Errorf("approval not supported by GUI yet")
		}
	}
	return runner, nil
}

// repoAwarePlanner 在保留原始 prompt 的同时，把仓库上下文交给底层规划器。
type repoAwarePlanner struct {
	base   agentflow.Planner
	prompt string
	repo   string
}

func (p repoAwarePlanner) Plan(ctx context.Context, _ string, repo *agentflow.Repo) ([]agent.Action, error) {
	return p.base.Plan(ctx, p.repo+"\n\n"+p.prompt, repo)
}

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
