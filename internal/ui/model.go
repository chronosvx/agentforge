// Package ui 实现 agentforge 的 Bubble Tea 聊天界面。
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/contextx"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/plugin"
	"github.com/chronosvx/agentforge/internal/search"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/skill"
)

// ChatBackend 是 UI 层对聊天后端的抽象，支持 mock/tui channel/真实 DSH API。
type ChatBackend interface {
	Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*client.Snapshot, error)
	Subscribe(ctx context.Context, channelID string, after int64) (<-chan client.Event, error)
	Invoke(ctx context.Context, channelID, method string, args []any) (any, error)
	Close(ctx context.Context, channelID string) error
	CloseConn() error
}

// RemoteSessionLister 是可选接口：后端能列出远端会话。
type RemoteSessionLister interface {
	ListSessions(ctx context.Context) ([]session.Entry, error)
}

// Canceller 是可选接口：后端能取消当前生成。
type Canceller interface {
	Cancel(ctx context.Context, channelID string) error
}

// ModelSwitcher 是可选接口：后端能显示/切换当前模型（Provider 直连模式）。
type ModelSwitcher interface {
	ProviderName() string
	Model() string
	SetModel(string) error
}

// sidebarWidth 是左侧会话工作区宽度。
const sidebarWidth = 26

// rightPanelWidth 是右侧常驻面板宽度。
const rightPanelWidth = 32

// persistInterval 是本地会话落盘的最短间隔。
const persistInterval = 500 * time.Millisecond

// narrowWidthThreshold 小于该宽度时自动隐藏右侧面板/分屏。
const narrowWidthThreshold = 80

// panelKind 表示当前右侧/底部信息面板类型。
type panelKind int

const (
	panelNone panelKind = iota
	panelContext
	panelCost
	panelSessions
	panelDiff
	panelTodos
	panelTools
	panelSkills
	panelSearch
	panelPlugins
	panelAgent
	panelMCP
	panelExec
)

// commandPaletteCommands 是命令面板中可搜索并执行的本地命令。
var commandPaletteCommands = []string{
	"/context", "/cost", "/diff", "/todos", "/tools", "/skills", "/sessions",
	"/plugin", "/search", "/find", "/right", "/split", "/model", "/new", "/rename", "/export",
	"/delete", "/close", "/skill-install", "/skill-sync", "/help",
	"/agent", "/agent-new", "/agent-reset", "/checkpoint", "/rewind", "/mcp", "/mcp-call",
	"/exec",
}

// Model 是 Bubble Tea 的根模型。
type Model struct {
	cfg        *config.Config
	cl         ChatBackend
	store      *session.Store
	asyncStore *session.AsyncStore
	auditFn    func(event string, fields map[string]any)

	sessionID   string
	channelID   string
	events      <-chan client.Event
	eventCancel context.CancelFunc

	messages []client.Message
	status   string
	err      error

	context      map[string]any
	cost         map[string]any
	tokenUsage   map[string]any
	diffs        []any
	todos        []any
	tools        []any
	skills       []skill.Entry
	localSkills  []skill.Entry
	remoteSkills []skill.Entry
	sessions     []session.Entry
	allSessions  []session.Entry
	tabs         []session.Entry
	tabIndex     int
	panel        panelKind
	sessionQuery string

	commandPaletteOpen     bool
	commandPaletteQuery    string
	commandPaletteCursor   int
	commandPaletteFiltered []string

	searchResults []search.Result
	searchCursor  int

	pluginManager *plugin.Manager
	plugins       []plugin.Plugin
	pluginCursor  int

	agentSession *agent.AgentSession
	agentCursor  int
	mcpClient    *mcp.Client
	mcpTools     []mcp.Tool
	mcpCursor    int
	window       *contextx.Window
	checkpoints  *contextx.CheckpointStore

	execOpen     bool
	executorType string
	execCmd      string
	execResult   string
	execErr      error
	execRunning  bool

	diffCursor    int
	diffDecisions map[int]string
	pendingApply  bool
	lastBackupDir string
	applyFiles    []string
	applyCursor   int
	applySelected map[string]bool
	skillCursor   int
	skillDetail   bool
	sessionEpoch  int
	stopRequested bool
	pendingNew    bool

	costLimit   float64
	costAlerted bool

	input          textarea.Model
	viewport       viewport.Model
	ready          bool
	width          int
	height         int
	rightPanel     bool
	splitEnabled   bool
	splitSessionID string

	// Virtual scrolling / incremental rendering state.
	dirty                 bool
	followBottom          bool
	scrollOffset          int
	lastRenderedCount     int
	lastRenderedStart     int
	lastRenderedWidth     int
	lastRenderedHeight    int
	viewportContentBuilds int

	persistDirty  bool
	lastPersistAt time.Time
}

// New 创建一个新的 UI Model。
//
// auditFn 是可选审计回调；不传或传 nil 时 UI 不记录审计事件。
func New(cfg *config.Config, cl ChatBackend, store *session.Store, auditFn ...func(event string, fields map[string]any)) Model {
	lang := i18n.LangZH
	if cfg != nil && cfg.Lang != "" {
		lang = i18n.Lang(cfg.Lang)
	}
	i18n.SetLang(lang)

	input := textarea.New()
	input.Placeholder = i18n.T("input.placeholder")
	input.SetWidth(80)
	input.SetHeight(3)
	input.ShowLineNumbers = false
	input.Focus()

	m := Model{
		cfg:           cfg,
		cl:            cl,
		store:         store,
		sessionID:     cfg.SessionID,
		status:        i18n.T("status.connecting"),
		input:         input,
		viewport:      viewport.New(80, 20),
		diffDecisions: map[int]string{},
		costLimit:     cfg.CostLimit,
		window:        contextx.NewWindow(),
		checkpoints:   contextx.NewCheckpointStore(),
		followBottom:  true,
	}
	if len(auditFn) > 0 && auditFn[0] != nil {
		m.auditFn = auditFn[0]
	}

	// 初始化必须在 New 中完成，Bubble Tea 不会保留 Init 值接收者上的修改。
	if cfg != nil {
		if locals, err := skill.LoadLocal(cfg.DataDir); err == nil {
			m.localSkills = locals
			m.skills = append([]skill.Entry(nil), locals...)
		}
	}
	if m.sessionID != "" && m.store != nil {
		if entry, err := m.store.Load(m.sessionID); err == nil {
			m.messages = entry.Messages
			m.context = entry.Context
			m.cost = entry.Cost
			m.diffs = entry.Diffs
			m.todos = entry.Todos
			m.tools = entry.Tools
			m.syncSkillsFromMessages()
			m.syncWindowFromMessages()
			m.status = "resumed: " + m.sessionID
		}
	}
	if m.sessionID == "" {
		m.sessionID = fmt.Sprintf("session-%d", time.Now().UnixMilli())
	}
	if m.store != nil {
		if list, err := m.store.List(); err == nil {
			m.sessions = append([]session.Entry(nil), list...)
			m.allSessions = append([]session.Entry(nil), list...)
		}
	}

	m.pluginManager = plugin.NewManager(cfg.Workspace)
	if err := m.pluginManager.LoadDir(filepath.Join(cfg.DataDir, "plugins")); err == nil {
		m.plugins = m.pluginManager.List()
	}

	return m
}

