package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

func (m *Model) renderMessages() {
	if len(m.messages) == 0 {
		if m.dirty || m.lastRenderedCount != 0 || m.lastRenderedStart != 0 ||
			m.lastRenderedWidth != m.viewport.Width || m.lastRenderedHeight != m.viewport.Height {
			m.viewportContentBuilds++
			m.viewport.SetContent(m.renderWelcome())
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

// renderWelcome 在空聊天时渲染友好欢迎页。
func (m *Model) renderWelcome() string {
	if m.workbenchMode {
		return m.renderWorkbenchWelcome()
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accentColor())).
		Bold(true).
		Render(i18n.T("app.welcome_title")))
	b.WriteString("\n\n")
	b.WriteString(i18n.T("app.welcome_subtitle"))
	b.WriteString("\n\n")
	b.WriteString(i18n.T("app.welcome_examples_title"))
	b.WriteString("\n")
	b.WriteString(i18n.T("app.welcome_examples"))
	b.WriteString("\n\n")
	b.WriteString(i18n.T("app.welcome_shortcuts"))

	width := m.viewport.Width - 2
	if width < 10 {
		width = 10
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(1, 2).
		Width(width)
	return style.Render(b.String())
}

// renderWorkbenchWelcome renders the empty-state copy with task/workbench tone.
func (m *Model) renderWorkbenchWelcome() string {
	lang := i18n.CurrentLang()
	title := "🧰 欢迎使用 agentforge 任务/命令工作台"
	subtitle := "直接输入任务或命令，Enter 执行，Alt+Enter 换行。"
	examplesTitle := "试试输入："
	examples := "修复 internal/ui 的滚轮问题 / 给 README 添加快速开始 / /chat 切换到经典聊天"
	shortcuts := "常用：Ctrl+P 命令面板 · ? 帮助 · /chat 聊天 · /workbench 工作台 · Ctrl+C 停止/退出"
	if lang == i18n.LangEN {
		title = "🧰 Welcome to agentforge Task/Command Workbench"
		subtitle = "Type a task or command, press Enter to run, or Alt+Enter for a newline."
		examplesTitle = "Try:"
		examples = "fix mouse wheel scrolling in internal/ui / add a quick start to README / /chat for classic chat"
		shortcuts = "Common: Ctrl+P commands · ? help · /chat chat · /workbench workbench · Ctrl+C stop/quit"
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.accentColor())).
		Bold(true).
		Render(title))
	b.WriteString("\n\n")
	b.WriteString(subtitle)
	b.WriteString("\n\n")
	b.WriteString(examplesTitle)
	b.WriteString("\n")
	b.WriteString(examples)
	b.WriteString("\n\n")
	b.WriteString(shortcuts)

	width := m.viewport.Width - 2
	if width < 10 {
		width = 10
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(1, 2).
		Width(width)
	return style.Render(b.String())
}
