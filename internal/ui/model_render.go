package ui

import (
	"strings"
)

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
