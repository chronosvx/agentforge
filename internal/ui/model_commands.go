package ui

import (
	"context"
	"encoding/json"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/skill"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Model) openCommandPalette() {
	m.commandPaletteOpen = true
	m.commandPaletteQuery = ""
	m.commandPaletteCursor = 0
	m.showAllCommands = false
	m.commandPaletteFiltered = m.commandPaletteEntries()
}

// commandPaletteEntries 返回命令面板可执行列表：基础命令（默认）或全部命令 + 已加载插件的命令。
func (m *Model) commandPaletteEntries() []string {
	base := commandPaletteCommands
	if !m.showAllCommands {
		base = basicCommandPaletteCommands
	}
	entries := append([]string(nil), base...)
	if m.pluginManager == nil {
		return entries
	}
	seen := make(map[string]bool, len(entries))
	for _, cmd := range entries {
		seen[cmd] = true
	}
	for _, p := range m.pluginManager.List() {
		name := p.Manifest.Name
		for _, command := range p.Manifest.Commands {
			command = strings.TrimSpace(command)
			if command == "" {
				continue
			}
			command = strings.TrimPrefix(command, "/")
			explicit := "/plugin:" + name + ":" + command
			if !seen[explicit] {
				entries = append(entries, explicit)
				seen[explicit] = true
			}
			short := "/" + command
			if !seen[short] {
				entries = append(entries, short)
				seen[short] = true
			}
		}
	}
	return entries
}

// closeCommandPalette 关闭命令面板并清空搜索状态。
func (m *Model) closeCommandPalette() {
	m.commandPaletteOpen = false
	m.commandPaletteQuery = ""
	m.commandPaletteCursor = 0
	m.commandPaletteFiltered = nil
}

// handleCommandPaletteKey 处理命令面板打开时的按键，返回是否已消费该按键以及需要执行的 tea.Cmd。
func (m *Model) handleCommandPaletteKey(key tea.KeyMsg) (bool, tea.Cmd) {
	switch key.String() {
	case "ctrl+p", "esc":
		m.closeCommandPalette()
		return true, nil
	case "enter":
		if len(m.commandPaletteFiltered) == 0 {
			return true, nil
		}
		if m.commandPaletteCursor < 0 || m.commandPaletteCursor >= len(m.commandPaletteFiltered) {
			m.commandPaletteCursor = 0
		}
		cmd := m.commandPaletteFiltered[m.commandPaletteCursor]
		m.closeCommandPalette()
		return true, m.handleCommand(cmd)
	case "tab":
		m.showAllCommands = !m.showAllCommands
		m.applyCommandPaletteFilter()
		return true, nil
	case "j", "down", "ctrl+n":
		if len(m.commandPaletteFiltered) > 0 && m.commandPaletteCursor < len(m.commandPaletteFiltered)-1 {
			m.commandPaletteCursor++
		}
		return true, nil
	case "k", "up":
		if m.commandPaletteCursor > 0 {
			m.commandPaletteCursor--
		}
		return true, nil
	case "home":
		m.commandPaletteCursor = 0
		return true, nil
	case "end":
		if len(m.commandPaletteFiltered) > 0 {
			m.commandPaletteCursor = len(m.commandPaletteFiltered) - 1
		}
		return true, nil
	case "backspace":
		if len(m.commandPaletteQuery) > 0 {
			m.commandPaletteQuery = m.commandPaletteQuery[:len(m.commandPaletteQuery)-1]
			m.applyCommandPaletteFilter()
		}
		return true, nil
	default:
		if key.String() == "ctrl+c" {
			return false, nil
		}
		if key.Type == tea.KeyRunes && len(key.Runes) > 0 {
			m.commandPaletteQuery += string(key.Runes)
			m.applyCommandPaletteFilter()
		}
		// 面板打开时消费其它非文本按键，避免误触到底层输入框。
		return true, nil
	}
}

// applyCommandPaletteFilter 根据当前查询重新过滤命令列表。
func (m *Model) applyCommandPaletteFilter() {
	m.commandPaletteFiltered = filterCommandPaletteList(m.commandPaletteQuery, m.commandPaletteEntries())
	if m.commandPaletteCursor >= len(m.commandPaletteFiltered) {
		m.commandPaletteCursor = 0
	}
}

