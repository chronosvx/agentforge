package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

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
	lines = append(lines, i18n.T("help.title"), "", i18n.T("help.basic_title"))
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
		MaxHeight(20)

	lines := m.helpLines()
	const visible = 18
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
