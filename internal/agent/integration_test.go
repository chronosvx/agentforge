package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/contextx"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/session"
)

// TestAgentExecutionLoopWithAsyncStorePersistence 验证 Agent 执行闭环可以
// 与 AsyncStore 组合：Plan -> Approve -> MarkExecuted 后把结果异步落盘，
// Flush 后 Load 可以读回完整状态。
func TestAgentExecutionLoopWithAsyncStorePersistence(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	async := session.NewAsyncStore(store, 4)
	defer async.Close()

	sess := agent.NewAgentSession(agent.RequireApproval)
	sess.Plan([]agent.Action{
		{Type: agent.ActionTypeRunCommand, Description: "first command", Command: "echo hello"},
		{Type: agent.ActionTypeRunCommand, Description: "second command", Command: "echo world"},
	})

	executed := make(map[string]string, 2)
	for i := 0; i < 2; i++ {
		next := sess.NextPendingAction()
		if next == nil {
			t.Fatalf("NextPendingAction() = nil, want action %d", i)
		}
		if sess.Phase != agent.PhaseAwaitingApproval {
			t.Fatalf("after NextPendingAction phase = %q, want %q", sess.Phase, agent.PhaseAwaitingApproval)
		}

		if err := sess.Approve(next.ID); err != nil {
			t.Fatalf("approve %q: %v", next.ID, err)
		}

		next = sess.NextPendingAction()
		if next == nil {
			t.Fatalf("NextPendingAction() after approve = nil, want action %d", i)
		}
		if next.Status != agent.ActionStatusApproved {
			t.Fatalf("action status = %q, want %q", next.Status, agent.ActionStatusApproved)
		}

		output := fmt.Sprintf("output-%d", i)
		if err := sess.MarkExecuted(next.ID, output); err != nil {
			t.Fatalf("mark executed %q: %v", next.ID, err)
		}
		executed[next.ID] = output
	}

	if len(executed) != 2 {
		t.Fatalf("executed count = %d, want 2", len(executed))
	}
	for i, a := range sess.Actions {
		if a.Status != agent.ActionStatusExecuted {
			t.Fatalf("action[%d] status = %q, want %q", i, a.Status, agent.ActionStatusExecuted)
		}
		if a.Output != executed[a.ID] {
			t.Fatalf("action[%d] output = %q, want %q", i, a.Output, executed[a.ID])
		}
	}

	actions := make([]any, 0, len(sess.Actions))
	for _, a := range sess.Actions {
		actions = append(actions, map[string]any{
			"id":          a.ID,
			"type":        string(a.Type),
			"description": a.Description,
			"status":      string(a.Status),
			"output":      a.Output,
		})
	}
	entry := session.Entry{
		ID:        "agent-closed-loop",
		Title:     "Agent closed loop",
		UpdatedAt: time.Now(),
		Context: map[string]any{
			"agentPhase": string(sess.Phase),
			"actions":    actions,
		},
	}
	if err := async.Save(entry); err != nil {
		t.Fatalf("async save: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := async.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got, err := async.Load("agent-closed-loop")
	if err != nil {
		t.Fatalf("load after flush: %v", err)
	}
	if got.Title != entry.Title {
		t.Fatalf("title = %q, want %q", got.Title, entry.Title)
	}
	ctxActions, ok := got.Context["actions"].([]any)
	if !ok {
		t.Fatalf("context actions type = %T, want []any", got.Context["actions"])
	}
	if len(ctxActions) != 2 {
		t.Fatalf("persisted actions len = %d, want 2", len(ctxActions))
	}
	for i, raw := range ctxActions {
		item, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("persisted action[%d] type = %T, want map[string]any", i, raw)
		}
		if item["status"] != string(agent.ActionStatusExecuted) {
			t.Fatalf("persisted action[%d] status = %v, want %q", i, item["status"], agent.ActionStatusExecuted)
		}
		if item["output"] == "" {
			t.Fatalf("persisted action[%d] output is empty", i)
		}
	}
}

// TestContextCheckpointWithAgentRollback 验证 contextx Checkpoint 与 agent
// 回滚流程可以组合：执行失败后 RequestRollback/Rollback，同时可以从
// checkpoint 恢复上下文。
func TestContextCheckpointWithAgentRollback(t *testing.T) {
	w := contextx.NewWindow()
	w.Add(contextx.Message{Role: "user", Content: "initial context"})
	w.Add(contextx.Message{Role: "assistant", Content: "planned action"})

	cp := w.Checkpoint("cp-agent-rollback")
	store := contextx.NewCheckpointStore()
	if err := store.Save(cp); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	sess := agent.NewAgentSession(agent.AllowAll)
	sess.Plan([]agent.Action{
		{Type: agent.ActionTypeRunCommand, Command: "apply risky change"},
	})

	next := sess.NextPendingAction()
	if next == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if err := sess.MarkExecuted(next.ID, "change applied"); err != nil {
		t.Fatalf("mark executed: %v", err)
	}
	if err := sess.MarkTested(next.ID, false); err != nil {
		t.Fatalf("mark tested: %v", err)
	}
	if sess.Phase != agent.PhaseFailed {
		t.Fatalf("after failed test phase = %q, want %q", sess.Phase, agent.PhaseFailed)
	}

	if err := sess.RequestRollback(next.ID); err != nil {
		t.Fatalf("request rollback: %v", err)
	}
	if sess.Phase != agent.PhaseRollingBack {
		t.Fatalf("after request rollback phase = %q, want %q", sess.Phase, agent.PhaseRollingBack)
	}
	if sess.RollbackState != agent.RollbackRequested {
		t.Fatalf("rollback state = %q, want %q", sess.RollbackState, agent.RollbackRequested)
	}
	if !sess.Actions[0].RollbackRequested {
		t.Fatal("action rollback requested = false, want true")
	}

	if err := sess.Rollback(next.ID); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if sess.Phase != agent.PhaseDone {
		t.Fatalf("after rollback phase = %q, want %q", sess.Phase, agent.PhaseDone)
	}
	if sess.RollbackState != agent.RollbackCompleted {
		t.Fatalf("rollback state = %q, want %q", sess.RollbackState, agent.RollbackCompleted)
	}
	if sess.Actions[0].Status != agent.ActionStatusRolledBack {
		t.Fatalf("action status = %q, want %q", sess.Actions[0].Status, agent.ActionStatusRolledBack)
	}
	if !sess.Actions[0].RolledBack {
		t.Fatal("action rolled back = false, want true")
	}

	loaded, err := store.Load(cp.ID)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	w.Rewind(loaded)
	if w.Len() != 2 {
		t.Fatalf("rewound window len = %d, want 2", w.Len())
	}
	msgs := w.Messages()
	if msgs[0].Content != "initial context" || msgs[1].Content != "planned action" {
		t.Fatalf("rewound messages = %#v, want original messages", msgs)
	}
}

// TestMCPCallToolFeedsAgentActionOutput 验证 MCP 客户端与 Agent 的集成：
// 从 httptest mock MCP server 列出工具并调用工具，把工具返回文本作为
// Agent Action 的执行输出。
func TestMCPCallToolFeedsAgentActionOutput(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		var req mcp.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		switch req.Method {
		case "initialize":
			writeMCPResult(t, w, req.ID, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
			})
		case "tools/list":
			writeMCPResult(t, w, req.ID, map[string]any{
				"tools": []mcp.Tool{
					{
						Name:        "echo",
						Description: "Echo",
						InputSchema: map[string]any{"type": "object"},
					},
				},
			})
		case "tools/call":
			writeMCPResult(t, w, req.ID, map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "mcp-tool-output"},
				},
				"isError": false,
			})
		default:
			writeMCPError(t, w, req.ID, -32601, "method not found")
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cl := mcp.New(srv.URL + "/mcp")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cl.Initialize(ctx); err != nil {
		t.Fatalf("mcp initialize: %v", err)
	}

	tools, err := cl.ListTools(ctx)
	if err != nil {
		t.Fatalf("mcp list tools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}

	result, err := cl.CallTool(ctx, "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("mcp call tool: %v", err)
	}
	output, ok := mcpResultText(result)
	if !ok {
		t.Fatalf("unexpected mcp result: %#v", result)
	}
	if output != "mcp-tool-output" {
		t.Fatalf("mcp output = %q, want %q", output, "mcp-tool-output")
	}

	sess := agent.NewAgentSession(agent.AllowAll)
	sess.Plan([]agent.Action{
		{Type: agent.ActionTypeRunCommand, Description: "run MCP echo", Command: "mcp echo"},
	})

	next := sess.NextPendingAction()
	if next == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if err := sess.MarkExecuted(next.ID, output); err != nil {
		t.Fatalf("mark executed: %v", err)
	}
	if err := sess.MarkTested(next.ID, true); err != nil {
		t.Fatalf("mark tested: %v", err)
	}
	if sess.Phase != agent.PhaseReviewingDiff {
		t.Fatalf("after successful test phase = %q, want %q", sess.Phase, agent.PhaseReviewingDiff)
	}
	if sess.Actions[0].Output != "mcp-tool-output" {
		t.Fatalf("action output = %q, want %q", sess.Actions[0].Output, "mcp-tool-output")
	}
}

func writeMCPResult(t *testing.T, w http.ResponseWriter, id any, result any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mcp.NewResponse(id, result)); err != nil {
		t.Errorf("encode mcp response: %v", err)
	}
}

func writeMCPError(t *testing.T, w http.ResponseWriter, id any, code int, message string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(mcp.NewErrorResponse(id, code, message, nil)); err != nil {
		t.Errorf("encode mcp error response: %v", err)
	}
}

func mcpResultText(result any) (string, bool) {
	m, ok := result.(map[string]any)
	if !ok {
		return "", false
	}
	content, ok := m["content"].([]any)
	if !ok {
		return "", false
	}
	if len(content) == 0 {
		return "", false
	}
	first, ok := content[0].(map[string]any)
	if !ok {
		return "", false
	}
	text, ok := first["text"].(string)
	return text, ok
}
