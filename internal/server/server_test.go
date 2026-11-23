package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/memory"
	"github.com/chronosvx/agentforge/internal/session"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	mem, err := memory.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{
		Config:  &config.Config{DataDir: dir},
		Store:   store,
		Memory:  mem,
		Token:   "test-token",
		Version: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, dir
}

func doRequest(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthRequiresToken(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec := doRequest(t, h, http.MethodGet, "/api/health", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}

	rec = doRequest(t, h, http.MethodGet, "/api/health", "", "test-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "test" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestSessionsAndMemory(t *testing.T) {
	srv, dir := newTestServer(t)
	h := srv.Handler()

	// 初始会话为空
	rec := doRequest(t, h, http.MethodGet, "/api/sessions", "", "test-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("sessions status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"sessions"`) {
		t.Fatalf("sessions body missing sessions: %s", rec.Body.String())
	}

	// 保存一条记忆后应能在 /api/memory 看到
	mem, err := memory.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.Save(context.Background(), memory.Entry{
		Project: "demo",
		Key:     "decision",
		Content: "use Flutter + Go daemon",
	}); err != nil {
		t.Fatal(err)
	}
	rec = doRequest(t, h, http.MethodGet, "/api/memory", "", "test-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("memory status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "use Flutter + Go daemon") {
		t.Fatalf("memory body missing entry: %s", rec.Body.String())
	}

	rec = doRequest(t, h, http.MethodGet, "/api/memory?project=demo", "", "test-token")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "decision") {
		t.Fatalf("memory project query failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentRunDryRunStream(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	body := `{"prompt":"test","work_dir":".","planner":"static","dry_run":true,"auto_approve":true}`
	rec := doRequest(t, h, http.MethodPost, "/api/agent/runs", body, "test-token")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create run status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("empty run id")
	}

	// 等待运行完成
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		run := srv.runs.Get(created.ID)
		if run != nil {
			run.mu.Lock()
			done := run.Done
			run.mu.Unlock()
			if done {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 从历史事件中应能看到 plan 与 done
	run := srv.runs.Get(created.ID)
	if run == nil {
		t.Fatal("run not found")
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	hasPlan, hasDone := false, false
	for _, ev := range run.Events {
		if ev.Type == "plan" {
			hasPlan = true
		}
		if ev.Type == "done" {
			hasDone = true
		}
	}
	if !hasPlan || !hasDone {
		t.Fatalf("events missing plan/done: %#v", run.Events)
	}
	if run.Err != "" {
		t.Fatalf("run error: %s", run.Err)
	}
}

func TestAgentRunEventsSSE(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	body := `{"prompt":"test","work_dir":".","planner":"static","dry_run":true,"auto_approve":true}`
	rec := doRequest(t, h, http.MethodPost, "/api/agent/runs", body, "test-token")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create run status = %d", rec.Code)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)

	req := httptest.NewRequest(http.MethodGet, "/api/agent/runs/"+created.ID+"/events", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("sse status = %d", rr.Code)
	}
	scanner := bufio.NewScanner(strings.NewReader(rr.Body.String()))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	gotDone := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: done") {
			gotDone = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan SSE body: %v", err)
	}
	if !gotDone {
		t.Fatalf("SSE did not contain done event; body=%q", rr.Body.String())
	}
}

func TestAgentRunRequiresPrompt(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec := doRequest(t, h, http.MethodPost, "/api/agent/runs", `{"prompt":""}`, "test-token")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
