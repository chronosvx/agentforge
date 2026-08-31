// Package markdown 提供轻量、零外部 Markdown 依赖的 TUI 渲染工具。
//
// 输出为带 ANSI 颜色的纯文本，可直接用于 Bubble Tea / Lip Gloss 渲染；
// 输入中的终端控制字符会在渲染前被清理，避免转义注入。
package markdown

import (
	"fmt"
	"strings"
)

const (
	ansiReset           = "\x1b[0m"
	ansiBold            = "\x1b[1m"
	ansiReverse         = "\x1b[7m"
	ansiReverseOff      = "\x1b[27m"
	ansiFgYellow        = "\x1b[33m"
	ansiFgCyan          = "\x1b[36m"
	ansiFgGreen         = "\x1b[32m"
	ansiFgBrightBlack   = "\x1b[90m"
	ansiFgBrightRed     = "\x1b[91m"
	ansiFgBrightGreen   = "\x1b[92m"
	ansiFgBrightYellow  = "\x1b[93m"
	ansiFgBrightBlue    = "\x1b[94m"
	ansiFgBrightMagenta = "\x1b[95m"
	ansiFgBrightCyan    = "\x1b[96m"
)

// Sanitize 去除字符串中的终端控制字符，仅保留换行和 Tab。
// 可用于清理来自 Agent、远端服务或用户输入的自由文本。
func Sanitize(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
}

// Render 将 Markdown 文本转换为带 ANSI 颜色的 TUI 字符串。
//
// 当前支持：
//   - 标题（# ~ ######）：加粗 + 颜色
//   - 行内代码（`code`）：反色显示
//   - 围栏代码块（``` 或 ~~~）：边框 + 行号
//   - 列表（-、*、+）：彩色标记
//   - 引用（>）：左侧竖线 + 缩进
//   - 分隔线（---、***、___）：简单横线
//
// 渲染前会先调用 Sanitize，确保输入中的控制字符不会泄漏到终端。
func Render(text string) string {
	text = Sanitize(text)
	lines := strings.Split(text, "\n")

	var b strings.Builder
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			b.WriteByte('\n')
			continue
		}

		if ch, n, ok := fenceInfo(line); ok {
			lang := fenceLanguage(line)
			var code []string
			j := i + 1
			for ; j < len(lines); j++ {
				if isClosingFence(lines[j], ch, n) {
					break
				}
				code = append(code, lines[j])
			}
			b.WriteString(renderCodeBlock(code, lang))
			b.WriteByte('\n')
			if j < len(lines) {
				i = j // 跳过结束 fence，for 循环的 i++ 会到下一行
			} else {
				i = len(lines)
			}
			continue
		}

		b.WriteString(renderLine(line))
		b.WriteByte('\n')
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// renderLine 渲染单个非代码块行。
func renderLine(line string) string {
	if level, content, ok := parseHeading(line); ok {
		return renderHeading(level, content)
	}

	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, ">") {
		return renderQuote(line)
	}
	if isListLine(trimmed) {
		return renderList(line)
	}
	if isThematicBreak(trimmed) {
		return renderThematicBreak()
	}

	return renderInline(line)
}

// parseHeading 解析 Markdown 标题，返回级别和正文。
func parseHeading(line string) (int, string, bool) {
	s := strings.TrimLeft(line, " \t")
	if s == "" || s[0] != '#' {
		return 0, "", false
	}

	level := 0
	for level < len(s) && s[level] == '#' {
		level++
	}
	if level > 6 || level == len(s) {
		return 0, "", false
	}
	if s[level] != ' ' && s[level] != '\t' {
		// 形如 "#foo" 不是标准标题。
		return 0, "", false
	}

	return level, strings.TrimSpace(s[level:]), true
}

// renderHeading 将标题渲染为加粗彩色文本。
func renderHeading(level int, content string) string {
	styles := []string{
		ansiBold + ansiFgBrightCyan,
		ansiBold + ansiFgBrightBlue,
		ansiBold + ansiFgBrightMagenta,
		ansiBold + ansiFgBrightYellow,
		ansiBold + ansiFgBrightGreen,
		ansiBold + ansiFgBrightRed,
	}
	style := styles[(level-1)%len(styles)]
	return style + renderInline(content) + ansiReset
}

// renderQuote 将引用行渲染为带左侧竖线的文本。
func renderQuote(line string) string {
	s := strings.TrimLeft(line, " \t")
	content := strings.TrimSpace(strings.TrimPrefix(s, ">"))
	for strings.HasPrefix(content, ">") {
		content = strings.TrimSpace(strings.TrimPrefix(content, ">"))
	}
	return ansiFgBrightBlack + "▎ " + ansiReset + renderInline(content)
}

