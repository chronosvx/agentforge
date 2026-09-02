package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

const (
	historyMaxEntries = 200
	historyMaxItemLen = 60
	historyVisible    = 15
)

// appendHistory 追加一条输入历史。文本会先 TrimSpace；空文本或与上一条相同时跳过。
// 最多保留 historyMaxEntries 条，超出时丢弃最旧的记录。
func (m *Model) appendHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if n := len(m.history); n > 0 && m.history[n-1] == text {
		return
	}
	m.history = append(m.history, text)
	if len(m.history) > historyMaxEntries {
		m.history = append([]string(nil), m.history[len(m.history)-historyMaxEntries:]...)
	}
}

// openHistorySearch 打开输入历史搜索并重置搜索状态。
func (m *Model) openHistorySearch() {
	m.historySearchOpen = true
	m.historySearchQuery = ""
	m.historySearchCursor = 0
	m.historySearchResults = m.filterHistory("")
}

// closeHistorySearch 关闭输入历史搜索并清理搜索状态。
func (m *Model) closeHistorySearch() {
	m.historySearchOpen = false
	m.historySearchQuery = ""
	m.historySearchCursor = 0
	m.historySearchResults = nil
}

// handleHistoryKey 处理历史搜索面板打开时的按键，返回是否已消费。
func (m *Model) handleHistoryKey(key string) bool {
	switch key {
	case "esc", "q":
		m.closeHistorySearch()
		return true
	case "j", "down", "ctrl+n":
		if len(m.historySearchResults) > 0 && m.historySearchCursor < len(m.historySearchResults)-1 {
			m.historySearchCursor++
		}
		return true
	case "k", "up":
		if m.historySearchCursor > 0 {
			m.historySearchCursor--
		}
		return true
	case "home":
		m.historySearchCursor = 0
		return true
	case "end":
		if n := len(m.historySearchResults); n > 0 {
			m.historySearchCursor = n - 1
		}
		return true
	case "enter":
		if len(m.historySearchResults) > 0 {
			if m.historySearchCursor < 0 || m.historySearchCursor >= len(m.historySearchResults) {
				m.historySearchCursor = 0
			}
			m.input.SetValue(m.historySearchResults[m.historySearchCursor])
			m.syncInputHeight()
			m.applyLayout()
		}
		m.closeHistorySearch()
		return true
	case "backspace":
		if m.historySearchQuery != "" {
			runes := []rune(m.historySearchQuery)
			m.historySearchQuery = string(runes[:len(runes)-1])
			m.applyHistoryFilter()
		}
		return true
	default:
		if isPrintableHistoryKey(key) {
			m.historySearchQuery += key
			m.applyHistoryFilter()
			return true
		}
		return false
	}
}

// applyHistoryFilter 根据当前 query 重新计算搜索结果，并保证 cursor 在有效范围内。
func (m *Model) applyHistoryFilter() {
	m.historySearchResults = m.filterHistory(m.historySearchQuery)
	if m.historySearchCursor < 0 {
		m.historySearchCursor = 0
	}
	if m.historySearchCursor >= len(m.historySearchResults) {
		m.historySearchCursor = 0
	}
}

// filterHistory 按大小写不敏感的包含匹配过滤输入历史。
func (m *Model) filterHistory(query string) []string {
	if query == "" {
		return append([]string(nil), m.history...)
	}
	q := strings.ToLower(query)
	results := make([]string, 0, len(m.history))
	for _, item := range m.history {
		if strings.Contains(strings.ToLower(item), q) {
			results = append(results, item)
		}
	}
	return results
}

// isPrintableHistoryKey 判断是否是一个可作为查询字符追加的按键。
func isPrintableHistoryKey(key string) bool {
	return utf8.RuneCountInString(key) == 1
}

// renderHistorySearch 渲染输入历史搜索面板。
func (m *Model) renderHistorySearch() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(72).
		MaxHeight(20)

	var b strings.Builder
	b.WriteString(i18n.T("history.title") + "\n")
	b.WriteString("> " + m.historySearchQuery + "\n")

	if len(m.historySearchResults) == 0 {
		if len(m.history) == 0 && m.historySearchQuery == "" {
			b.WriteString(i18n.T("history.empty") + "\n")
		} else {
			b.WriteString(i18n.T("history.no_match") + "\n")
		}
	} else {
		start := 0
		if m.historySearchCursor >= historyVisible {
			start = m.historySearchCursor - historyVisible + 1
		}
		end := start + historyVisible
		if end > len(m.historySearchResults) {
			end = len(m.historySearchResults)
		}
		for i := start; i < end; i++ {
			marker := " "
			if i == m.historySearchCursor {
				marker = ">"
			}
			item := sanitizeTerminalText(m.historySearchResults[i])
			item = strings.ReplaceAll(item, "\n", " ")
			item = strings.ReplaceAll(item, "\t", " ")
			item = truncateText(item, historyMaxItemLen)
			b.WriteString(fmt.Sprintf("%s %s\n", marker, item))
		}
	}

	b.WriteString(i18n.T("history.hint") + "\n")
	return "\n" + style.Render(b.String())
}
