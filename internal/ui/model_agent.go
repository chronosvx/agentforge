package ui

import (
	"fmt"
	"github.com/chronosvx/agentforge/internal/agent"
	"strings"
)

func (m *Model) newAgentSession() {
	s := agent.NewAgentSession(agent.RequireApproval)
	s.Plan([]agent.Action{
		{Type: agent.ActionTypeEditFile, Path: "README.md", Description: "示例：编辑 README.md"},
		{Type: agent.ActionTypeRunCommand, Command: "go test ./...", Description: "示例：运行测试"},
	})
	m.agentSession = s
	m.agentCursor = 0
}

// handleAgentKey 处理 Agent 审批面板的键盘操作，返回是否已消费。
func (m *Model) handleAgentKey(key string) bool {
	if m.agentSession == nil {
		return false
	}
	pending := m.agentSession.PendingApprovals()
	if len(pending) == 0 {
		switch key {
		case "e":
			if a := m.agentSession.NextPendingAction(); a != nil {
				_ = m.agentSession.MarkExecuted(a.ID, "executed from UI")
				m.status = "agent executing: " + a.ID
			} else {
				m.status = "agent: no pending action"
			}
			return true
		case "t":
			for i := range m.agentSession.Actions {
				a := &m.agentSession.Actions[i]
				if a.Status == agent.ActionStatusExecuted {
					_ = m.agentSession.MarkTested(a.ID, true)
					m.status = "agent tested: " + a.ID
					return true
				}
			}
			m.status = "agent: no executed action to test"
			return true
		}
		return false
	}
	if m.agentCursor < 0 {
		m.agentCursor = 0
	}
	if m.agentCursor >= len(pending) {
		m.agentCursor = len(pending) - 1
	}
	id := pending[m.agentCursor].ID
	switch key {
	case "j", "down":
		if m.agentCursor < len(pending)-1 {
			m.agentCursor++
		}
		return true
	case "k", "up":
		if m.agentCursor > 0 {
			m.agentCursor--
		}
		return true
	case "a":
		if err := m.agentSession.Approve(id); err != nil {
			m.status = "agent approve error: " + err.Error()
		} else {
			m.status = "agent approved: " + id
		}
		return true
	case "r":
		if err := m.agentSession.Reject(id); err != nil {
			m.status = "agent reject error: " + err.Error()
		} else {
			m.status = "agent rejected: " + id
		}
		return true
	case "e":
		if err := m.agentSession.MarkExecuted(id, "executed from UI"); err != nil {
			m.status = "agent execute error: " + err.Error()
		} else {
			m.status = "agent executed: " + id
		}
		return true
	case "t":
		if err := m.agentSession.MarkTested(id, true); err != nil {
			m.status = "agent test error: " + err.Error()
		} else {
			m.status = "agent tested: " + id
		}
		return true
	}
	return false
}

func (m *Model) renderAgentPanel() string {
	if m.agentSession == nil {
		return "No agent session. Use /agent-new to create one."
	}
	s := m.agentSession
	var b strings.Builder
	b.WriteString("Agent Session\n")
	b.WriteString(fmt.Sprintf("Phase: %s\n", s.Phase))
	b.WriteString(fmt.Sprintf("Policy: %v\n", s.Policy))
	pending := s.PendingApprovals()
	b.WriteString(fmt.Sprintf("Pending approvals: %d\n", len(pending)))
	for i, a := range pending {
		marker := " "
		if i == m.agentCursor {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s[%d] %s %s — %s\n", marker, i+1, a.ID, a.Type, a.Description))
	}
	if len(s.Actions) > 0 {
		b.WriteString("\nActions:\n")
		for i, a := range s.Actions {
			b.WriteString(fmt.Sprintf("  %d. %s [%s] %s\n", i+1, a.ID, a.Status, a.Description))
		}
	}
	return b.String()
}
