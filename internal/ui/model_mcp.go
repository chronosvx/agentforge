package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/mcp"
)

func (m *Model) mcpConnectCmd(url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client := mcp.New(url)
		if err := client.Initialize(ctx); err != nil {
			return mcpConnectMsg{url: url, err: err}
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			return mcpConnectMsg{url: url, err: err}
		}
		return mcpConnectMsg{url: url, client: client, tools: tools}
	}
}

// mcpCallCmd 异步调用已连接的 MCP 工具；需要确认时挂起并打开审批面板。
func (m *Model) mcpCallCmd(name string, args map[string]any) tea.Cmd {
	if m.mcpPolicy == nil {
		m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
	}
	if m.mcpAudit == nil {
		m.mcpAudit = mcp.NewAuditLog()
	}
	decision := m.mcpPolicy.Decide(name)
	m.mcpAudit.Record(name, decision)
	m.auditEvent("mcp.call", map[string]any{"tool": name, "mode": string(decision.Mode), "allowed": decision.Allowed, "requiresApproval": decision.RequiresApproval})

	if !decision.Allowed {
		return func() tea.Msg {
			return mcpCallMsg{name: name, err: errors.New("mcp tool denied by policy")}
		}
	}

	if decision.RequiresApproval {
		m.pendingMCPName = name
		m.pendingMCPArgs = args
		m.mcpPendingOpen = true
		m.status = i18n.T("mcp.approval_pending")
		return nil
	}

	return m.executeMCPCall(name, args)
}

// executeMCPCall 实际执行 MCP CallTool 并返回 mcpCallMsg。
func (m *Model) executeMCPCall(name string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		decision := m.mcpPolicy.Decide(name)
		m.auditEvent("mcp.call", map[string]any{"tool": name, "mode": string(decision.Mode), "allowed": decision.Allowed, "requiresApproval": decision.RequiresApproval})
		if m.mcpClient == nil {
			return mcpCallMsg{name: name, err: errors.New("mcp not connected")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := m.mcpClient.CallTool(ctx, name, args)
		m.auditEvent("mcp.call.complete", map[string]any{"tool": name, "error": err != nil})
		return mcpCallMsg{name: name, result: result, err: err}
	}
}

// closeMCPApproval 关闭 MCP 审批面板并清空挂起的调用信息。
func (m *Model) closeMCPApproval() {
	m.mcpPendingOpen = false
	m.pendingMCPName = ""
	m.pendingMCPArgs = nil
}

// handleMCPApprovalKey 处理 MCP 工具审批面板的按键，返回是否已消费以及需要执行的 tea.Cmd。
func (m *Model) handleMCPApprovalKey(key string) (bool, tea.Cmd) {
	name := m.pendingMCPName
	if name == "" {
		if key == "esc" || key == "q" {
			m.closeMCPApproval()
			return true, nil
		}
		return false, nil
	}
	args := m.pendingMCPArgs

	switch key {
	case "a":
		m.closeMCPApproval()
		m.status = i18n.T("mcp.approved_once")
		return true, m.executeMCPCall(name, args)
	case "s":
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		m.mcpPolicy.Set(name, mcp.ModeAuto)
		m.closeMCPApproval()
		m.status = i18n.T("mcp.always_allowed")
		return true, m.executeMCPCall(name, args)
	case "d":
		m.closeMCPApproval()
		m.status = i18n.T("mcp.denied_once")
		return true, nil
	case "n":
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		m.mcpPolicy.Set(name, mcp.ModeDeny)
		m.closeMCPApproval()
		m.status = i18n.T("mcp.denied_and_remembered")
		return true, nil
	case "esc", "q":
		m.closeMCPApproval()
		m.status = "mcp call cancelled"
		return true, nil
	}
	return false, nil
}

// renderMCPApproval 渲染 MCP 工具审批面板。
func (m *Model) renderMCPApproval() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("3")).
		Padding(0, 1).
		Width(72).
		MaxHeight(20)

	var b strings.Builder
	b.WriteString(i18n.T("mcp.approval_title") + "\n\n")
	if m.pendingMCPName != "" {
		b.WriteString(i18n.T("mcp.approval_tool", m.pendingMCPName) + "\n")
	} else {
		b.WriteString(i18n.T("mcp.approval_tool", "(none)") + "\n")
	}
	argsText := "{}"
	if len(m.pendingMCPArgs) > 0 {
		if data, err := json.Marshal(m.pendingMCPArgs); err == nil {
			argsText = string(data)
		}
	}
	b.WriteString(i18n.T("mcp.approval_args", argsText) + "\n")
	if m.mcpPolicy != nil {
		mode := m.mcpPolicy.Mode(m.pendingMCPName)
		b.WriteString(fmt.Sprintf("Policy: %s\n", mode))
	}
	b.WriteString("\n" + i18n.T("mcp.approval_hint"))
	return "\n" + style.Render(b.String())
}

// startMultiAgent 启动一个多 Agent 并行执行任务，并打开多 Agent 面板。
func (m *Model) renderMCPPanel() string {
	if m.mcpClient == nil {
		return "No MCP server connected. Use /mcp <url> to connect."
	}
	if len(m.mcpTools) == 0 {
		return "MCP connected, but no tools advertised."
	}
	var b strings.Builder
	b.WriteString("MCP Tools\n")
	for i, t := range m.mcpTools {
		marker := " "
		if i == m.mcpCursor {
			marker = ">"
		}
		desc := t.Description
		if desc == "" {
			desc = "(no description)"
		}
		b.WriteString(fmt.Sprintf("%s[%d] %s — %s\n", marker, i+1, t.Name, desc))
	}
	return b.String()
}