// SetAudit 设置审计回调；传 nil 可关闭审计。
func (m *Model) SetAudit(auditFn func(event string, fields map[string]any)) {
	m.auditFn = auditFn
}

// SetAuditLog 设置审计日志器；传 nil 可关闭审计。
func (m *Model) SetAuditLog(l *security.AuditLog) {
	if l == nil {
		m.auditFn = nil
		return
	}
	m.auditFn = func(event string, fields map[string]any) {
		_ = l.Append(event, fields)
	}
}

// auditEvent 调用审计回调记录一条 UI 关键操作事件；未注入审计器时忽略。
func (m *Model) auditEvent(event string, fields map[string]any) {
	if m.auditFn == nil {
		return
	}
	m.auditFn(event, fields)
}

// Init 返回启动时执行的命令。
func (m Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.connectCmd(), m.loadWorkspaceCmd())
}

// Update 处理所有消息。
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	skipInputUpdate := false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applyLayout()
		m.ready = true

	case workspaceLoadedMsg:
		m.tabs = msg.tabs
		m.tabIndex = 0
		for i, s := range m.tabs {
			if s.ID == m.sessionID {
				m.tabIndex = i
				break
			}
		}

	case connectedMsg:
		if msg.epoch != m.sessionEpoch {
			// 来自旧会话的迟到连接结果，忽略。
			if msg.cancel != nil {
				msg.cancel()
			}
			break
		}
		if m.eventCancel != nil {
			m.eventCancel()
		}
		m.eventCancel = msg.cancel
		m.channelID = msg.snapshot.ChannelID
		m.events = msg.events
		m.err = nil
		if m.cfg != nil && m.cfg.APIURL != "" && msg.snapshot.ChannelID != "" {
			m.sessionID = msg.snapshot.ChannelID
		}
		m.status = "connected: " + m.channelID
		m.syncMessagesFromState(msg.snapshot.State)
		m.syncSkillsFromMessages()
		m.syncUsageFromState(msg.snapshot.State)
		m.syncTokenUsageFromState(msg.snapshot.State)
		m.syncDiffsFromState(msg.snapshot.State)
		m.syncTodosFromState(msg.snapshot.State)
		m.syncToolsFromState(msg.snapshot.State)
		m.syncSkillsFromState(msg.snapshot.State)
		m.checkCostAlert()
		m.persist()
		cmds = append(cmds, m.waitEventCmd())

	case eventMsg:
		if msg.channelID != "" && m.channelID != "" && msg.channelID != m.channelID {
			// 来自旧 channel 的迟到事件，忽略。
			cmds = append(cmds, m.waitEventCmd())
			break
		}
		m.status = "streaming..."
		m.syncMessagesFromState(msg.event.State)
		m.syncSkillsFromMessages()
		m.syncUsageFromState(msg.event.State)
		m.syncTokenUsageFromState(msg.event.State)
		m.syncDiffsFromState(msg.event.State)
		m.syncTodosFromState(msg.event.State)
		m.syncToolsFromState(msg.event.State)
		m.syncSkillsFromState(msg.event.State)
		m.checkCostAlert()
		if m.pluginManager != nil {
			_ = m.pluginManager.Emit(plugin.HookOnMessage, map[string]any{
				"sessionId": m.sessionID,
				"event":     msg.event,
			})
		}
		m.persist()
		cmds = append(cmds, m.waitEventCmd())

	case disconnectedMsg:
		if msg.epoch != m.sessionEpoch {
			break
		}
		if m.eventCancel != nil {
			m.eventCancel()
			m.eventCancel = nil
		}
		m.status = i18n.T("status.disconnected")
		m.err = fmt.Errorf("event stream closed")
		cmds = append(cmds, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return reconnectMsg{}
		}))

	case reconnectMsg:
		m.status = "reconnecting..."
		cmds = append(cmds, m.connectCmd())

	case editorDoneMsg:
		if msg.err != nil {
			m.status = "editor error: " + msg.err.Error()
		} else {
			m.input.SetValue(msg.content)
			m.status = "edited"
		}

	case execResultMsg:
		m.execRunning = false
		if msg.err != nil {
			m.execErr = msg.err
			m.status = "exec error: " + msg.err.Error()
		} else {
			m.execResult = msg.result
			m.status = "exec done"
		}

	case skillInstallMsg:
		if msg.err != nil {
			m.status = "skill install error: " + msg.err.Error()
		} else {
			m.refreshLocalSkills()
			m.status = fmt.Sprintf("installed %d skill(s)", len(msg.entries))
		}
		m.skillDetail = false

	case skillSyncMsg:
		if msg.err != nil {
			m.status = "skill sync error: " + msg.err.Error()
		} else {
			m.refreshLocalSkills()
			m.status = fmt.Sprintf("synced %d skill(s)", len(msg.entries))
		}
		m.skillDetail = false

	case pluginResultMsg:
		if msg.err != nil {
			m.status = "plugin error: " + msg.err.Error()
		} else if msg.output != "" {
			output := sanitizeTerminalText(strings.ReplaceAll(strings.TrimSpace(msg.output), "\n", " "))
			m.status = "plugin: " + truncateText(output, 120)
		} else {
			m.status = "plugin: ok"
		}

	case mcpConnectMsg:
		if msg.err != nil {
			m.status = "mcp error: " + msg.err.Error()
			break
		}
		if m.mcpClient != nil {
			_ = m.mcpClient.Close()
		}
		m.mcpClient = msg.client
		m.mcpTools = msg.tools
		m.mcpCursor = 0
		m.panel = panelMCP
		m.status = fmt.Sprintf("mcp connected: %s (%d tools)", msg.url, len(msg.tools))

	case mcpCallMsg:
		if msg.err != nil {
			m.status = "mcp call error: " + msg.err.Error()
			break
		}
		data, err := json.Marshal(msg.result)
		if err != nil {
			data = []byte(fmt.Sprint(msg.result))
		}
		m.status = "mcp result: " + truncateText(sanitizeTerminalText(string(data)), 160)

	case sentMsg:
		m.status = "sent"
		m.persist()

	case switchSessionMsg:
		if m.eventCancel != nil {
			m.eventCancel()
			m.eventCancel = nil
		}
		if m.channelID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = m.cl.Close(ctx, m.channelID)
			cancel()
		}
		m.sessionEpoch++
		m.sessionID = msg.id
		m.auditEvent("session.switch", map[string]any{"sessionId": msg.id})
		m.channelID = ""
		m.events = nil
		m.panel = panelNone
		m.err = nil
		m.status = "switching to " + msg.id
		m.messages = nil
		m.dirty = true
		m.followBottom = true
		m.syncWindowFromMessages()
		m.context = nil
		m.cost = nil
		m.diffs = nil
		m.todos = nil
		m.tools = nil
		m.skills = append([]skill.Entry(nil), m.localSkills...)
		m.remoteSkills = nil
		m.skillCursor = 0
		m.skillDetail = false
		if m.store != nil {
			if entry, err := m.store.Load(msg.id); err == nil {
				m.messages = entry.Messages
				m.dirty = true
				m.context = entry.Context
				m.cost = entry.Cost
				m.diffs = entry.Diffs
				m.todos = entry.Todos
				m.tools = entry.Tools
				m.syncSkillsFromMessages()
			}
		}
		m.syncWindowFromMessages()
		for i, s := range m.tabs {
			if s.ID == m.sessionID {
				m.tabIndex = i
				break
			}
		}
		cmds = append(cmds, m.connectCmd())

	case errMsg:
		m.err = msg.err
		m.status = "error: " + msg.err.Error()

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			m.handleMouseWheel(msg)
			if (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) &&
				(m.commandPaletteOpen || m.mouseWheelPanelOpen()) {
				skipInputUpdate = true
			}
		}

	case tea.KeyMsg:
		// 命令面板打开时，先拦截面板内按键。
		if m.commandPaletteOpen {
			if handled, paletteCmd := m.handleCommandPaletteKey(msg); handled {
				skipInputUpdate = true
				if paletteCmd != nil {
					cmds = append(cmds, paletteCmd)
				}
				if !m.pendingNew {
					return m, tea.Batch(cmds...)
				}
			}
		}
		if !skipInputUpdate {
			// Ctrl+←/→ 切换会话 Tab。
			switch msg.String() {
			case "ctrl+right":
				if len(m.tabs) > 0 {
					next := (m.tabIndex + 1) % len(m.tabs)
					target := m.tabs[next].ID
					if target != m.sessionID {
						cmds = append(cmds, switchSessionCmd(target))
					} else {
						m.tabIndex = next
					}
				}
				break
			case "ctrl+left":
				if len(m.tabs) > 0 {
					prev := (m.tabIndex - 1 + len(m.tabs)) % len(m.tabs)
					target := m.tabs[prev].ID
					if target != m.sessionID {
						cmds = append(cmds, switchSessionCmd(target))
					} else {
						m.tabIndex = prev
					}
				}
				break
			}
			// 右侧 Overview 面板内按数字打开对应详情面板。
			if m.rightPanel {
				if m.handleRightPanelKey(msg.String()) {
					break
				}
			}
			// 双会话并排视图内切换右侧会话。
			if m.splitEnabled {
				if msg.String() == "s" && m.splitSessionID != "" && m.splitSessionID != m.sessionID {
					cmds = append(cmds, switchSessionCmd(m.splitSessionID))
					break
				}
				if m.handleSplitKey(msg.String()) {
					break
				}
			}
			// Diff Review 面板内的键盘操作：j/k 移动，a 接受，r 拒绝。
			if m.panel == panelDiff {
				if m.handleDiffKey(msg.String()) {
					break
				}
			}
			// Skills 面板内的键盘操作：j/k 选择，space 切换，Enter 查看详情。
			if m.panel == panelSkills {
				if m.handleSkillKey(msg.String()) {
					break
				}
			}
			// Agent 审批面板内的键盘操作：j/k 选择，a 审批，r 拒绝，e 执行，t 测试通过。
			if m.panel == panelAgent {
				if m.handleAgentKey(msg.String()) {
					break
				}
			}
			// MCP 工具列表面板 j/k 移动。
			if m.panel == panelMCP {
				switch msg.String() {
				case "j", "down":
					if m.mcpCursor < len(m.mcpTools)-1 {
						m.mcpCursor++
					}
					break
				case "k", "up":
					if m.mcpCursor > 0 {
						m.mcpCursor--
					}
					break
				}
			}
			// 搜索结果面板 j/k 移动。
			if m.panel == panelSearch {
				switch msg.String() {
				case "j", "down":
					if m.searchCursor < len(m.searchResults)-1 {
						m.searchCursor++
					}
					break
				case "k", "up":
					if m.searchCursor > 0 {
						m.searchCursor--
					}
					break
				}
			}
			// 插件面板 j/k 移动。
			if m.panel == panelPlugins {
				switch msg.String() {
				case "j", "down":
					if m.pluginCursor < len(m.plugins)-1 {
						m.pluginCursor++
					}
					break
				case "k", "up":
					if m.pluginCursor > 0 {
						m.pluginCursor--
					}
					break
				}
			}
			// 在会话面板中按数字键切换本地会话。
			if m.panel == panelSessions {
				if idx, ok := digitIndex(msg.String()); ok && idx >= 0 && idx < len(m.sessions) {
					target := m.sessions[idx].ID
					if target != m.sessionID {
						cmds = append(cmds, switchSessionCmd(target))
					}
					break
				}
			}
			switch msg.String() {
			case "ctrl+p":
				m.openCommandPalette()
				return m, tea.Batch(cmds...)
			case "ctrl+c":
				if !m.stopRequested {
					m.stopRequested = true
					m.status = i18n.T("status.stop_requested")
					if m.channelID != "" {
						if canceller, ok := m.cl.(Canceller); ok {
							cmds = append(cmds, cancelCmd(canceller, m.channelID))
						}
					}
					break
				}
				if m.eventCancel != nil {
					m.eventCancel()
					m.eventCancel = nil
				}
				m.flushPersist()
				return m, tea.Quit
			case "ctrl+e":
				cmds = append(cmds, m.editCmd())
				break
			case "ctrl+j":
				m.input.InsertString("\n")
				break
			case "enter":
				text := strings.TrimSpace(m.input.Value())
				if text == "" {
					break
				}
				m.input.SetValue("")
				if strings.HasPrefix(text, "/") {
					if cmd := m.handleCommand(text); cmd != nil {
						cmds = append(cmds, cmd)
					}
					break
				}
				m.messages = append(m.messages, client.Message{Role: "user", Content: text})
				m.dirty = true
				m.syncWindowFromMessages()
				cmds = append(cmds, m.sendCmd(text))
				if m.splitEnabled && m.splitSessionID != "" && m.splitSessionID != m.sessionID {
					cmds = append(cmds, m.sendCmdTo(text, m.splitSessionID))
				}
			}
		}
	}

	if m.pendingNew {
		m.pendingNew = false
		previousSessionID := m.sessionID
		if m.eventCancel != nil {
			m.eventCancel()
			m.eventCancel = nil
		}
		if m.channelID != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = m.cl.Close(ctx, m.channelID)
			cancel()
		}
		m.sessionEpoch++
		m.sessionID = ""
		m.auditEvent("session.new", map[string]any{"previousSessionId": previousSessionID})
		m.channelID = ""
		m.events = nil
		m.messages = nil
		m.dirty = true
		m.followBottom = true
		m.syncWindowFromMessages()
		m.context = nil
		m.cost = nil
		m.diffs = nil
		m.todos = nil
		m.tools = nil
		m.skills = append([]skill.Entry(nil), m.localSkills...)
		m.remoteSkills = nil
		m.skillCursor = 0
		m.skillDetail = false
		m.panel = panelNone
		m.err = nil
		m.status = "new session..."
		cmds = append(cmds, m.connectCmd())
	}

	// 文本框和 viewport 的常规更新。
	if !skipInputUpdate {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		cmds = append(cmds, cmd)

		// 虚拟滚动由 UI 层自己处理，避免 viewport 只对窗口切片做局部滚动。
		if m.ready && isViewportScrollMsg(msg) {
			m.scrollByViewportMsg(msg)
		} else {
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	if m.ready {
		if !isViewportScrollMsg(msg) {
			if !m.followBottom {
				m.followBottom = true
				m.scrollOffset = m.bottomScrollOffset()
				m.dirty = true
			} else if m.dirty {
				m.scrollOffset = m.bottomScrollOffset()
			}
		}
		m.renderMessages()
	}

	return m, tea.Batch(cmds...)
}

// isViewportScrollMsg 判断消息是否为用户主动滚动 viewport 的操作。
// 这类操作后不要强制跳回底部，以便用户能停留在历史消息位置。
func isViewportScrollMsg(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return msg.Action == tea.MouseActionPress &&
			(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown)
	case tea.KeyMsg:
		switch msg.String() {
		case "pgup", "pgdown", "ctrl+u", "ctrl+d":
			return true
		}
	}
	return false
}

