package ui

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chronosvx/agentforge/internal/i18n"
)

const (
	mentionMaxResults = 200
	mentionVisible    = 10
)

// mentionSkipDirs 是文件补全扫描时需要跳过的目录名。
var mentionSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"vendor":       true,
}

// mentionWorkspace 返回文件补全的根目录：优先使用工作区配置，缺省为当前目录。
func (m *Model) mentionWorkspace() string {
	if m.cfg != nil && strings.TrimSpace(m.cfg.Workspace) != "" {
		return m.cfg.Workspace
	}
	return "."
}

// openMention 打开 @ 文件补全浮层并加载候选。
func (m *Model) openMention(query ...string) {
	q := ""
	if len(query) > 0 {
		q = query[0]
	}
	m.mentionOpen = true
	m.mentionQuery = q
	m.mentionCursor = 0
	m.mentionResults = m.mentionCandidates(q)
}

// closeMention 关闭 @ 文件补全浮层并清理状态。
func (m *Model) closeMention() {
	m.mentionOpen = false
	m.mentionQuery = ""
	m.mentionCursor = 0
	m.mentionResults = nil
}

// syncMentionFromInput 根据当前输入框内容同步 @ 补全状态。
// 该函数在普通按键落入输入框后调用：输入 @ 时打开浮层，删除 @ 或输入空白时关闭浮层。
func (m *Model) syncMentionFromInput() {
	value := m.input.Value()
	at := strings.LastIndex(value, "@")
	if at < 0 {
		m.closeMention()
		return
	}

	after := value[at+1:]
	// 文件补全词是 @ 后到空白前的连续文本；一旦出现空白，说明该次补全已结束。
	if strings.IndexAny(after, " \t\r\n") >= 0 {
		m.closeMention()
		return
	}

	query := after
	if !m.mentionOpen {
		m.openMention(query)
		return
	}
	if m.mentionQuery != query {
		m.mentionQuery = query
		m.mentionResults = m.mentionCandidates(query)
		m.mentionCursor = 0
	}
}

// handleMentionKey 处理 @ 文件补全浮层打开时的按键，返回是否已消费。
func (m *Model) handleMentionKey(key string) bool {
	switch key {
	case "esc", "q":
		m.closeMention()
		return true
	case "j", "down", "ctrl+n":
		if n := len(m.mentionResults); n > 0 && m.mentionCursor < n-1 {
			m.mentionCursor++
		}
		return true
	case "k", "up":
		if m.mentionCursor > 0 {
			m.mentionCursor--
		}
		return true
	case "home":
		m.mentionCursor = 0
		return true
	case "end":
		if n := len(m.mentionResults); n > 0 {
			m.mentionCursor = n - 1
		}
		return true
	case "enter":
		m.acceptMention()
		return true
	}
	return false
}

// acceptMention 将当前选中的相对路径插入输入框并关闭浮层。
// 输入中的 @ 会保留，选中的路径替换 @ 后的查询文本。
func (m *Model) acceptMention() {
	if len(m.mentionResults) == 0 {
		m.closeMention()
		return
	}
	if m.mentionCursor < 0 {
		m.mentionCursor = 0
	}
	if m.mentionCursor >= len(m.mentionResults) {
		m.mentionCursor = len(m.mentionResults) - 1
	}

	selected := m.mentionResults[m.mentionCursor]
	value := m.input.Value()
	at := strings.LastIndex(value, "@")
	if at < 0 {
		m.input.InsertString(selected)
	} else {
		queryEnd := len(value)
		if idx := strings.IndexAny(value[at+1:], " \t\r\n"); idx >= 0 {
			queryEnd = at + 1 + idx
		}
		m.input.SetValue(value[:at+1] + selected + value[queryEnd:])
		m.input.CursorEnd()
	}
	m.syncInputHeight()
	m.applyLayout()
	m.closeMention()
}

// mentionCandidates 扫描工作目录并返回匹配 query 的相对路径候选。
// 跳过 .git、node_modules、dist、build、vendor；跳过 .DS_Store 文件；最多 200 条。
func (m *Model) mentionCandidates(query string) []string {
	root := m.mentionWorkspace()
	lowerQuery := strings.ToLower(strings.TrimSpace(query))
	results := make([]string, 0, 64)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if mentionSkipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if name == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if lowerQuery != "" && !strings.Contains(strings.ToLower(rel), lowerQuery) {
			return nil
		}
		results = append(results, rel)
		if len(results) >= mentionMaxResults {
			return filepath.SkipAll
		}
		return nil
	})

	if len(results) > mentionMaxResults {
		results = results[:mentionMaxResults]
	}
	return results
}

// renderMention 渲染 @ 文件补全浮层。
func (m *Model) renderMention() string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("5")).
		Padding(0, 1).
		Width(72).
		MaxHeight(mentionVisible + 4)

	var b strings.Builder
	b.WriteString(i18n.T("mention.title") + "\n")
	if len(m.mentionResults) == 0 {
		b.WriteString(i18n.T("mention.empty") + "\n")
	} else {
		start := 0
		if m.mentionCursor >= mentionVisible {
			start = m.mentionCursor - mentionVisible + 1
		}
		end := start + mentionVisible
		if end > len(m.mentionResults) {
			end = len(m.mentionResults)
		}
		for i := start; i < end; i++ {
			marker := " "
			if i == m.mentionCursor {
				marker = ">"
			}
			item := sanitizeTerminalText(m.mentionResults[i])
			item = strings.ReplaceAll(item, "\n", " ")
			item = strings.ReplaceAll(item, "\t", " ")
			b.WriteString(fmt.Sprintf("%s %s\n", marker, item))
		}
	}
	b.WriteString(i18n.T("mention.hint") + "\n")
	return "\n" + style.Render(b.String())
}
