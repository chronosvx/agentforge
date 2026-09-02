package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/i18n"
)

// openRewind 打开时间回溯面板，并从当前消息中收集所有用户消息作为候选。
func (m *Model) openRewind() {
	var candidates []client.Message
	for _, msg := range m.messages {
		if msg.Role == "user" {
			candidates = append(candidates, msg)
		}
	}
	m.rewindCandidates = candidates
	m.rewindOpen = true
	m.rewindCursor = 0
}

// closeRewind 关闭时间回溯面板并清理候选与光标状态。
func (m *Model) closeRewind() {
	m.rewindOpen = false
	m.rewindCandidates = nil
	m.rewindCursor = 0
}

// handleRewindKey 处理时间回溯面板打开时的按键，返回是否已消费该按键。
func (m *Model) handleRewindKey(key string) bool {
	switch key {
	case "esc", "q":
		m.closeRewind()
		return true
	case "j", "down", "ctrl+n":
		if m.rewindCursor < len(m.rewindCandidates)-1 {
			m.rewindCursor++
		}
		return true
	case "k", "up":
		if m.rewindCursor > 0 {
			m.rewindCursor--
		}
		return true
	case "enter":
		if len(m.rewindCandidates) == 0 || m.rewindCursor < 0 || m.rewindCursor >= len(m.rewindCandidates) {
			return false
		}

		candidate := m.rewindCandidates[m.rewindCursor]
		idx := -1
		userCount := 0
		for i, msg := range m.messages {
			if msg.Role != "user" {
				continue
			}
			if userCount == m.rewindCursor {
				idx = i
				break
			}
			userCount++
		}
		if idx < 0 {
			return false
		}

		m.messages = m.messages[:idx]
		m.input.SetValue(candidate.Content)
		m.syncInputHeight()
		m.applyLayout()
		m.dirty = true
		m.followBottom = true
		m.status = i18n.T("rewind.picked")
		m.closeRewind()
		return true
	}
	return false
}

// renderRewind 渲染时间回溯面板。
func (m *Model) renderRewind() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(60).
		MaxHeight(20)

	var b strings.Builder
	b.WriteString(i18n.T("rewind.title") + "\n")
	if len(m.rewindCandidates) == 0 {
		b.WriteString(i18n.T("rewind.empty") + "\n")
	} else {
		for i, msg := range m.rewindCandidates {
			marker := " "
			if i == m.rewindCursor {
				marker = ">"
			}
			content := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(msg.Content)
			content = truncateText(sanitizeTerminalText(content), 60)
			b.WriteString(fmt.Sprintf("%s %s\n", marker, content))
		}
	}
	b.WriteString(i18n.T("rewind.hint"))
	return "\n" + style.Render(b.String())
}
