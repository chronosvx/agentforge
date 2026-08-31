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
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/mcpperm"
	"github.com/chronosvx/agentforge/internal/memory"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"github.com/chronosvx/agentforge/internal/plugin"
	"github.com/chronosvx/agentforge/internal/search"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/skill"
	"github.com/chronosvx/agentforge/internal/timeline"
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
	panelMulti
)

// commandPaletteCommands 是命令面板中可搜索并执行的本地命令。
var commandPaletteCommands = []string{
	"/context", "/cost", "/diff", "/todos", "/tools", "/skills", "/sessions",
	"/plugin", "/search", "/find", "/right", "/split", "/model", "/new", "/rename", "/export",
	"/delete", "/close", "/skill-install", "/skill-sync", "/help",
	"/agent", "/agent-new", "/agent-reset", "/checkpoint", "/rewind", "/mcp", "/mcp-call",
	"/mcp-approve", "/mcp-cancel", "/mcp-auto", "/mcp-confirm", "/mcp-deny", "/mcp-audit",
	"/timeline", "/fork", "/remember", "/forget", "/memory",
	"/exec", "/multi", "/multi-run", "/agents",
}

// Model 是 Bubble Tea 的根模型。
type Model struct {
	cfg        *config.Config
	cl         ChatBackend
	store      session.LocalStore
	asyncStore *session.AsyncStore
	auditFn    func(event string, fields map[string]any)

	sessionID   string
	channelID   string
	events      <-chan client.Event
	eventCancel context.CancelFunc

	messages        []client.Message
	status          string
	err             error
	updateAvailable string

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

	agentSession   *agent.AgentSession
	agentCursor    int
	mcpClient      *mcp.Client
	mcpTools       []mcp.Tool
	mcpCursor      int
	mcpPolicy      *mcpperm.Policy
	mcpAudit       *mcpperm.AuditLog
	pendingMCPName string
	pendingMCPArgs map[string]any
	window         *contextx.Window
	checkpoints    *contextx.CheckpointStore
	timeline       *timeline.Timeline
	memoryStore    *memory.MemoryStore

	multiTasks   []multiagent.Task
	multiPending []multiagent.Task
	multiResults []multiagent.Result
	multiStates  map[string]string
	multiOutputs map[string]string
	multiEventCh chan multiagent.Event
	multiDoneCh  chan multiAgentDoneMsg
	multiRunning bool
	multiCursor  int

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
func New(cfg *config.Config, cl ChatBackend, store session.LocalStore, auditFn ...func(event string, fields map[string]any)) Model {
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
		mcpPolicy:     mcpperm.NewPolicy(mcpperm.ModeConfirm),
		mcpAudit:      mcpperm.NewAuditLog(),
		timeline:      timeline.New(),
		followBottom:  true,
	}
	if cfg != nil && cfg.DataDir != "" {
		if ms, err := memory.NewStore(cfg.DataDir); err == nil {
			m.memoryStore = ms
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		if ms, err := memory.NewStore(filepath.Join(home, ".agentforge")); err == nil {
			m.memoryStore = ms
		}
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
// SetUpdateAvailable 设置状态栏中的新版本提示。
func (m *Model) SetUpdateAvailable(version string) {
	m.updateAvailable = strings.TrimSpace(version)
}

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
		if msg.requiresApproval {
			m.status = "mcp call requires approval (auto-approved in demo)"
			break
		}
		data, err := json.Marshal(msg.result)
		if err != nil {
			data = []byte(fmt.Sprint(msg.result))
		}
		m.status = "mcp result: " + truncateText(sanitizeTerminalText(string(data)), 160)

	case multiAgentEventMsg:
		if m.multiStates == nil {
			m.multiStates = map[string]string{}
		}
		switch msg.Event.Type {
		case multiagent.EventTaskStarted:
			m.multiStates[msg.Event.TaskID] = "running"
			m.status = "multi-agent running: " + msg.Event.TaskID
		case multiagent.EventTaskDone:
			m.multiStates[msg.Event.TaskID] = "done"
			if msg.Event.Output != "" {
				m.multiOutputs[msg.Event.TaskID] = msg.Event.Output
			}
			m.status = "multi-agent done: " + msg.Event.TaskID
		case multiagent.EventTaskFailed:
			m.multiStates[msg.Event.TaskID] = "failed"
			if msg.Event.Output != "" {
				m.multiOutputs[msg.Event.TaskID] = msg.Event.Output
			}
			m.status = "multi-agent failed: " + msg.Event.TaskID
		}
		cmds = append(cmds, m.waitMultiAgentCmd())

	case multiAgentDoneMsg:
		m.multiRunning = false
		m.multiResults = msg.Results
		if msg.Err != nil {
			m.status = "multi-agent finished with errors: " + msg.Err.Error()
		} else {
			m.status = "multi-agent finished"
		}
		if m.panel != panelMulti {
			m.panel = panelMulti
		}

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
	updateText := ""
	if m.updateAvailable != "" {
		updateText = " | ⬆ " + sanitizeTerminalText(m.updateAvailable)
	}
	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(fmt.Sprintf(" %s%s | session: %s | channel: %s%s | /context /cost /diff /todos /tools /skills /sessions /right /split /help ", statusPrefix, statusText, sanitizeTerminalText(m.sessionID), sanitizeTerminalText(m.channelID), updateText))

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

func limitTabs(entries []session.Entry, max int) []session.Entry {
	if len(entries) <= max {
		return entries
	}
	return entries[:max]
}

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

func (m Model) sendCmd(text string) tea.Cmd {
	return m.sendCmdTo(text, m.channelID)
}

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

func switchSessionCmd(id string) tea.Cmd {
	return func() tea.Msg {
		return switchSessionMsg{id: id}
	}
}

func digitIndex(s string) (int, bool) {
	if len(s) != 1 || s[0] < '0' || s[0] > '9' {
		return 0, false
	}
	return int(s[0] - '0'), true
}

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

func (m *Model) checkCostAlert() {
	if m.costLimit <= 0 || m.costAlerted || m.cost == nil {
		return
	}
	if v, ok := m.cost["estimatedYuan"].(float64); ok && v >= m.costLimit {
		m.costAlerted = true
		m.status = fmt.Sprintf("⚠️ cost limit exceeded: %.2f >= %.2f", v, m.costLimit)
	}
}

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

func sanitizeSessionID(id string) string {
	repl := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return repl.Replace(id)
}

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
func truncateText(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}

// renderMessages 将当前可视范围内的消息渲染进 viewport。
