package ui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
	"strings"
)

func (m *Model) mouseWheelPanelOpen() bool {
	switch m.panel {
	case panelDiff, panelSearch, panelPlugins, panelSkills, panelAgent, panelMCP:
		return true
	}
	return false
}

// handleMouseWheel 处理滚轮事件：命令面板和可滚动列表中用滚轮移动光标。
// viewport 自身的滚轮滚动仍由后续 viewport.Update 处理。
func (m *Model) handleMouseWheel(msg tea.MouseMsg) {
	if msg.Button != tea.MouseButtonWheelUp && msg.Button != tea.MouseButtonWheelDown {
		return
	}

	if m.commandPaletteOpen {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if m.commandPaletteCursor > 0 {
				m.commandPaletteCursor--
			}
		case tea.MouseButtonWheelDown:
			if m.commandPaletteCursor < len(m.commandPaletteFiltered)-1 {
				m.commandPaletteCursor++
			}
		}
		return
	}

	switch m.panel {
	case panelDiff:
		if msg.Button == tea.MouseButtonWheelUp && m.diffCursor > 0 {
			m.diffCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.diffCursor < len(m.diffs)-1 {
			m.diffCursor++
		}
	case panelSearch:
		if msg.Button == tea.MouseButtonWheelUp && m.searchCursor > 0 {
			m.searchCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
	case panelPlugins:
		if msg.Button == tea.MouseButtonWheelUp && m.pluginCursor > 0 {
			m.pluginCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.pluginCursor < len(m.plugins)-1 {
			m.pluginCursor++
		}
	case panelSkills:
		if msg.Button == tea.MouseButtonWheelUp && m.skillCursor > 0 {
			m.skillCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.skillCursor < len(m.skills)-1 {
			m.skillCursor++
		}
	case panelAgent:
		pending := []agent.Action{}
		if m.agentSession != nil {
			pending = m.agentSession.PendingApprovals()
		}
		if msg.Button == tea.MouseButtonWheelUp && m.agentCursor > 0 {
			m.agentCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.agentCursor < len(pending)-1 {
			m.agentCursor++
		}
	case panelMCP:
		if msg.Button == tea.MouseButtonWheelUp && m.mcpCursor > 0 {
			m.mcpCursor--
		} else if msg.Button == tea.MouseButtonWheelDown && m.mcpCursor < len(m.mcpTools)-1 {
			m.mcpCursor++
		}
	}
}

func (m *Model) renderPanel() string {
	if m.panel == panelNone {
		return ""
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("6")).
		Padding(0, 1).
		Width(60).
		MaxHeight(20)

	switch m.panel {
	case panelContext:
		return "\n" + style.Render(m.renderContextPanel())
	case panelCost:
		return "\n" + style.Render(m.renderCostPanel())
	case panelSessions:
		return "\n" + style.Render(m.renderSessionsPanel())
	case panelDiff:
		return "\n" + style.Render(m.renderDiffPanel())
	case panelTodos:
		return "\n" + style.Render(m.renderTodosPanel())
	case panelTools:
		return "\n" + style.Render(m.renderToolsPanel())
	case panelSearch:
		return "\n" + style.Render(m.renderSearchPanel())
	case panelPlugins:
		return "\n" + style.Render(m.renderPluginsPanel())
	case panelSkills:
		return "\n" + style.Render(m.renderSkillsPanel())
	case panelAgent:
		return "\n" + style.Render(m.renderAgentPanel())
	case panelMCP:
		return "\n" + style.Render(m.renderMCPPanel())
	case panelExec:
		return "\n" + style.Render(m.renderExecPanel())
	case panelMulti:
		return "\n" + style.Render(m.renderMultiAgentPanel())
	default:
		return ""
	}
}

func (m *Model) renderTabs() string {
	if len(m.tabs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range m.tabs {
		label := sanitizeTerminalText(s.ID)
		if len(label) > 16 {
			label = label[:16] + "…"
		}
		if i == m.tabIndex && s.ID == m.sessionID {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("6")).
				Padding(0, 1).
				Render(" " + label + " "))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Padding(0, 1).
				Render(" " + label + " "))
		}
		b.WriteString(" ")
	}
	return b.String()
}

func (m *Model) renderSidebar() string {
	if len(m.tabs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Sessions\n")
	for i, s := range m.tabs {
		label := sanitizeTerminalText(s.ID)
		if len(label) > 20 {
			label = label[:20] + "…"
		}
		marker := " "
		if i == m.tabIndex && s.ID == m.sessionID {
			marker = "▶"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", marker, label))
	}
	return lipgloss.NewStyle().
		Width(sidebarWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(0, 1).
		Render(b.String())
}

func (m *Model) accentColor() string {
	if m.cfg != nil && m.cfg.Theme == "catppuccin" {
		return "5"
	}
	return "6"
}

func (m *Model) handleRightPanelKey(key string) bool {
	switch key {
	case "1":
		m.handleCommand("/context")
		return true
	case "2":
		m.handleCommand("/cost")
		return true
	case "3":
		m.handleCommand("/diff")
		return true
	case "4":
		m.handleCommand("/todos")
		return true
	case "5":
		m.handleCommand("/skills")
		return true
	}
	return false
}

func (m *Model) handleSplitKey(key string) bool {
	if len(m.tabs) < 2 {
		return false
	}
	var delta int
	switch key {
	case "]":
		delta = 1
	case "[":
		delta = -1
	default:
		return false
	}
	ids := make([]string, 0, len(m.tabs))
	for _, s := range m.tabs {
		ids = append(ids, s.ID)
	}
	cur := 0
	for i, id := range ids {
		if id == m.splitSessionID {
			cur = i
			break
		}
	}
	next := (cur + delta + len(ids)) % len(ids)
	m.splitSessionID = ids[next]
	m.status = "split session: " + m.splitSessionID
	return true
}

func (m *Model) renderSplitPanel() string {
	var b strings.Builder
	b.WriteString(i18n.T("panel.split_title", m.splitSessionID) + "\n\n")
	if m.store != nil && m.splitSessionID != "" {
		if entry, err := m.store.Load(m.splitSessionID); err == nil {
			for _, msg := range entry.Messages {
				role := msg.Role
				if role == "" {
					role = "?"
				}
				content := sanitizeTerminalText(msg.Content)
				if len(content) > 300 {
					content = content[:300] + "…"
				}
				b.WriteString(fmt.Sprintf("[%s] %s\n\n", role, content))
			}
		} else {
			b.WriteString("(no local history)\n")
		}
	}
	return lipgloss.NewStyle().
		Width(rightPanelWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(0, 1).
		Render(b.String())
}

func (m *Model) renderRightPanel() string {
	var b strings.Builder
	b.WriteString("Overview (1 ctx, 2 cost, 3 diff, 4 todo, 5 skills)\n")
	if m.context != nil {
		b.WriteString(fmt.Sprintf("tokens: %v\n", m.context["totalTokens"]))
		b.WriteString(fmt.Sprintf("output: %v\n", m.context["output"]))
	}
	if m.cost != nil {
		b.WriteString(fmt.Sprintf("cost: %v\n", m.cost["estimatedYuan"]))
	}
	b.WriteString(fmt.Sprintf("diffs: %d\n", len(m.diffs)))
	b.WriteString(fmt.Sprintf("todos: %d\n", len(m.todos)))
	b.WriteString(fmt.Sprintf("skills: %d\n", len(m.skills)))
	if m.costLimit > 0 {
		status := "ok"
		if m.costAlerted {
			status = "⚠️ over"
		}
		b.WriteString(fmt.Sprintf("limit: %v (%s)\n", m.costLimit, status))
	}
	return lipgloss.NewStyle().
		Width(rightPanelWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(0, 1).
		Render(b.String())
}
func (m *Model) renderContextPanel() string {
	if m.context == nil {
		return i18n.T("panel.context_unavailable")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.context_title") + "\n")
	b.WriteString(fmt.Sprintf("totalTokens:  %v\n", m.context["totalTokens"]))
	b.WriteString(fmt.Sprintf("cacheHits:    %v\n", m.context["cacheHits"]))
	b.WriteString(fmt.Sprintf("uncachedInput:%v\n", m.context["uncachedInput"]))
	b.WriteString(fmt.Sprintf("output:       %v\n", m.context["output"]))
	if bd, ok := m.context["breakdown"].(map[string]any); ok {
		b.WriteString("breakdown:\n")
		for k, v := range bd {
			b.WriteString(fmt.Sprintf("  %-10s %v\n", k, v))
		}
	}
	return b.String()
}
func (m *Model) renderCostPanel() string {
	if m.cost == nil {
		return i18n.T("panel.cost_unavailable")
	}
	est := 0.0
	if m.tokenUsage != nil {
		est = cost.EstimateFromUsage(m.tokenUsage)
	}
	return fmt.Sprintf(i18n.T("panel.cost_title")+"\ncurrency:          %v\nestimatedYuan:     %v\ntokenUsageEstimate: %.6f\n", m.cost["currency"], m.cost["estimatedYuan"], est)
}
func (m *Model) renderSessionsPanel() string {
	if len(m.sessions) == 0 {
		return i18n.T("panel.sessions_empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.sessions_title") + "\n")
	for i, s := range m.sessions {
		marker := " "
		if s.ID == m.sessionID {
			marker = "*"
		}
		b.WriteString(fmt.Sprintf("%s %d. %s  %s\n", marker, i+1, sanitizeTerminalText(s.ID), s.UpdatedAt.Format("01-02 15:04")))
	}
	return b.String()
}

func (m *Model) renderSearchPanel() string {
	if len(m.searchResults) == 0 {
		return i18n.T("panel.search_empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.search_title") + "\n")
	for i, r := range m.searchResults {
		marker := " "
		if i == m.searchCursor {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s[%d] %s/%s: %s\n", marker, i+1, sanitizeTerminalText(r.SessionID), sanitizeTerminalText(r.Role), sanitizeTerminalText(r.Snippet)))
	}
	return b.String()
}

func (m *Model) renderPluginsPanel() string {
	if len(m.plugins) == 0 {
		return i18n.T("panel.plugins_empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.plugins_title") + "\n")
	for i, p := range m.plugins {
		marker := " "
		if i == m.pluginCursor {
			marker = ">"
		}
		b.WriteString(fmt.Sprintf("%s[%d] %s v%s — %s\n", marker, i+1, p.Manifest.Name, p.Manifest.Version, p.Manifest.Description))
	}
	return b.String()
}

func (m *Model) handleSkillKey(key string) bool {
	if len(m.skills) == 0 {
		return false
	}
	if m.skillDetail {
		switch key {
		case "enter", "esc":
			m.skillDetail = false
			m.status = "skill: " + m.skills[m.skillCursor].Name
			return true
		}
		return false
	}

	switch key {
	case "j", "down":
		if m.skillCursor < len(m.skills)-1 {
			m.skillCursor++
		}
		return true
	case "k", "up":
		if m.skillCursor > 0 {
			m.skillCursor--
		}
		return true
	case " ":
		s := &m.skills[m.skillCursor]
		s.Enabled = !s.Enabled
		if s.Enabled {
			m.status = "enabled skill: " + s.Name
		} else {
			m.status = "disabled skill: " + s.Name
		}
		return true
	case "enter":
		m.skillDetail = true
		m.status = i18n.T("status.skill_detail", m.skills[m.skillCursor].Name)
		return true
	}
	return false
}

func (m *Model) renderTodosPanel() string {
	if len(m.todos) == 0 {
		return i18n.T("panel.todos_empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.todos_title") + "\n")
	for i, raw := range m.todos {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		content, _ := item["content"].(string)
		status, _ := item["status"].(string)
		marker := "⬜"
		switch status {
		case "completed":
			marker = "✅"
		case "in_progress":
			marker = "🔄"
		}
		b.WriteString(fmt.Sprintf("%s %d. [%s] %s\n", marker, i+1, status, sanitizeTerminalText(content)))
	}
	return b.String()
}

func (m *Model) renderToolsPanel() string {
	if len(m.tools) == 0 {
		return i18n.T("panel.tools_empty")
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.tools_title") + "\n")
	for i, raw := range m.tools {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		name, _ := item["name"].(string)
		args, _ := item["arguments"].(string)
		result, _ := item["result"].(string)
		if typ == "call" {
			b.WriteString(fmt.Sprintf("%d. ⚙ %s %s\n", i+1, name, sanitizeTerminalText(args)))
		} else {
			b.WriteString(fmt.Sprintf("%d. 📄 %s\n", i+1, sanitizeTerminalText(result)))
		}
	}
	return b.String()
}
func (m *Model) renderSkillsPanel() string {
	if len(m.skills) == 0 {
		return i18n.T("panel.skills_empty")
	}
	if m.skillDetail {
		s := m.skills[m.skillCursor]
		var b strings.Builder
		b.WriteString(i18n.T("panel.skill_detail_title") + "\n\n")
		b.WriteString("# " + sanitizeTerminalText(s.Name) + "  [" + skillState(s.Enabled) + "/" + s.Source + "]\n\n")
		detail := s.Detail
		if detail == "" {
			detail = s.Description
		}
		if detail == "" {
			detail = i18n.T("panel.skill_no_detail")
		}
		b.WriteString(sanitizeTerminalText(detail))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(i18n.T("panel.skills_title") + "\n")
	for i, s := range m.skills {
		marker := " "
		if i == m.skillCursor {
			marker = ">"
		}
		desc := s.Description
		if desc == "" {
			desc = i18n.T("panel.skill_no_desc")
		}
		b.WriteString(fmt.Sprintf("%s [%s] %-24s %-6s %s\n", marker, skillState(s.Enabled), truncateText(s.Name, 24), s.Source, truncateText(desc, 40)))
	}
	return b.String()
}
func skillState(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func (m *Model) applySessionFilter() {
	if m.sessionQuery == "" {
		// 复制一份，避免 sessions 与 allSessions 共享底层数组时互相覆盖。
		m.sessions = append([]session.Entry(nil), m.allSessions...)
		return
	}
	q := strings.ToLower(m.sessionQuery)
	m.sessions = make([]session.Entry, 0, len(m.allSessions))
	for _, s := range m.allSessions {
		if strings.Contains(strings.ToLower(s.ID), q) || strings.Contains(strings.ToLower(s.Title), q) {
			m.sessions = append(m.sessions, s)
		}
	}
}

func (m *Model) removeSessionFromWorkspace(id string) {
	filter := func(list []session.Entry) []session.Entry {
		out := list[:0]
		for _, s := range list {
			if s.ID != id {
				out = append(out, s)
			}
		}
		return out
	}
	m.tabs = filter(m.tabs)
	m.sessions = filter(m.sessions)
	if m.tabIndex >= len(m.tabs) {
		m.tabIndex = 0
	}
}
