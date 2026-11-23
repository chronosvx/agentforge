package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chronosvx/agentforge/internal/agentflow"
)

// AgentRequest 是 GUI 发起一次 Agent 运行的输入。
type AgentRequest struct {
	Prompt      string        `json:"prompt"`
	WorkDir     string        `json:"work_dir"`
	RepoMode    bool          `json:"repo_mode"`
	Planner     string        `json:"planner"`
	Provider    string        `json:"provider"`
	ProviderKey string        `json:"provider_key,omitempty"`
	BaseURL     string        `json:"base_url,omitempty"`
	Model       string        `json:"model,omitempty"`
	AutoApprove bool          `json:"auto_approve"`
	DryRun      bool          `json:"dry_run"`
	TestCmd     string        `json:"test_cmd,omitempty"`
	Timeout     time.Duration `json:"timeout,omitempty"`
}

// Run 是一次 Agent 运行的实时状态。
type Run struct {
	ID        string            `json:"id"`
	Request   AgentRequest      `json:"request"`
	Events    []agentflow.Event `json:"events"`
	Done      bool              `json:"done"`
	Err       string            `json:"error,omitempty"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at,omitempty"`

	mu     sync.Mutex
	subs   map[chan agentflow.Event]struct{}
	closed bool
}

// RunManager 管理多个并发 Agent 运行。
type RunManager struct {
	mu   sync.Mutex
	next uint64
	runs map[string]*Run
}

// NewRunManager 创建一个空的运行管理器。
func NewRunManager() *RunManager {
	return &RunManager{runs: make(map[string]*Run)}
}

// Start 注册一个新运行并立即在后台执行。
func (m *RunManager) Start(ctx context.Context, runner *agentflow.Runner, req AgentRequest) (*Run, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("server: prompt is required")
	}
	m.mu.Lock()
	m.next++
	id := fmt.Sprintf("run-%d-%d", time.Now().UnixNano(), m.next)
	run := &Run{
		ID:        id,
		Request:   req,
		Events:    make([]agentflow.Event, 0),
		StartedAt: time.Now(),
		subs:      make(map[chan agentflow.Event]struct{}),
	}
	m.runs[id] = run
	m.mu.Unlock()

	go m.execute(ctx, run, runner)
	return run, nil
}

// Get 返回指定运行；不存在时返回 nil。
func (m *RunManager) Get(id string) *Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[id]
}

// List 返回当前所有运行的快照。
func (m *RunManager) List() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]map[string]any, 0, len(m.runs))
	for _, run := range m.runs {
		out = append(out, run.Snapshot())
	}
	return out
}

// Subscribe 注册一个事件订阅者，返回事件 channel 和取消函数。
func (m *RunManager) Subscribe(id string) (<-chan agentflow.Event, func(), error) {
	run := m.Get(id)
	if run == nil {
		return nil, nil, fmt.Errorf("server: run %s not found", id)
	}
	ch := make(chan agentflow.Event, 64)
	run.mu.Lock()
	if run.closed {
		run.mu.Unlock()
		close(ch)
		return ch, func() {}, nil
	}
	run.subs[ch] = struct{}{}
	run.mu.Unlock()

	cancel := func() {
		run.mu.Lock()
		if _, ok := run.subs[ch]; ok {
			delete(run.subs, ch)
			close(ch)
		}
		run.mu.Unlock()
	}
	return ch, cancel, nil
}

func (m *RunManager) execute(ctx context.Context, run *Run, runner *agentflow.Runner) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if runner.OnEvent == nil {
		runner.OnEvent = func(ev agentflow.Event) {
			run.publish(ev)
		}
	} else {
		base := runner.OnEvent
		runner.OnEvent = func(ev agentflow.Event) {
			base(ev)
			run.publish(ev)
		}
	}

	_, err := runner.Run(runCtx, agentflow.Request{
		Prompt:      run.Request.Prompt,
		WorkDir:     run.Request.WorkDir,
		AutoApprove: run.Request.AutoApprove,
		Timeout:     run.Request.Timeout,
		TestCmd:     run.Request.TestCmd,
		DryRun:      run.Request.DryRun,
	})
	run.mu.Lock()
	run.Done = true
	run.EndedAt = time.Now()
	if err != nil {
		run.Err = err.Error()
	}
	run.closed = true
	for ch := range run.subs {
		close(ch)
	}
	run.subs = nil
	run.mu.Unlock()
}

func (run *Run) publish(ev agentflow.Event) {
	run.mu.Lock()
	defer run.mu.Unlock()
	run.Events = append(run.Events, ev)
	for ch := range run.subs {
		select {
		case ch <- ev:
		default:
			// 订阅者消费太慢时丢弃该事件，避免阻塞 Agent 执行。
		}
	}
}

// Snapshot 返回运行的 JSON 友好快照。
func (run *Run) Snapshot() map[string]any {
	run.mu.Lock()
	defer run.mu.Unlock()
	return map[string]any{
		"id":         run.ID,
		"request":    run.Request,
		"events":     run.Events,
		"done":       run.Done,
		"error":      run.Err,
		"started_at": run.StartedAt,
		"ended_at":   run.EndedAt,
	}
}
