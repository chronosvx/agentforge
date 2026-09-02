package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

// helpShortcutFallback 提供帮助面板快捷键文案的本地回退。
// 正常情况下优先使用 i18n 语言包；这些 key 尚未加入语言包时，避免直接显示 key 名。
var helpShortcutFallback = map[string][2]string{
	"help.shortcuts_title": {"全局快捷键", "Global shortcuts"},
	"help.key_send":        {"Enter 发送消息", "Enter send message"},
	"help.key_newline":     {"Ctrl+J 在输入框换行", "Ctrl+J newline in input"},
	"help.key_edit":        {"Ctrl+E 用编辑器编辑当前输入", "Ctrl+E edit current input in $EDITOR"},
	"help.key_palette":     {"Ctrl+P 打开命令面板（支持模糊搜索）", "Ctrl+P open command palette (fuzzy search)"},
	"help.key_help":        {"? 或 Ctrl+/ 打开/关闭帮助", "? or Ctrl+/ toggle help"},
	"help.key_stop":        {"Ctrl+C 停止生成，再按一次退出", "Ctrl+C stop generation, press again to quit"},
	"help.key_focus":       {"Tab 在输入框与面板之间切换焦点", "Tab toggle focus between input and panels"},
	"help.key_back":        {"Esc 返回上一级 / 关闭面板", "Esc go back / close panel"},
	"help.key_sessions":    {"Ctrl+←/→ 切换会话", "Ctrl+←/→ switch sessions"},
	"help.key_scroll":      {"↑/↓、PgUp/PgDn、鼠标滚轮滚动消息", "↑/↓, PgUp/PgDn, mouse wheel to scroll"},
	"help.key_history":     {"Ctrl+R 搜索输入历史", "Ctrl+R search input history"},
	"help.key_redraw":      {"Ctrl+L 清屏/强制重绘", "Ctrl+L clear/force redraw"},
	"help.key_recall":      {"Alt+Up 取回最后一条 user 消息", "Alt+Up recall the last user message"},
	"help.key_agentview":   {"/agentview 打开会话总览", "/agentview open session overview"},
	"help.key_rewind":      {"空输入双击 Esc 时间回溯", "Double-Esc on empty input to rewind"},
}

// helpText 读取帮助文案；缺失 key 时使用内置回退，避免把 key 名展示给用户。
func helpText(key string) string {
	if text := i18n.T(key); text != key {
		return text
	}
	texts, ok := helpShortcutFallback[key]
	if !ok {
		return key
	}
	if i18n.CurrentLang() == i18n.LangEN {
		return texts[1]
	}
	return texts[0]
}

func (m *Model) openHelp() {
	m.helpOpen = true
	m.helpCursor = 0
	m.status = i18n.T("help.title")
}

func (m *Model) closeHelp() {
	m.helpOpen = false
	m.helpCursor = 0
}

// handleHelpKey 处理帮助面板打开时的按键，返回是否已消费。
func (m *Model) handleHelpKey(key string) bool {
	switch key {
	case "esc", "q", "?":
		m.closeHelp()
		return true
	case "j", "down":
		if m.helpCursor < len(m.helpLines())-1 {
			m.helpCursor++
		}
		return true
	case "k", "up":
		if m.helpCursor > 0 {
			m.helpCursor--
		}
		return true
	case "home":
		m.helpCursor = 0
		return true
	case "end":
		if n := len(m.helpLines()); n > 0 {
			m.helpCursor = n - 1
		}
		return true
	}
	return false
}

// helpLines 生成帮助面板内容行。
func (m *Model) helpLines() []string {
	var lines []string
	lines = append(lines, helpText("help.title"), "", helpText("help.shortcuts_title"))
	for _, key := range []string{
		"help.key_send",
		"help.key_newline",
		"help.key_edit",
		"help.key_palette",
		"help.key_help",
		"help.key_stop",
		"help.key_focus",
		"help.key_back",
		"help.key_sessions",
		"help.key_scroll",
		"help.key_history",
		"help.key_redraw",
		"help.key_recall",
		"help.key_agentview",
		"help.key_rewind",
	} {
		lines = append(lines, helpText(key))
	}
	lines = append(lines, "", helpText("help.basic_title"))
	appendCommandRows(&lines, basicCommandPaletteCommands, 4)
	lines = append(lines, "", i18n.T("help.advanced_title"))
	advanced := make([]string, 0, len(commandPaletteCommands))
	for _, cmd := range commandPaletteCommands {
		if !isBasicCommand(cmd) {
			advanced = append(advanced, cmd)
		}
	}
	appendCommandRows(&lines, advanced, 4)
	lines = append(lines, "", i18n.T("help.palette_toggle_hint"), i18n.T("help.close_hint"))
	return lines
}

func appendCommandRows(dst *[]string, commands []string, perRow int) {
	if perRow <= 0 {
		perRow = 4
	}
	for i := 0; i < len(commands); i += perRow {
		end := i + perRow
		if end > len(commands) {
			end = len(commands)
		}
		*dst = append(*dst, strings.Join(commands[i:end], "  "))
	}
}

// renderHelpPanel 渲染帮助面板。
func (m *Model) renderHelpPanel() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(72).
		MaxHeight(22)

	lines := m.helpLines()
	const visible = 20
	start := 0
	if m.helpCursor >= visible {
		start = m.helpCursor - visible + 1
	}
	end := start + visible
	if end > len(lines) {
		end = len(lines)
	}
	if start > end {
		start = end
	}

	var b strings.Builder
	for i, line := range lines[start:end] {
		marker := "  "
		if i+start == m.helpCursor {
			marker = "> "
		}
		b.WriteString(marker + line + "\n")
	}
	return "\n" + style.Render(b.String())
}
