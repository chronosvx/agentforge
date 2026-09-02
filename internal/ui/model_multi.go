package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/agentflow"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"strings"
)

func (m *Model) startMultiAgent(tasks []multiagent.Task) tea.Cmd {
	if len(tasks) == 0 {
		m.status = "multi-agent: no tasks"
		return nil
	}
	for i := range tasks {
		if tasks[i].ID == "" {
			tasks[i].ID = fmt.Sprintf("task-%d", i)
		}
		if tasks[i].WorkDir == "" {
			tasks[i].WorkDir = m.workspaceDir()
		}
	}
	m.multiTasks = tasks
	m.multiResults = nil
	m.multiStates = map[string]string{}
	m.multiOutputs = map[string]string{}
	m.multiRunning = true
	m.multiCursor = 0
	m.panel = panelMulti
	m.status = fmt.Sprintf("multi-agent started: %d tasks", len(tasks))

	evCh := make(chan multiagent.Event, 64)
	doneCh := make(chan multiAgentDoneMsg, 1)
	m.multiEventCh = evCh
	m.multiDoneCh = doneCh

	runner := multiagent.NewRunner(func(ctx context.Context, task multiagent.Task) (string, error) {
		r := agentflow.NewRunner(agentflow.NewStaticPlanner(), nil)
		res, err := r.Run(ctx, agentflow.Request{
			Prompt:      task.Prompt,
			WorkDir:     task.WorkDir,
			AutoApprove: true,
			TestCmd:     task.TestCmd,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("phase=%s diff=%d", res.Session.Phase, len(res.Diff)), nil
	})
	runner.OnEvent = func(ev multiagent.Event) {
		evCh <- ev
	}
	go func() {
		results, err := runner.Run(context.Background(), tasks, nil)
		doneCh <- multiAgentDoneMsg{Results: results, Err: err}
	}()

	return m.waitMultiAgentCmd()
}

// waitMultiAgentCmd 等待多 Agent 运行的下一个事件或最终结果。
func (m *Model) waitMultiAgentCmd() tea.Cmd {
	return func() tea.Msg {
		select {
		case ev := <-m.multiEventCh:
			return multiAgentEventMsg{Event: ev}
		case done := <-m.multiDoneCh:
			return done
		}
	}
}

// workspaceDir 返回当前工作目录，缺省为当前目录。
func (m *Model) workspaceDir() string {
	if m.cfg != nil && strings.TrimSpace(m.cfg.Workspace) != "" {
		return strings.TrimSpace(m.cfg.Workspace)
	}
	return "."
}

// handleMultiKey 处理 Multi-Agent 面板聚焦时的上下移动。
// 当前显示的是运行中的任务列表或待执行的任务列表；没有可显示任务时返回 false。
func (m *Model) handleMultiKey(key string) bool {
	tasks := m.multiTasks
	if !m.multiRunning && len(m.multiPending) > 0 {
		tasks = m.multiPending
	}
	if len(tasks) == 0 {
		return false
	}
	if m.multiCursor < 0 {
		m.multiCursor = 0
	}
	if m.multiCursor >= len(tasks) {
		m.multiCursor = len(tasks) - 1
	}

	switch key {
	case "j", "down":
		if m.multiCursor < len(tasks)-1 {
			m.multiCursor++
		}
		return true
	case "k", "up":
		if m.multiCursor > 0 {
			m.multiCursor--
		}
		return true
	}
	return false
}

// pluginCommand 识别命令面板/输入框中的插件命令并返回执行 tea.Cmd。
func (m *Model) renderMultiAgentPanel() string {
	var b strings.Builder
	b.WriteString("Multi-Agent\n")

	pendingView := !m.multiRunning && len(m.multiPending) > 0
	tasks := m.multiTasks
	if pendingView {
		tasks = m.multiPending
	}
	if len(tasks) == 0 {
		b.WriteString("Use /multi <prompt1,prompt2,...> to start.\n")
		return b.String()
	}

	byID := make(map[string]multiagent.Task, len(tasks))
	children := make(map[string][]string, len(tasks))
	hasParent := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
		for _, dep := range t.Dependencies {
			children[dep] = append(children[dep], t.ID)
			hasParent[t.ID] = true
		}
	}
	roots := make([]string, 0, len(tasks))
	for _, t := range tasks {
		if !hasParent[t.ID] {
			roots = append(roots, t.ID)
		}
	}

	writeNode := func(id string, depth int) {}
	writeNode = func(id string, depth int) {
		t, ok := byID[id]
		if !ok {
			return
		}
		state := "pending"
		if !pendingView {
			if s, ok := m.multiStates[id]; ok {
				state = s
			}
		}
		idx := -1
		for i, tt := range tasks {
			if tt.ID == id {
				idx = i
				break
			}
		}
		marker := " "
		if idx == m.multiCursor {
			marker = ">"
		}
		indent := strings.Repeat("  ", depth)
		b.WriteString(fmt.Sprintf("%s%s%s [%s] %s\n", indent, marker, id, state, truncateText(t.Prompt, 40)))
		if !pendingView {
			if out, ok := m.multiOutputs[id]; ok && out != "" {
				b.WriteString(indent + "    " + truncateText(out, 80) + "\n")
			}
		}
		for _, child := range children[id] {
			writeNode(child, depth+1)
		}
	}

	if len(roots) > 0 {
		visited := make(map[string]bool, len(tasks))
		var renderTree func(id string, depth int)
		renderTree = func(id string, depth int) {
			if visited[id] {
				return
			}
			visited[id] = true
			writeNode(id, depth)
			for _, child := range children[id] {
				renderTree(child, depth+1)
			}
		}
		for _, id := range roots {
			renderTree(id, 0)
		}
		// 处理环或孤儿节点，避免漏显。
		for _, t := range tasks {
			if !visited[t.ID] {
				renderTree(t.ID, 0)
			}
		}
	} else {
		for i, t := range tasks {
			state := "pending"
			if !pendingView {
				if s, ok := m.multiStates[t.ID]; ok {
					state = s
				}
			}
			marker := " "
			if i == m.multiCursor {
				marker = ">"
			}
			b.WriteString(fmt.Sprintf("%s%s [%s] %s\n", marker, t.ID, state, truncateText(t.Prompt, 40)))
			if !pendingView {
				if out, ok := m.multiOutputs[t.ID]; ok && out != "" {
					b.WriteString("    " + truncateText(out, 80) + "\n")
				}
			}
		}
	}

	if m.multiRunning {
		b.WriteString("\nRunning...\n")
	} else if len(m.multiPending) > 0 {
		b.WriteString("\nPending... Use /multi-run to execute.\n")
	} else if len(m.multiResults) > 0 {
		b.WriteString(fmt.Sprintf("\nDone: %d tasks\n", len(m.multiResults)))
	}
	return b.String()
}

func splitCommaList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// multiTaskJSON 是 /multi 命令 JSON 输入的单条任务结构。
type multiTaskJSON struct {
	ID      string   `json:"id"`
	Prompt  string   `json:"prompt"`
	WorkDir string   `json:"workdir"`
	TestCmd string   `json:"test_cmd"`
	Deps    []string `json:"deps"`
}

// parseMultiTasks 解析 /multi 的 JSON 任务数组，并填充默认工作目录。
func parseMultiTasks(s, defaultWorkDir string) ([]multiagent.Task, error) {
	var raw []multiTaskJSON
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, errors.New("empty task list")
	}
	tasks := make([]multiagent.Task, 0, len(raw))
	for i, r := range raw {
		if strings.TrimSpace(r.Prompt) == "" {
			return nil, fmt.Errorf("task %d has empty prompt", i)
		}
		wd := r.WorkDir
		if wd == "" {
			wd = defaultWorkDir
		}
		tasks = append(tasks, multiagent.Task{
			ID:           r.ID,
			Prompt:       r.Prompt,
			WorkDir:      wd,
			TestCmd:      r.TestCmd,
			Dependencies: append([]string(nil), r.Deps...),
		})
	}
	return tasks, nil
}