// openCommandPalette 打开命令面板并重置搜索状态。
func (m *Model) openCommandPalette() {
	m.commandPaletteOpen = true
	m.commandPaletteQuery = ""
	m.commandPaletteCursor = 0
	m.commandPaletteFiltered = m.commandPaletteEntries()
}

// commandPaletteEntries 返回命令面板可执行列表：内置命令 + 已加载插件的命令。
func (m *Model) commandPaletteEntries() []string {
	entries := append([]string(nil), commandPaletteCommands...)
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

// mouseWheelPanelOpen 报告当前是否打开了可用滚轮移动光标的列表面板。
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
	case "j", "down":
		if len(m.commandPaletteFiltered) > 0 && m.commandPaletteCursor < len(m.commandPaletteFiltered)-1 {
			m.commandPaletteCursor++
		}
		return true, nil
	case "k", "up":
		if m.commandPaletteCursor > 0 {
			m.commandPaletteCursor--
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

// applyLayout 根据窗口宽度和左右侧栏状态重新计算主区域尺寸。
func (m *Model) applyLayout() {
	if m.width <= 0 || m.height <= 0 {
		return
	}
	mainWidth := m.width - 2
	if m.width < narrowWidthThreshold {
		// 窄屏下自动隐藏右侧常驻面板和分屏，优先保证主聊天区可读。
		m.rightPanel = false
		m.splitEnabled = false
		m.splitSessionID = ""
	}
	if len(m.tabs) > 0 {
		mainWidth -= sidebarWidth + 2
	}
	if m.rightPanel {
		mainWidth -= rightPanelWidth + 2
	}
	if m.splitEnabled {
		mainWidth -= rightPanelWidth + 2
	}
	if mainWidth < 20 {
		mainWidth = m.width - 2
	}
	m.viewport.Width = mainWidth
	height := m.height - 5
	if height < 1 {
		height = 1
	}
	m.viewport.Height = height
	m.input.SetWidth(mainWidth - 2)
	m.dirty = true
}

// View 渲染界面。
func (m Model) View() string {
	if !m.ready {
		return i18n.T("app.loading") + "\n"
	}

	m.renderMessages()

	style := lipgloss.NewStyle().Padding(0, 1)
	inputView := m.input.View()
	if m.err != nil {
		inputView += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(sanitizeTerminalText(m.err.Error()))
	}

	statusPrefix := "• "
	statusText := sanitizeTerminalText(m.status)
	switch {
	case m.err != nil:
		statusPrefix = "✖ "
	case (m.costAlerted || strings.Contains(statusText, "error") || strings.Contains(statusText, "⚠")) && !strings.HasPrefix(statusText, "⚠"):
		statusPrefix = "⚠ "
	}
	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(fmt.Sprintf(" %s%s | session: %s | channel: %s | /context /cost /diff /todos /tools /skills /sessions /right /split /help ", statusPrefix, statusText, sanitizeTerminalText(m.sessionID), sanitizeTerminalText(m.channelID)))

	panelView := m.renderPanel()
	paletteView := ""
	if m.commandPaletteOpen {
		paletteView = m.renderCommandPalette()
	}
	tabBar := m.renderTabs()
	main := style.Render(tabBar + "\n" + m.viewport.View() + panelView + paletteView + "\n" + inputView + "\n" + statusLine)

	parts := make([]string, 0, 3)
	if sidebar := m.renderSidebar(); sidebar != "" {
		parts = append(parts, sidebar)
	}
	parts = append(parts, main)
	if m.rightPanel {
		parts = append(parts, m.renderRightPanel())
	}
	if m.splitEnabled {
		parts = append(parts, m.renderSplitPanel())
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// connectCmd 打开会话并订阅事件流。
func (m Model) connectCmd() tea.Cmd {
	epoch := m.sessionEpoch
	return func() tea.Msg {
		// Open 使用短超时；Subscribe 返回的流需要独立的长生命周期 context，
		// 不能随 Open 的超时一起取消，否则 dshapi 后端的转发 goroutine 会立刻退出。
		openCtx, openCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer openCancel()
		snap, err := m.cl.Open(openCtx, m.cfg.Workspace, m.sessionID, nil)
		if err != nil {
			return errMsg{err}
		}
		streamCtx, streamCancel := context.WithCancel(context.Background())
		type subscribeResult struct {
			events <-chan client.Event
			err    error
		}
		resultCh := make(chan subscribeResult, 1)
		go func() {
			events, err := m.cl.Subscribe(streamCtx, snap.ChannelID, snap.Version)
			resultCh <- subscribeResult{events: events, err: err}
		}()
		select {
		case res := <-resultCh:
			if res.err != nil {
				streamCancel()
				return errMsg{res.err}
			}
			return connectedMsg{snapshot: snap, events: res.events, cancel: streamCancel, epoch: epoch}
		case <-time.After(10 * time.Second):
			streamCancel()
			return errMsg{fmt.Errorf("subscribe timed out")}
		}
	}
}

// loadWorkspaceCmd 加载用于 Tab 工作区的会话列表（远端优先，本地兜底）。
func (m Model) loadWorkspaceCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if lister, ok := m.cl.(RemoteSessionLister); ok {
			if list, err := lister.ListSessions(ctx); err == nil {
				return workspaceLoadedMsg{tabs: limitTabs(list, 8)}
			}
		}
		if m.store != nil {
			if list, err := m.store.List(); err == nil {
				return workspaceLoadedMsg{tabs: limitTabs(list, 8)}
			}
		}
		return workspaceLoadedMsg{}
	}
}

// limitTabs 保留最近的 max 个会话作为 Tab。
func limitTabs(entries []session.Entry, max int) []session.Entry {
	if len(entries) <= max {
		return entries
	}
	return entries[:max]
}

// waitEventCmd 从事件流读取下一条事件。
func (m Model) waitEventCmd() tea.Cmd {
	epoch := m.sessionEpoch
	return func() tea.Msg {
		if m.events == nil {
			return disconnectedMsg{epoch: epoch}
		}
		ev, ok := <-m.events
		if !ok {
			return disconnectedMsg{epoch: epoch}
		}
		return eventMsg{event: ev, channelID: ev.ChannelID}
	}
}

// sendCmd 发送用户消息。
func (m Model) sendCmd(text string) tea.Cmd {
	return m.sendCmdTo(text, m.channelID)
}

// sendCmdTo 向指定 channel 发送消息。
func (m Model) sendCmdTo(text, channelID string) tea.Cmd {
	return func() tea.Msg {
		if channelID == "" {
			return errMsg{fmt.Errorf("not connected")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := m.cl.Invoke(ctx, channelID, "chat", []any{text})
		if err != nil {
			return errMsg{err}
		}
		m.auditEvent("message.sent", map[string]any{
			"sessionId": m.sessionID,
			"channelId": channelID,
		})
		return sentMsg{text: text}
	}
}

// cancelCmd 请求取消当前生成。
func cancelCmd(canceller Canceller, channelID string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := canceller.Cancel(ctx, channelID); err != nil {
			return errMsg{err}
		}
		return sentMsg{text: ""}
	}
}

// editCmd 使用 $EDITOR 编辑当前输入内容。
func (m Model) editCmd() tea.Cmd {
	return func() tea.Msg {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		tmp, err := os.CreateTemp("", "agentforge-edit-*.txt")
		if err != nil {
			return editorDoneMsg{err: err}
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if _, err := tmp.WriteString(m.input.Value()); err != nil {
			tmp.Close()
			return editorDoneMsg{err: err}
		}
		if err := tmp.Close(); err != nil {
			return editorDoneMsg{err: err}
		}

		cmd := exec.Command(editor, tmpPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return editorDoneMsg{err: err}
		}

		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return editorDoneMsg{err: err}
		}
		return editorDoneMsg{content: string(data)}
	}
}

// switchSessionCmd 请求切换到指定本地会话。
func switchSessionCmd(id string) tea.Cmd {
	return func() tea.Msg {
		return switchSessionMsg{id: id}
	}
}

// digitIndex 将 "0"-"9" 转为索引；非数字返回 ok=false。
func digitIndex(s string) (int, bool) {
	if len(s) != 1 || s[0] < '0' || s[0] > '9' {
		return 0, false
	}
	return int(s[0] - '0'), true
}

// syncMessagesFromState 从 Channel state 中同步 messages。
func (m *Model) syncMessagesFromState(state map[string]any) {
	if state == nil {
		return
	}
	raw, ok := state["messages"]
	if !ok {
		return
	}
	list, ok := raw.([]any)
	if !ok {
		return
	}

	messages := make([]client.Message, 0, len(list))
	for _, item := range list {
		mm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		content, _ := mm["content"].(string)
		if role != "" || content != "" {
			messages = append(messages, client.Message{Role: role, Content: content})
		}
	}
	if !messagesEqual(m.messages, messages) {
		m.messages = messages
		m.dirty = true
		m.syncWindowFromMessages()
	}
}

// syncWindowFromMessages 将当前 UI 消息列表同步到 contextx.Window。
// Window 用于 Checkpoint/Rewind 快照，不直接替代 m.messages。
func (m *Model) syncWindowFromMessages() {
	if m.window == nil {
		m.window = contextx.NewWindow()
	}
	// Window 没有 Replace/Reset 方法，这里通过 Rewind 到空 checkpoint 再逐条 Add。
	m.window.Rewind(contextx.Checkpoint{})
	for _, msg := range m.messages {
		m.window.Add(msg)
	}
}

// syncUsageFromState 从 Channel state 中同步 context/cost 信息。
func (m *Model) syncUsageFromState(state map[string]any) {
	if state == nil {
		return
	}
	if v, ok := state["context"]; ok {
		if usage, ok := v.(map[string]any); ok {
			m.context = usage
		}
	}
	if v, ok := state["cost"]; ok {
		if usage, ok := v.(map[string]any); ok {
			m.cost = usage
		}
	}
}

// syncTokenUsageFromState 从 Channel state 中同步 tokenUsage。
func (m *Model) syncTokenUsageFromState(state map[string]any) {
	if state == nil {
		return
	}
	if v, ok := state["tokenUsage"]; ok {
		if usage, ok := v.(map[string]any); ok {
			m.tokenUsage = usage
		}
	}
}

// syncDiffsFromState 从 Channel state 中同步 diff 列表。
func (m *Model) syncDiffsFromState(state map[string]any) {
	if state == nil {
		return
	}
	if v, ok := state["diffs"]; ok {
		if diffs, ok := v.([]any); ok {
			m.diffs = diffs
		}
	}
}

// syncTodosFromState 从 Channel state 中同步 todo 列表。
func (m *Model) syncTodosFromState(state map[string]any) {
	if state == nil {
		return
	}
	if v, ok := state["todos"]; ok {
		if todos, ok := v.([]any); ok {
			m.todos = todos
		}
	}
}

// checkCostAlert 检查成本是否超过阈值。
func (m *Model) checkCostAlert() {
	if m.costLimit <= 0 || m.costAlerted || m.cost == nil {
		return
	}
	if v, ok := m.cost["estimatedYuan"].(float64); ok && v >= m.costLimit {
		m.costAlerted = true
		m.status = fmt.Sprintf("⚠️ cost limit exceeded: %.2f >= %.2f", v, m.costLimit)
	}
}

// syncToolsFromState 从 Channel state 中同步工具调用列表。
func (m *Model) syncToolsFromState(state map[string]any) {
	if state == nil {
		return
	}
	if v, ok := state["tools"]; ok {
		if tools, ok := v.([]any); ok {
			if !toolsEqual(m.tools, tools) {
				m.tools = tools
				m.dirty = true
			}
		}
	}
}

// syncSkillsFromMessages 从已解析的会话消息文本中提取远端 available_skills。
func (m *Model) syncSkillsFromMessages() {
	var remote []skill.Entry
	for _, msg := range m.messages {
		remote = append(remote, skill.ParseAvailableSkills(msg.Content)...)
	}
	if len(remote) > 0 {
		m.remoteSkills = remote
		m.mergeSkills()
	}
}

// syncSkillsFromState 从 Channel state 中同步可能出现的 skills 字段。
func (m *Model) syncSkillsFromState(state map[string]any) {
	if state == nil {
		return
	}
	var remote []skill.Entry
	for _, key := range []string{"skills", "available_skills", "availableSkills"} {
		if v, ok := state[key]; ok {
			remote = append(remote, skill.ParseRemoteValue(v)...)
		}
	}
	if len(remote) > 0 {
		m.remoteSkills = remote
		m.mergeSkills()
	}
}

// mergeSkills 合并本地与远端 Skill，并保留当前 UI 中的启用状态。
func (m *Model) mergeSkills() {
	enabled := make(map[string]bool, len(m.skills))
	for _, s := range m.skills {
		enabled[s.Name] = s.Enabled
	}
	merged := skill.Merge(m.localSkills, m.remoteSkills)
	for i := range merged {
		if v, ok := enabled[merged[i].Name]; ok {
			merged[i].Enabled = v
		}
	}
	m.skills = merged
	if m.skillCursor >= len(m.skills) && m.skillCursor > 0 {
		m.skillCursor = len(m.skills) - 1
	}
}

// refreshLocalSkills 重新扫描本地 Skill 并刷新合并后的 Skills 面板数据。
func (m *Model) refreshLocalSkills() {
	if m.cfg == nil {
		return
	}
	locals, err := skill.LoadLocal(m.cfg.DataDir)
	if err != nil {
		m.status = "skill reload error: " + err.Error()
		return
	}
	m.localSkills = locals
	m.mergeSkills()
}

// skillInstallCmd 返回异步安装 Skill 的 tea.Cmd。本地目录走 InstallFromDir，其余按 git 仓库处理。
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
// 需要异步执行的命令会返回一个 tea.Cmd。
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
			m.panel = panelNone
		} else {
			m.panel = panelContext
		}
	case "/cost":
		if m.panel == panelCost {
			m.panel = panelNone
		} else {
			m.panel = panelCost
		}
	case "/diff":
		if m.panel == panelDiff {
			m.panel = panelNone
		} else {
			m.panel = panelDiff
		}
	case "/todos":
		if m.panel == panelTodos {
			m.panel = panelNone
		} else {
			m.panel = panelTodos
		}
	case "/tools":
		if m.panel == panelTools {
			m.panel = panelNone
		} else {
			m.panel = panelTools
		}
	case "/skills":
		if m.panel == panelSkills {
			m.panel = panelNone
		} else {
			m.panel = panelSkills
		}
		m.skillDetail = false
	case "/plugin":
		if m.panel == panelPlugins {
			m.panel = panelNone
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
				m.panel = panelNone
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
		name := strings.TrimSpace(strings.TrimPrefix(cmd, "/checkpoint"))
		if name == "" || m.checkpoints == nil {
			m.status = "usage: /checkpoint <name>"
			break
		}
		cp := m.window.Checkpoint(name)
		if err := m.checkpoints.Save(cp); err != nil {
			m.status = "checkpoint error: " + err.Error()
		} else {
			m.status = "checkpoint saved: " + name
		}
	case "/rewind":
		name := strings.TrimSpace(strings.TrimPrefix(cmd, "/rewind"))
		if name == "" || m.checkpoints == nil {
			m.status = "usage: /rewind <name>"
			break
		}
		cp, err := m.checkpoints.Load(name)
		if err != nil {
			m.status = "rewind error: " + err.Error()
			break
		}
		m.messages = append([]client.Message(nil), cp.Messages...)
		m.dirty = true
		m.followBottom = true
		m.window.Rewind(cp)
		m.persist()
		m.status = "rewound to checkpoint: " + name
	case "/mcp":
		url := strings.TrimSpace(strings.TrimPrefix(cmd, "/mcp"))
		if url == "" {
			if m.mcpClient == nil {
				m.status = "usage: /mcp <url>"
			} else if m.panel == panelMCP {
				m.panel = panelNone
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
		idx := &search.Index{}
		idx.AddAll(list)
		m.searchResults = idx.Search(query, 50)
		m.searchCursor = 0
		m.panel = panelSearch
		m.status = i18n.T("status.find_results", len(m.searchResults))
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
	case "/sessions":
		if m.panel == panelSessions {
			m.panel = panelNone
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
		m.panel = panelNone
		m.status = i18n.T("help.commands")
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

// newAgentSession 创建/重置一个带示例动作的 Agent 审批会话。
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

// mcpConnectCmd 异步连接 MCP 服务器并列出工具。
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

// mcpCallCmd 异步调用已连接的 MCP 工具。
func (m *Model) mcpCallCmd(name string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		result, err := m.mcpClient.CallTool(ctx, name, args)
		return mcpCallMsg{name: name, result: result, err: err}
	}
}

// pluginCommand 识别命令面板/输入框中的插件命令并返回执行 tea.Cmd。
// 支持 /plugin:<插件名>:<命令> 与不冲突的 /<命令> 短别名。
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

// renderPanel 渲染当前打开的信息面板。
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
	default:
		return ""
	}
}

// renderCommandPalette 渲染命令面板浮层。
func (m *Model) renderCommandPalette() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(60).
		MaxHeight(20)

	var b strings.Builder
	b.WriteString(i18n.T("panel.command_title") + "\n")
	b.WriteString("> " + m.commandPaletteQuery + "\n")
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

// renderTabs 渲染顶部会话 Tab 栏。
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

// renderSidebar 渲染左侧会话工作区（分屏）。
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

// accentColor 根据主题返回主色调。
func (m *Model) accentColor() string {
	if m.cfg != nil && m.cfg.Theme == "catppuccin" {
		return "5"
	}
	return "6"
}

// handleRightPanelKey 处理右侧 Overview 面板的数字快捷键。
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

// handleSplitKey 处理双会话并排视图的切换按键，返回是否已消费。
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

// renderSplitPanel 渲染双会话并排视图中的另一个会话（只读）。
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

// renderRightPanel 渲染右侧常驻信息面板。
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

// renderSearchPanel 渲染历史消息搜索结果。
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

// renderPluginsPanel 渲染插件列表。
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

// renderAgentPanel 渲染 Agent 执行/审批面板。
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

// renderMCPPanel 渲染 MCP 工具列表面板。
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

// handleApplyKey 处理 git apply 前的逐文件确认模式。
func (m *Model) handleApplyKey(key string) bool {
	if len(m.applyFiles) == 0 {
		return false
	}
	switch key {
	case "j", "down":
		if m.applyCursor < len(m.applyFiles)-1 {
			m.applyCursor++
		}
		return true
	case "k", "up":
		if m.applyCursor > 0 {
			m.applyCursor--
		}
		return true
	case " ":
		file := m.applyFiles[m.applyCursor]
		if m.applySelected == nil {
			m.applySelected = map[string]bool{}
		}
		m.applySelected[file] = !m.applySelected[file]
		return true
	case "g":
		m.pendingApply = false
		out, err := m.applyAcceptedDiffsWithFilter(m.applySelected)
		if err != nil {
			m.status = "git apply error: " + err.Error()
		} else {
			m.status = "git apply ok: " + out
		}
		return true
	case "esc":
		m.pendingApply = false
		m.status = "git apply cancelled"
		return true
	}
	return false
}

// handleDiffKey 处理 Diff Review 面板的键盘操作，返回是否已消费。
func (m *Model) handleDiffKey(key string) bool {
	if len(m.diffs) == 0 {
		return false
	}
	if m.pendingApply {
		return m.handleApplyKey(key)
	}
	switch key {
	case "j", "down":
		if m.diffCursor < len(m.diffs)-1 {
			m.diffCursor++
		}
		return true
	case "k", "up":
		if m.diffCursor > 0 {
			m.diffCursor--
		}
		return true
	case "a":
		if m.diffDecisions == nil {
			m.diffDecisions = map[int]string{}
		}
		m.diffDecisions[m.diffCursor] = "accepted"
		m.status = fmt.Sprintf("diff %d accepted", m.diffCursor+1)
		return true
	case "r":
		if m.diffDecisions == nil {
			m.diffDecisions = map[int]string{}
		}
		m.diffDecisions[m.diffCursor] = "rejected"
		m.status = fmt.Sprintf("diff %d rejected", m.diffCursor+1)
		return true
	case "p":
		path, err := m.savePatch()
		if err != nil {
			m.status = "save patch error: " + err.Error()
		} else {
			m.status = "patch saved: " + path
		}
		return true
	case "g":
		if !m.pendingApply {
			m.pendingApply = true
			m.applyFiles = acceptedDiffFiles(m.diffs, m.diffDecisions)
			m.applyCursor = 0
			m.applySelected = map[string]bool{}
			for _, f := range m.applyFiles {
				m.applySelected[f] = true
			}
			m.status = i18n.T("status.apply_confirm")
			return true
		}
		m.pendingApply = false
		out, err := m.applyAcceptedDiffsWithFilter(m.applySelected)
		if err != nil {
			m.status = "git apply error: " + err.Error()
		} else {
			m.status = "git apply ok: " + out
		}
		return true
	case "b":
		if m.lastBackupDir == "" {
			m.status = "no backup available"
			return true
		}
		repoDir := m.cfg.Workspace
		if repoDir == "" {
			repoDir = "."
		}
		if err := m.restoreBackup(m.lastBackupDir, repoDir); err != nil {
			m.status = "rollback error: " + err.Error()
		} else {
			m.status = "rolled back from " + m.lastBackupDir
		}
		return true
	case "esc":
		m.pendingApply = false
		m.status = "git apply cancelled"
		return true
	}
	return false
}

// handleSkillKey 处理 Skills 面板的键盘操作，返回是否已消费。
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

// savePatch 把所有已接受的 diff 写入 <data-dir>/patches/<session>-<time>.patch。
func (m *Model) savePatch() (string, error) {
	var b strings.Builder
	for i, raw := range m.diffs {
		if m.diffDecisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		diff, _ := item["diff"].(string)
		if diff != "" {
			b.WriteString(diff)
			if !strings.HasSuffix(diff, "\n") {
				b.WriteString("\n")
			}
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no accepted diffs")
	}

	dir := filepath.Join(m.cfg.DataDir, "patches")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d.patch", sanitizeSessionID(m.sessionID), time.Now().UnixMilli())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// acceptedDiffFiles 返回所有已接受 diff 涉及的文件路径（去重）。
func acceptedDiffFiles(diffs []any, decisions map[int]string) []string {
	seen := map[string]bool{}
	var out []string
	for i, raw := range diffs {
		if decisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

// cleanDiffPath 校验并规范化来自远端 diff 的文件路径，防止目录穿越。
func cleanDiffPath(file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("empty diff path")
	}
	if strings.ContainsAny(file, `\`) || filepath.IsAbs(file) || strings.HasPrefix(file, "/") {
		return "", fmt.Errorf("invalid diff path: %q", file)
	}
	clean := filepath.Clean(file)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid diff path: %q", file)
	}
	return clean, nil
}

// backupFiles 在 git apply 前把涉及文件备份到 <data-dir>/backups/<session>-<ts>/。
func (m *Model) backupFiles(files []string, repoDir string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to backup")
	}
	backupRoot := filepath.Join(m.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return "", err
	}
	dir := filepath.Join(backupRoot, fmt.Sprintf("%s-%d", sanitizeSessionID(m.sessionID), time.Now().UnixMilli()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, file := range files {
		safe, err := cleanDiffPath(file)
		if err != nil {
			return "", err
		}
		src := filepath.Join(repoDir, safe)
		data, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		dst := filepath.Join(dir, safe)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// restoreBackup 从备份目录恢复文件到工作区。
func (m *Model) restoreBackup(backupDir, repoDir string) error {
	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}
		if _, err := cleanDiffPath(rel); err != nil {
			return err
		}
		dst := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode().Perm())
	})
}

// applyAcceptedDiffs 应用所有已接受的 diff（保留给旧调用/测试）。
func (m *Model) applyAcceptedDiffs() (string, error) {
	return m.applyAcceptedDiffsWithFilter(nil)
}

// applyAcceptedDiffsWithFilter 把已接受的 diff（可按文件过滤）通过 git apply 应用到工作区。
func (m *Model) applyAcceptedDiffsWithFilter(filter map[string]bool) (string, error) {
	var b strings.Builder
	var files []string
	for i, raw := range m.diffs {
		if m.diffDecisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		diff, _ := item["diff"].(string)
		if diff == "" {
			continue
		}
		if filter != nil && !filter[file] {
			continue
		}
		safe, err := cleanDiffPath(file)
		if err != nil {
			return "", err
		}
		b.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			b.WriteString("\n")
		}
		files = append(files, safe)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no accepted diffs selected")
	}

	tmp, err := os.CreateTemp("", "agentforge-apply-*.patch")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	repoDir := m.cfg.Workspace
	if repoDir == "" {
		repoDir = "."
	}

	backupDir, err := m.backupFiles(files, repoDir)
	if err != nil {
		return "", fmt.Errorf("backup before apply: %w", err)
	}
	m.lastBackupDir = backupDir

	check := exec.Command("git", "-C", repoDir, "apply", "--check", tmpPath)
	if out, err := check.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git apply --check: %v: %s", err, strings.TrimSpace(string(out)))
	}

	apply := exec.Command("git", "-C", repoDir, "apply", tmpPath)
	if out, err := apply.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git apply: %v: %s", err, strings.TrimSpace(string(out)))
	}

	m.auditEvent("diff.apply", map[string]any{
		"sessionId": m.sessionID,
		"files":     files,
		"backupDir": backupDir,
	})
	return "patch applied (backup: " + backupDir + ")", nil
}

func sanitizeSessionID(id string) string {
	repl := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return repl.Replace(id)
}

// renderApplyConfirm 渲染 git apply 前的逐文件确认列表。
func (m *Model) renderApplyConfirm() string {
	var b strings.Builder
	b.WriteString(i18n.T("panel.apply_confirm") + "\n")
	for i, file := range m.applyFiles {
		marker := " "
		if i == m.applyCursor {
			marker = ">"
		}
		sel := " "
		if m.applySelected[file] {
			sel = "x"
		}
		b.WriteString(fmt.Sprintf("%s [%s] %s\n", marker, sel, file))
	}
	return b.String()
}

func (m *Model) renderDiffPanel() string {
	if len(m.diffs) == 0 {
		return i18n.T("panel.diff_empty")
	}
	if m.pendingApply {
		return m.renderApplyConfirm()
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.diff_title") + "\n")
	for i, raw := range m.diffs {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		diff, _ := item["diff"].(string)
		marker := " "
		if i == m.diffCursor {
			marker = ">"
		}
		decision := "pending"
		if d, ok := m.diffDecisions[i]; ok {
			decision = d
		}
		b.WriteString(fmt.Sprintf("\n%s[%d] %s  [%s]\n%s\n", marker, i+1, sanitizeTerminalText(file), decision, sanitizeTerminalText(diff)))
	}
	return b.String()
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

// sanitizeTerminalText 去除来自 Agent/服务端文本中的终端控制字符，防止转义注入。
func sanitizeTerminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
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

func truncateText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// renderMessages 将当前可视范围内的消息渲染进 viewport。
// 消息未变化且滚动位置/尺寸未变时不会重建 viewport 内容。
func (m *Model) renderMessages() {
	if len(m.messages) == 0 {
		if m.dirty || m.lastRenderedCount != 0 || m.lastRenderedStart != 0 ||
			m.lastRenderedWidth != m.viewport.Width || m.lastRenderedHeight != m.viewport.Height {
			m.viewportContentBuilds++
			m.viewport.SetContent("")
			m.viewport.SetYOffset(0)
			m.lastRenderedCount = 0
			m.lastRenderedStart = 0
			m.lastRenderedWidth = m.viewport.Width
			m.lastRenderedHeight = m.viewport.Height
			m.dirty = false
		}
		return
	}

	start, end, rendered := m.visibleMessageRange()
	if !m.dirty && m.lastRenderedCount == len(m.messages) && m.lastRenderedStart == start &&
		m.lastRenderedWidth == m.viewport.Width && m.lastRenderedHeight == m.viewport.Height {
		return
	}

	var b strings.Builder
	for _, r := range rendered {
		b.WriteString(r.text)
	}
	if end == len(m.messages) {
		b.WriteString(m.renderToolsBlock())
	}

	m.viewportContentBuilds++
	m.viewport.SetContent(b.String())
	m.viewport.SetYOffset(0)
	m.lastRenderedCount = len(m.messages)
	m.lastRenderedStart = start
	m.lastRenderedWidth = m.viewport.Width
	m.lastRenderedHeight = m.viewport.Height
	m.dirty = false
}

// exportSession 将当前会话导出为 Markdown 和 JSON 到 <data-dir>/exports/。
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

// applySessionFilter 根据 sessionQuery 过滤会话列表。
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

// removeSessionFromWorkspace 从 tabs/sessions 中移除指定会话。
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

// persist 标记需要保存并执行节流落盘：最多每 persistInterval 写一次。
func (m *Model) persist() {
	if m.store == nil || m.sessionID == "" {
		return
	}
	m.persistDirty = true
	if time.Since(m.lastPersistAt) >= persistInterval {
		m.persistNow()
	}
}

// flushPersist 立即把尚未落盘的会话写入本地 store，退出前调用。
func (m *Model) flushPersist() {
	m.persistNow()
	if m.asyncStore != nil {
		_ = m.asyncStore.Flush(context.Background())
	}
}

// FlushPersist 是退出前等待异步会话落盘的导出方法，供 main 调用。
func (m Model) FlushPersist() {
	m.flushPersist()
}

// ClosePersist 关闭异步持久化 writer，并等待已提交任务完成。
func (m Model) ClosePersist() error {
	if m.asyncStore == nil {
		return nil
	}
	return m.asyncStore.Close()
}

// persistNow 执行一次实际保存；未标记脏数据时直接返回。
func (m *Model) persistNow() {
	if !m.persistDirty || m.store == nil || m.sessionID == "" {
		return
	}
	now := time.Now()
	entry := session.Entry{
		ID:        m.sessionID,
		Workspace: m.cfg.Workspace,
		Title:     m.cfg.Workspace,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  m.messages,
		Context:   m.context,
		Cost:      m.cost,
		Diffs:     m.diffs,
		Todos:     m.todos,
		Tools:     m.tools,
	}
	if m.asyncStore != nil {
		if err := m.asyncStore.Save(entry); err != nil {
			m.persistDirty = true
			return
		}
		m.persistDirty = false
		m.lastPersistAt = now
		return
	}
	if err := m.store.Save(entry); err != nil {
		// 保留脏标记，等待下一次节流重试。
		m.persistDirty = true
		return
	}
	m.persistDirty = false
	m.lastPersistAt = now
}

// 消息类型定义

type workspaceLoadedMsg struct {
	tabs []session.Entry
}

type connectedMsg struct {
	snapshot *client.Snapshot
	events   <-chan client.Event
	cancel   context.CancelFunc
	epoch    int
}

type eventMsg struct {
	event     client.Event
	channelID string
}

type disconnectedMsg struct {
	epoch int
}

type reconnectMsg struct{}

type editorDoneMsg struct {
	content string
	err     error
}

type skillInstallMsg struct {
	entries []skill.Entry
	err     error
}

type skillSyncMsg struct {
	entries []skill.Entry
	err     error
}

type pluginResultMsg struct {
	output string
	err    error
}

type mcpConnectMsg struct {
	url    string
	client *mcp.Client
	tools  []mcp.Tool
	err    error
}

type mcpCallMsg struct {
	name   string
	result any
	err    error
}

type sentMsg struct {
	text string
}

type switchSessionMsg struct {
	id string
}

type errMsg struct {
	err error
}
