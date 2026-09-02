package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

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

	hint := i18n.T("footer.shortcuts")
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
