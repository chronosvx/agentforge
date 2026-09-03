package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

// footerShortcutsHint returns the default footer shortcut hint. The text is
// kept in the UI layer so the newline shortcut can be updated without touching
// the i18n package.
func (m *Model) footerShortcutsHint() string {
	if m.workbenchMode {
		if i18n.CurrentLang() == i18n.LangEN {
			return "Enter run · Alt+Enter newline · Ctrl+P commands · ? help · Ctrl+C stop/quit"
		}
		return "Enter 执行 · Alt+Enter 换行 · Ctrl+P 命令 · ? 帮助 · Ctrl+C 停止/退出"
	}
	if i18n.CurrentLang() == i18n.LangEN {
		return "Enter send · Alt+Enter/Shift+Enter/Ctrl+J newline · Ctrl+P commands · ? help · Ctrl+C stop/quit"
	}
	return "Enter 发送 · Alt+Enter/Shift+Enter/Ctrl+J 换行 · Ctrl+P 命令 · ? 帮助 · Ctrl+C 停止/退出"
}

// renderStatusBar 渲染底部状态栏：保留原有状态、会话、渠道和更新信息，
// 并根据当前界面状态追加新手友好的快捷键提示。
func (m *Model) renderStatusBar() string {
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

	hint := m.footerShortcutsHint()
	switch {
	case m.helpOpen:
		hint = i18n.T("footer.help_hint")
	case m.commandPaletteOpen:
		hint = i18n.T("footer.palette_hint")
	case m.panelFocused:
		hint = i18n.T("footer.panel_focus_hint")
	case m.panel != panelNone || m.rightPanel || m.splitEnabled:
		hint = i18n.T("footer.panel_open_hint")
	}

	return lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(fmt.Sprintf(" %s%s | session: %s | channel: %s%s | %s | %s ", statusPrefix, statusText, sanitizeTerminalText(m.sessionID), sanitizeTerminalText(m.channelID), updateText, m.renderHUD(), hint))
}
