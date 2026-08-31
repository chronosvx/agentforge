package ui

import (
	"fmt"
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/markdown"
)

// renderedMessage 是单个消息的渲染结果，供虚拟滚动计算可视范围时复用。
type renderedMessage struct {
	text   string
	height int
}

// viewportHeight 返回 viewport 可用于内容的行数。
func (m *Model) viewportHeight() int {
	h := m.viewport.Height - m.viewport.Style.GetVerticalFrameSize()
	if h < 1 {
		h = 1
	}
	return h
}

// renderMessage 渲染单条消息为带样式的文本。
func (m *Model) renderMessage(msg client.Message) string {
	role := msg.Role
	if role == "" {
		role = "unknown"
	}
	color := "6" // cyan
	if role == "user" {
		color = "5" // magenta
	} else if role == "assistant" {
		color = "2" // green
	}
	header := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render("◆ " + role)
	body := lipgloss.NewStyle().Padding(0, 0, 0, 2).Render(markdown.Render(msg.Content))
	return header + "\n" + body + "\n\n"
}

// renderToolsBlock 渲染消息区底部的工具调用摘要。
func (m *Model) renderToolsBlock() string {
	if len(m.tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n— Tools —\n")
	for i, raw := range m.tools {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := item["type"].(string)
		name, _ := item["name"].(string)
		args, _ := item["arguments"].(string)
		result, _ := item["result"].(string)
		if typ == "call" {
			b.WriteString(fmt.Sprintf("⚙ %d. %s %s\n", i+1, sanitizeTerminalText(name), sanitizeTerminalText(args)))
		} else {
			b.WriteString(fmt.Sprintf("📄 %d. %s\n", i+1, sanitizeTerminalText(result)))
		}
	}
	return b.String()
}

// toolsEqual 比较两份工具列表是否内容相同。
func toolsEqual(a, b []any) bool {
	return reflect.DeepEqual(a, b)
}

// messagesEqual 比较两份消息列表是否内容相同。
func messagesEqual(a, b []client.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
	}
	return true
}

// toolsBlockHeight 返回消息区底部工具摘要的渲染行高。
func (m *Model) toolsBlockHeight() int {
	if len(m.tools) == 0 {
		return 0
	}
	return lipgloss.Height(m.renderToolsBlock())
}

// visibleMessageRange 从当前 scrollOffset 开始，按真实渲染行高向后收集
// 能放入 viewport 的消息。返回实际起止下标和已渲染的消息。
func (m *Model) visibleMessageRange() (start, end int, rendered []renderedMessage) {
	if len(m.messages) == 0 {
		return 0, 0, nil
	}
	start = m.scrollOffset
	if start < 0 {
		start = 0
	}
	if start >= len(m.messages) {
		start = len(m.messages) - 1
	}
	if start != m.scrollOffset {
		m.scrollOffset = start
	}

	h := m.viewportHeight()
	used := 0
	end = start
	for i := start; i < len(m.messages); i++ {
		text := m.renderMessage(m.messages[i])
		height := lipgloss.Height(text)
		if i > start && used+height > h {
			break
		}
		rendered = append(rendered, renderedMessage{text: text, height: height})
		used += height
		end = i + 1
		if used >= h {
			break
		}
	}
	// 至少显示一条消息，避免空 viewport。
	if len(rendered) == 0 && start < len(m.messages) {
		text := m.renderMessage(m.messages[start])
		rendered = append(rendered, renderedMessage{text: text, height: lipgloss.Height(text)})
		end = start + 1
	}

	// 贴底时保留工具摘要的空间，确保工具区也能在当前窗口内可见。
	if end == len(m.messages) {
		toolsHeight := m.toolsBlockHeight()
		for len(rendered) > 1 && used+toolsHeight > h {
			used -= rendered[0].height
			rendered = rendered[1:]
			start++
		}
	}
	m.scrollOffset = start
	return start, end, rendered
}

// bottomScrollOffset 返回贴底时第一条可见消息的下标。
func (m *Model) bottomScrollOffset() int {
	if len(m.messages) == 0 {
		return 0
	}
	h := m.viewportHeight()
	toolsHeight := m.toolsBlockHeight()
	used := 0
	start := len(m.messages)
	for i := len(m.messages) - 1; i >= 0; i-- {
		height := lipgloss.Height(m.renderMessage(m.messages[i]))
		if i < len(m.messages)-1 && used+height+toolsHeight > h {
			break
		}
		used += height
		start = i
		if used+toolsHeight >= h {
			break
		}
	}
	if start >= len(m.messages) {
		start = len(m.messages) - 1
	}
	return start
}

// scrollByViewportMsg 将虚拟滚动偏移量按用户滚动操作移动。
func (m *Model) scrollByViewportMsg(msg tea.Msg) {
	if len(m.messages) == 0 {
		return
	}
	delta := 0
	switch msg := msg.(type) {
	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			delta = -1
		case tea.MouseButtonWheelDown:
			delta = 1
		default:
			return
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "pgup", "ctrl+u":
			delta = -m.visibleMessageCountForScroll()
		case "pgdown", "ctrl+d":
			delta = m.visibleMessageCountForScroll()
		default:
			return
		}
	default:
		return
	}

	old := m.scrollOffset
	m.scrollOffset += delta
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	maxStart := len(m.messages) - 1
	if m.scrollOffset > maxStart {
		m.scrollOffset = maxStart
	}
	if m.scrollOffset != old {
		m.dirty = true
		m.followBottom = false
	}
}

// visibleMessageCountForScroll 返回翻页/滚动时使用的消息步长。
func (m *Model) visibleMessageCountForScroll() int {
	h := m.viewportHeight()
	if h <= 3 {
		return 1
	}
	// 每条消息按至少 3 行估算，避免一页滚动过多。
	n := h / 3
	if n < 1 {
		n = 1
	}
	return n
}
