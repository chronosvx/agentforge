package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/session"
)

// openAgentView 打开会话总览面板，并刷新本地会话列表。
func (m *Model) openAgentView() {
	m.agentViewOpen = true
	m.agentViewCursor = 0
	m.agentViewQuery = ""
	if m.store != nil {
		if list, err := m.store.List(); err == nil {
			m.allSessions = append([]session.Entry(nil), list...)
			m.sessions = append([]session.Entry(nil), list...)
		}
	}
}

// closeAgentView 关闭会话总览面板并清理状态。
func (m *Model) closeAgentView() {
	m.agentViewOpen = false
	m.agentViewCursor = 0
	m.agentViewQuery = ""
}

// agentViewSessions 返回会话总览当前使用的列表。
// 优先使用 m.sessions（可能已被 /search 过滤），为空时回退到 m.allSessions。
func (m *Model) agentViewSessions() []session.Entry {
	if len(m.sessions) > 0 {
		return m.sessions
	}
	return m.allSessions
}

// handleAgentViewKey 处理会话总览面板的键盘操作，返回是否已消费以及需要执行的 tea.Cmd。
func (m *Model) handleAgentViewKey(key string) (bool, tea.Cmd) {
	sessions := m.agentViewSessions()
	switch key {
	case "esc", "q":
		m.closeAgentView()
		return true, nil
	case "j", "down", "ctrl+n":
		if m.agentViewCursor < 0 {
			m.agentViewCursor = 0
		}
		if m.agentViewCursor < len(sessions)-1 {
			m.agentViewCursor++
		}
		return true, nil
	case "k", "up":
		if len(sessions) == 0 {
			m.agentViewCursor = 0
			return true, nil
		}
		if m.agentViewCursor > len(sessions)-1 {
			m.agentViewCursor = len(sessions) - 1
		}
		if m.agentViewCursor > 0 {
			m.agentViewCursor--
		}
		return true, nil
	case "enter":
		if m.agentViewCursor >= 0 && m.agentViewCursor < len(sessions) {
			selected := sessions[m.agentViewCursor]
			if selected.ID != "" && selected.ID != m.sessionID {
				return true, switchSessionCmd(selected.ID)
			}
		}
		m.closeAgentView()
		return true, nil
	}
	return false, nil
}

// renderAgentView 渲染会话总览面板。
func (m *Model) renderAgentView() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.accentColor())).
		Padding(0, 1).
		Width(64).
		MaxHeight(24)

	var b strings.Builder
	b.WriteString(i18n.T("agentview.title") + "\n")
	b.WriteString(i18n.T("agentview.current") + ": " + sanitizeTerminalText(m.sessionID) + "\n")
	b.WriteString(i18n.T("agentview.recent") + "\n")

	sessions := m.agentViewSessions()
	if len(sessions) == 0 {
		b.WriteString(i18n.T("agentview.empty") + "\n")
	} else {
		for i, s := range sessions {
			marker := " "
			if i == m.agentViewCursor {
				marker = ">"
			}
			title := s.Title
			if title == "" {
				title = s.ID
			}
			line := fmt.Sprintf("%s %s", marker, sanitizeTerminalText(title))
			if !s.UpdatedAt.IsZero() {
				line += "  " + s.UpdatedAt.Format("01-02 15:04")
			}
			b.WriteString(line + "\n")
		}
	}

	if m.multiRunning || len(m.multiTasks) > 0 || len(m.multiPending) > 0 {
		b.WriteString(i18n.T("agentview.running") + "\n")
		tasks := m.multiTasks
		if !m.multiRunning && len(m.multiPending) > 0 {
			tasks = m.multiPending
		}
		const maxShown = 3
		for i, task := range tasks {
			if i >= maxShown {
				break
			}
			summary := strings.TrimSpace(task.Prompt)
			if summary == "" {
				summary = task.ID
			}
			b.WriteString("  " + sanitizeTerminalText(truncateText(summary, 48)) + "\n")
		}
	}

	b.WriteString(i18n.T("agentview.hint"))
	return "\n" + style.Render(b.String())
}