// filterCommandPalette 对内置命令列表做前缀/包含/子序列模糊匹配。
func filterCommandPalette(query string) []string {
	return filterCommandPaletteList(query, commandPaletteCommands)
}

// filterCommandPaletteList 对给定命令列表做前缀/包含/子序列模糊匹配。
func filterCommandPaletteList(query string, commands []string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return append([]string(nil), commands...)
	}

	var prefix, contains, fuzzy []string
	for _, cmd := range commands {
		lower := strings.ToLower(cmd)
		switch {
		case strings.HasPrefix(lower, q):
			prefix = append(prefix, cmd)
		case strings.Contains(lower, q):
			contains = append(contains, cmd)
		case isFuzzySubsequence(lower, q):
			fuzzy = append(fuzzy, cmd)
		}
	}
	return append(append(prefix, contains...), fuzzy...)
}

// isFuzzySubsequence 判断 q 是否为 s 的子序列（按字符顺序出现即可）。
func isFuzzySubsequence(s, q string) bool {
	if q == "" {
		return true
	}
	si := 0
	for qi := 0; qi < len(q); qi++ {
		found := false
		for si < len(s) {
			if s[si] == q[qi] {
				found = true
				si++
				break
			}
			si++
		}
		if !found {
			return false
		}
	}
	return true
}

func (m *Model) skillInstallCmd(target string) tea.Cmd {
	return func() tea.Msg {
		if info, err := os.Stat(target); err == nil && info.IsDir() {
			entries, err := skill.InstallFromDir(target, m.cfg.DataDir)
			return skillInstallMsg{entries: entries, err: err}
		}
		entries, err := skill.InstallFromRepo(target, m.cfg.DataDir)
		return skillInstallMsg{entries: entries, err: err}
	}
}

