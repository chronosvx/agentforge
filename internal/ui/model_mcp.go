package ui

import (
	"context"
	"errors"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/mcpperm"
	"strings"
	"time"
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

// mcpCallCmd 异步调用已连接的 MCP 工具；需要确认时挂起等待 /mcp-approve。
func (m *Model) mcpCallCmd(name string, args map[string]any) tea.Cmd {
	if m.mcpPolicy == nil {
		m.mcpPolicy = mcpperm.NewPolicy(mcpperm.ModeConfirm)
	}
	if m.mcpAudit == nil {
		m.mcpAudit = mcpperm.NewAuditLog()
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
		m.status = "mcp call requires approval: /mcp-approve or /mcp-cancel"
		return nil
	}

	return m.executeMCPCall(name, args)
}

// executeMCPCall 实际执行 MCP CallTool 并返回 mcpCallMsg。
func (m *Model) executeMCPCall(name string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
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
