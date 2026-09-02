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