// handleCommand 处理以 / 开头的本地命令。目前返回时不发送给 Agent。
func (m *Model) handleCommand(cmd string) tea.Cmd {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return nil
	}
	name := strings.ToLower(fields[0])
	if pluginCmd := m.pluginCommand(name, fields[1:]); pluginCmd != nil {
		return pluginCmd
	}
	switch name {
	case "/context":
		if m.panel == panelContext {
			m.closePanel()
		} else {
			m.panel = panelContext
		}
	case "/cost":
		if m.panel == panelCost {
			m.closePanel()
		} else {
			m.panel = panelCost
		}
	case "/diff":
		if m.panel == panelDiff {
			m.closePanel()
		} else {
			m.panel = panelDiff
		}
	case "/todos":
		if m.panel == panelTodos {
			m.closePanel()
		} else {
			m.panel = panelTodos
		}
	case "/tools":
		if m.panel == panelTools {
			m.closePanel()
		} else {
			m.panel = panelTools
		}
	case "/skills":
		if m.panel == panelSkills {
			m.closePanel()
		} else {
			m.panel = panelSkills
		}
		m.skillDetail = false
	case "/plugin":
		if m.panel == panelPlugins {
			m.closePanel()
		} else {
			m.panel = panelPlugins
		}
		if m.pluginManager != nil {
			m.plugins = m.pluginManager.List()
		}
	case "/agent":
		sub := ""
		if len(fields) > 1 {
			sub = strings.ToLower(fields[1])
		}
		switch sub {
		case "new", "reset":
			m.newAgentSession()
			m.agentCursor = 0
			m.panel = panelAgent
			m.status = "agent session " + sub + ": " + string(m.agentSession.Phase)
		default:
			if m.agentSession == nil {
				m.newAgentSession()
				m.agentCursor = 0
			}
			if m.panel == panelAgent {
				m.closePanel()
			} else {
				m.panel = panelAgent
			}
		}
	case "/agent-new":
		m.newAgentSession()
		m.agentCursor = 0
		m.panel = panelAgent
		m.status = "agent session created: " + string(m.agentSession.Phase)
	case "/agent-reset":
		m.newAgentSession()
		m.agentCursor = 0
		m.panel = panelAgent
		m.status = "agent session reset: " + string(m.agentSession.Phase)
	case "/checkpoint":
		m.handleCheckpointCommand(cmd)
	case "/rewind":
		if strings.TrimSpace(strings.TrimPrefix(cmd, "/rewind")) == "" {
			m.openRewind()
		} else {
			m.handleRewindCommand(cmd)
		}
	case "/mcp":
		url := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp"))
		if url == "" {
			if m.mcpClient == nil {
				m.status = "usage: /mcp <url>"
			} else if m.panel == panelMCP {
				m.closePanel()
			} else {
				m.panel = panelMCP
			}
			break
		}
		return m.mcpConnectCmd(url)
	case "/mcp-call":
		if m.mcpClient == nil {
			m.status = "mcp not connected: use /mcp <url>"
			break
		}
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp-call"))
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			m.status = "usage: /mcp-call <tool> <json>"
			break
		}
		tool := strings.TrimSpace(parts[0])
		raw := strings.TrimSpace(parts[1])
		var args map[string]any
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			m.status = "mcp-call invalid json: " + err.Error()
			break
		}
		return m.mcpCallCmd(tool, args)
	case "/mcp-approve":
		if m.pendingMCPName == "" {
			m.status = "no pending mcp call to approve"
			break
		}
		name := m.pendingMCPName
		args := m.pendingMCPArgs
		m.pendingMCPName = ""
		m.pendingMCPArgs = nil
		m.mcpPendingOpen = false
		m.status = "mcp call approved: " + name
		return m.executeMCPCall(name, args)
	case "/mcp-cancel":
		m.pendingMCPName = ""
		m.pendingMCPArgs = nil
		m.mcpPendingOpen = false
		m.status = "mcp call cancelled"
	case "/mcp-auto":
		tool := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp-auto"))
		if tool == "" {
			m.status = "usage: /mcp-auto <tool>"
			break
		}
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		m.mcpPolicy.Set(tool, mcp.ModeAuto)
		m.status = "mcp policy auto: " + tool
	case "/mcp-confirm":
		tool := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp-confirm"))
		if tool == "" {
			m.status = "usage: /mcp-confirm <tool>"
			break
		}
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		m.mcpPolicy.Set(tool, mcp.ModeConfirm)
		m.status = "mcp policy confirm: " + tool
	case "/mcp-deny":
		tool := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp-deny"))
		if tool == "" {
			m.status = "usage: /mcp-deny <tool>"
			break
		}
		if m.mcpPolicy == nil {
			m.mcpPolicy = mcp.NewPolicy(mcp.ModeConfirm)
		}
		m.mcpPolicy.Set(tool, mcp.ModeDeny)
		m.status = "mcp policy deny: " + tool
	case "/mcp-audit":
		if m.mcpAudit == nil {
			m.mcpAudit = mcp.NewAuditLog()
		}
		entries := m.mcpAudit.List()
		const maxAuditEntries = 5
		start := 0
		if len(entries) > maxAuditEntries {
			start = len(entries) - maxAuditEntries
		}
		lines := make([]string, 0, len(entries)-start)
		for _, e := range entries[start:] {
			lines = append(lines, fmt.Sprintf("%s %s %t %t", e.Tool, e.Decision.Mode, e.Decision.Allowed, e.Decision.RequiresApproval))
		}
		if len(lines) == 0 {
			m.status = "mcp audit: no entries"
		} else {
			m.status = "mcp audit: " + strings.Join(lines, "; ")
		}
	case "/timeline":
		m.handleTimelineCommand()
	case "/fork":
		m.handleForkCommand(cmd)
	case "/remember":
		m.handleRememberCommand(cmd)
	case "/forget":
		m.handleForgetCommand(cmd)
	case "/memory":
		m.handleMemoryCommand(cmd)
	case "/multi":
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/multi"))
		if rest == "" {
			m.status = "usage: /multi <prompt1,prompt2,...> 或 /multi [{\"id\":\"a\",\"prompt\":\"...\",\"deps\":[\"b\"]}]"
			break
		}
		if strings.HasPrefix(rest, "[") || strings.HasPrefix(rest, "{") {
			tasks, err := parseMultiTasks(rest, m.workspaceDir())
			if err != nil {
				m.status = "multi json error: " + err.Error()
				break
			}
			m.multiPending = tasks
			m.panel = panelMulti
			m.status = "multi-agent tasks ready: /multi-run to execute"
			break
		}
		prompts := splitCommaList(rest)
		if len(prompts) == 0 {
			m.status = "usage: /multi <prompt1,prompt2,...>"
			break
		}
		tasks := make([]multiagent.Task, 0, len(prompts))
		for i, p := range prompts {
			tasks = append(tasks, multiagent.Task{ID: fmt.Sprintf("task-%d", i), Prompt: p, WorkDir: m.workspaceDir()})
		}
		m.multiPending = tasks
		m.panel = panelMulti
		m.status = "multi-agent tasks ready: /multi-run to execute"
	case "/multi-run":
		if len(m.multiPending) == 0 {
			m.status = "multi-agent: no pending tasks"
			break
		}
		tasks := m.multiPending
		m.multiPending = nil
		return m.startMultiAgent(tasks)
	case "/agents":
		if m.panel == panelMulti {
			m.closePanel()
		} else {
			m.panel = panelMulti
		}
	case "/agentview":
		if m.agentViewOpen {
			m.closeAgentView()
		} else {
			m.openAgentView()
		}
	case "/history":
		if m.historySearchOpen {
			m.closeHistorySearch()
		} else {
			m.openHistorySearch()
		}
	case "/compact":
		m.handleCompactCommand()
	case "/exec":
		rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/exec"))
		if rest == "" {
			m.execOpen = !m.execOpen
			m.panel = panelExec
			break
		}
		m.execCmd = rest
		m.execResult = ""
		m.execErr = nil
		m.execRunning = true
		m.panel = panelExec
		m.execOpen = true
		return m.execRunCmd(rest)
	case "/search":
		m.sessionQuery = strings.TrimSpace(strings.TrimPrefix(cmd, "/search"))
		if m.sessionQuery == "" {
			m.sessionQuery = ""
			m.status = i18n.T("status.search_cleared")
		} else {
			m.status = i18n.T("status.search_sessions", m.sessionQuery)
		}
		m.applySessionFilter()
	case "/find":
		query := strings.TrimSpace(strings.TrimPrefix(cmd, "/find"))
		if query == "" || m.store == nil {
			m.status = i18n.T("usage.find")
			break
		}
		list, err := m.store.List()
		if err != nil {
			m.status = "find error: " + err.Error()
			break
		}
		idx := &session.Index{}
		idx.AddAll(list)
		m.searchResults = idx.Search(query, 50)
		m.searchCursor = 0
		m.panel = panelSearch
		m.status = i18n.T("status.find_results", len(m.searchResults))
	case "/clear":
		m.messages = nil
		m.context = nil
		m.cost = nil
		m.diffs = nil
		m.todos = nil
		m.tools = nil
		m.searchResults = nil
		m.searchCursor = 0
		m.dirty = true
		m.followBottom = true
		m.status = i18n.T("status.cleared")
	case "/status":
		m.status = m.statusSummary()
	case "/right":
		m.rightPanel = !m.rightPanel
		m.applyLayout()
	case "/split":
		if !m.splitEnabled && len(m.tabs) > 1 {
			m.splitEnabled = true
			for _, s := range m.tabs {
				if s.ID != m.sessionID {
					m.splitSessionID = s.ID
					break
				}
			}
		} else {
			m.splitEnabled = false
			m.splitSessionID = ""
		}
		m.applyLayout()
	case "/sessions", "/resume":
		if m.panel == panelSessions {
			m.closePanel()
		} else {
			m.panel = panelSessions
		}
		if lister, ok := m.cl.(RemoteSessionLister); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if list, err := lister.ListSessions(ctx); err == nil {
				m.allSessions = list
				m.applySessionFilter()
			}
			cancel()
		} else if m.store != nil {
			if list, err := m.store.List(); err == nil {
				m.allSessions = list
				m.applySessionFilter()
			}
		}
	case "/help":
		m.closePanel()
		m.openHelp()
	case "/close":
		if m.eventCancel != nil {
			m.eventCancel()
			m.eventCancel = nil
		}
		if m.channelID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = m.cl.Close(ctx, m.channelID)
			cancel()
		}
		m.events = nil
		m.status = "channel closed"
	case "/model":
		sw, ok := m.cl.(ModelSwitcher)
		if !ok {
			m.status = i18n.T("status.model_provider_only")
			break
		}
		modelArg := strings.TrimSpace(strings.TrimPrefix(cmd, "/model"))
		if modelArg == "" {
			providerName := sw.ProviderName()
			if providerName == "" {
				providerName = "provider"
			}
			m.status = i18n.T("status.current_model", sw.Model(), providerName)
			break
		}
		if err := sw.SetModel(modelArg); err != nil {
			m.status = "model error: " + err.Error()
			break
		}
		m.status = i18n.T("status.model_switched", modelArg)
	case "/new":
		m.sessionTitle = ""
		m.pendingNew = true
		m.status = "new session..."
	case "/export":
		path, err := m.exportSession()
		if err != nil {
			m.status = "export error: " + err.Error()
		} else {
			m.auditEvent("session.export", map[string]any{
				"sessionId": m.sessionID,
				"path":      path,
			})
			m.status = "exported: " + path
		}
	case "/rename":
		title := strings.TrimSpace(strings.TrimPrefix(cmd, "/rename"))
		if title == "" || m.store == nil {
			m.status = i18n.T("usage.rename")
			break
		}
		if entry, err := m.store.Load(m.sessionID); err == nil {
			entry.Title = title
			entry.UpdatedAt = time.Now()
			if err := m.store.Save(*entry); err != nil {
				m.status = "rename error: " + err.Error()
			} else {
				m.sessionTitle = title
				m.status = "renamed: " + title
			}
		} else {
			m.status = "rename error: " + err.Error()
		}
	case "/delete":
		if m.store != nil {
			if err := m.store.Delete(m.sessionID); err != nil {
				m.status = "delete error: " + err.Error()
			} else {
				m.auditEvent("session.delete", map[string]any{"sessionId": m.sessionID})
				m.removeSessionFromWorkspace(m.sessionID)
				m.status = "deleted: " + m.sessionID
			}
		}
	case "/skill-install":
		target := strings.TrimSpace(strings.TrimPrefix(cmd, "/skill-install"))
		if target == "" || m.cfg == nil {
			m.status = i18n.T("usage.skill_install")
			return nil
		}
		return m.skillInstallCmd(target)
	case "/skill-sync":
		manifestURL := strings.TrimSpace(strings.TrimPrefix(cmd, "/skill-sync"))
		if manifestURL == "" || m.cfg == nil {
			m.status = i18n.T("usage.skill_sync")
			return nil
		}
		return func() tea.Msg {
			entries, err := skill.SyncFromManifest(manifestURL, m.cfg.DataDir)
			return skillSyncMsg{entries: entries, err: err}
		}
	default:
		m.status = i18n.T("status.unknown_command", cmd)
	}
	return nil
}