// isListLine 判断是否为简单的无序列表项。
func isListLine(s string) bool {
	if len(s) < 2 {
		return false
	}
	switch s[0] {
	case '-', '*', '+':
		return s[1] == ' ' || s[1] == '\t'
	}
	return false
}

// renderList 渲染无序列表项，保留原标记但加上颜色。
func renderList(line string) string {
	s := strings.TrimLeft(line, " \t")
	marker := s[0]
	content := strings.TrimSpace(s[1:])

	color := ansiFgYellow
	switch marker {
	case '*':
		color = ansiFgGreen
	case '+':
		color = ansiFgCyan
	}
	return color + string(marker) + " " + ansiReset + renderInline(content)
}

// isThematicBreak 判断是否为 Markdown 分隔线。
func isThematicBreak(s string) bool {
	if s == "" {
		return false
	}
	ch := s[0]
	if ch != '-' && ch != '*' && ch != '_' {
		return false
	}
	count := 0
	for _, r := range s {
		if r == rune(ch) {
			count++
			continue
		}
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return count >= 3
}

// renderThematicBreak 渲染简单横线。
func renderThematicBreak() string {
	return ansiFgBrightBlack + strings.Repeat("─", 40) + ansiReset
}

// renderInline 渲染行内内容，目前处理 `code` 行内代码。
func renderInline(s string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '`')
		if start < 0 {
			b.WriteString(s)
			return b.String()
		}

		b.WriteString(s[:start])
		rest := s[start+1:]
		end := strings.IndexByte(rest, '`')
		if end < 0 {
			// 未闭合的反引号按普通字符输出。
			b.WriteByte('`')
			b.WriteString(rest)
			return b.String()
		}

		code := rest[:end]
		b.WriteString(ansiReverse)
		b.WriteString(code)
		b.WriteString(ansiReverseOff)
		s = rest[end+1:]
	}
}

// fenceInfo 返回围栏代码块标记的字符、长度和是否合法。
// 支持 ``` 与 ~~~ 两种围栏。
func fenceInfo(line string) (byte, int, bool) {
	s := strings.TrimLeft(line, " \t")
	if len(s) < 3 {
		return 0, 0, false
	}

	ch := s[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}

	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	return ch, n, true
}

// fenceLanguage 返回开 fence 行中声明的语言（可能为空）。
func fenceLanguage(line string) string {
	s := strings.TrimLeft(line, " \t")
	_, n, ok := fenceInfo(s)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s[n:])
}

// isClosingFence 判断某行是否为当前代码块的结束 fence。
func isClosingFence(line string, openChar byte, openN int) bool {
	s := strings.TrimLeft(line, " \t")
	ch, n, ok := fenceInfo(s)
	if !ok || ch != openChar || n < openN {
		return false
	}
	return strings.TrimSpace(s[n:]) == ""
}

// renderCodeBlock 将代码块渲染为带边框和行号的文本。
func renderCodeBlock(code []string, lang string) string {
	if len(code) == 0 {
		code = []string{""}
	}

	lineNoWidth := len(fmt.Sprintf("%d", len(code)))
	if lineNoWidth < 2 {
		lineNoWidth = 2
	}

	if lang == "" {
		lang = "code"
	}

	maxWidth := 0
	for _, line := range code {
		if w := runeLen(line); w > maxWidth {
			maxWidth = w
		}
	}

	boxWidth := maxWidth + lineNoWidth + 7
	if boxWidth < 20 {
		boxWidth = 20
	}

	var b strings.Builder
	// 顶边框，附带语言标识。
	b.WriteString(ansiFgBrightBlack)
	b.WriteString("┌─ ")
	b.WriteString(lang)
	b.WriteString(" ")
	b.WriteString(strings.Repeat("─", max(0, boxWidth-runeLen(lang)-5)))
	b.WriteString("┐")
	b.WriteString(ansiReset)
	b.WriteByte('\n')

	for i, line := range code {
		b.WriteString(ansiFgBrightBlack + "│ " + ansiReset)
		b.WriteString(ansiFgBrightBlack + fmt.Sprintf("%*d │ ", lineNoWidth, i+1) + ansiReset)
		b.WriteString(line)
		if pad := maxWidth - runeLen(line); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(ansiFgBrightBlack + " │" + ansiReset)
		b.WriteByte('\n')
	}

	b.WriteString(ansiFgBrightBlack)
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", boxWidth-2))
	b.WriteString("┘")
	b.WriteString(ansiReset)

	return b.String()
}

// runeLen 返回字符串的 rune 数量，用于简单宽度计算。
func runeLen(s string) int {
	return len([]rune(s))
}

// max 返回两个 int 中的较大值。
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