func (m *Model) statusSummary() string {
	workspace := m.workspaceDir()
	theme := "default"
	lang := string(i18n.CurrentLang())
	if m.cfg != nil {
		if m.cfg.Theme != "" {
			theme = m.cfg.Theme
		}
		if m.cfg.Lang != "" {
			lang = m.cfg.Lang
		}
	}
	return fmt.Sprintf("%s · theme: %s · lang: %s",
		i18n.T("status.summary", m.sessionID, workspace, len(m.messages), m.statusTokenCount(), m.statusCost()),
		theme, lang)
}

func (m *Model) statusTokenCount() string {
	if m.tokenUsage != nil {
		if v, ok := m.tokenUsage["totalTokens"]; ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	if tokens, ok := currentOutputTokens(m.tokenUsage, m.context); ok {
		return fmt.Sprint(tokens)
	}
	if m.context != nil {
		if v, ok := m.context["totalTokens"]; ok && v != nil {
			return fmt.Sprint(v)
		}
	}
	return "-"
}

func (m *Model) statusCost() string {
	if m.cost == nil {
		return "-"
	}
	if v, ok := m.cost["estimatedYuan"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return "-"
}

func (m *Model) pluginCommand(name string, args []string) tea.Cmd {
	if m.pluginManager == nil {
		return nil
	}
	if strings.HasPrefix(name, "/plugin:") {
		rest := strings.TrimPrefix(name, "/plugin:")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 {
			m.status = i18n.T("usage.plugin")
			return nil
		}
		pluginName, command := parts[0], parts[1]
		if actual, ok := m.pluginCommandName(pluginName, command); ok {
			return m.pluginRunCmd(actual, args)
		}
		return nil
	}

	command := strings.TrimPrefix(name, "/")
	if command == "" || isBuiltinCommandName(command) {
		return nil
	}
	if actual, ok := m.pluginCommandName("", command); ok {
		return m.pluginRunCmd(actual, args)
	}
	return nil
}

// pluginCommandName 在插件管理器中查找命令的实际拼写；pluginName 为空时匹配任意插件。
// 比较时忽略大小写，返回 manifest 中的原始命令名以便传给 RunCommand。
func (m *Model) pluginCommandName(pluginName, command string) (string, bool) {
	if m.pluginManager == nil {
		return "", false
	}
	command = strings.TrimPrefix(strings.TrimSpace(command), "/")
	for _, p := range m.pluginManager.List() {
		if pluginName != "" && !strings.EqualFold(p.Manifest.Name, pluginName) {
			continue
		}
		for _, c := range p.Manifest.Commands {
			c = strings.TrimPrefix(strings.TrimSpace(c), "/")
			if strings.EqualFold(c, command) {
				return c, true
			}
		}
	}
	return "", false
}

// pluginRunCmd 异步执行插件命令，避免阻塞 UI。
func (m *Model) pluginRunCmd(command string, args []string) tea.Cmd {
	return func() tea.Msg {
		if m.pluginManager == nil {
			return pluginResultMsg{err: fmt.Errorf("plugin manager unavailable")}
		}
		out, err := m.pluginManager.RunCommand(command, args)
		return pluginResultMsg{output: out, err: err}
	}
}

// isBuiltinCommandName 报告名称是否为内置命令（用于避免插件短别名覆盖内置命令）。
func isBuiltinCommandName(name string) bool {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	for _, cmd := range commandPaletteCommands {
		if strings.TrimPrefix(cmd, "/") == name {
			return true
		}
	}
	return false
}

func (m *Model) renderCommandPalette() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(60).
		MaxHeight(20)

	var b strings.Builder
	b.WriteString(i18n.T("panel.command_title") + "\n")
	mode := "basic"
	if m.showAllCommands {
		mode = "all"
	}
	b.WriteString("> " + m.commandPaletteQuery + "  [" + mode + "]\n")
	if len(m.commandPaletteFiltered) == 0 {
		b.WriteString(i18n.T("panel.no_match") + "\n")
	} else {
		for i, cmd := range m.commandPaletteFiltered {
			marker := " "
			if i == m.commandPaletteCursor {
				marker = ">"
			}
			b.WriteString(fmt.Sprintf("%s %s\n", marker, cmd))
		}
	}
	return "\n" + style.Render(b.String())
}

func (m *Model) exportSession() (string, error) {
	dir := filepath.Join(m.cfg.DataDir, "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := filepath.Join(dir, sanitizeSessionID(m.sessionID))

	md := "# " + m.sessionID + "\n\n"
	for _, msg := range m.messages {
		md += "## " + msg.Role + "\n\n" + msg.Content + "\n\n"
	}
	if err := os.WriteFile(base+".md", []byte(md), 0o600); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(session.Entry{ID: m.sessionID, Messages: m.messages, Context: m.context, Cost: m.cost, Diffs: m.diffs, Todos: m.todos, Tools: m.tools}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".json", data, 0o600); err != nil {
		return "", err
	}
	return base + ".md, " + base + ".json", nil
}
