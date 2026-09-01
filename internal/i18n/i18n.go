// Package i18n 提供 agentforge 的轻量国际化支持。
package i18n

import (
	"fmt"
	"sync"
)

// Lang 表示界面语言。
type Lang string

const (
	// LangZH 是简体中文。
	LangZH Lang = "zh"
	// LangEN 是英文。
	LangEN Lang = "en"
)

var (
	mu      sync.RWMutex
	current Lang = LangZH
)

// zhMessages 是简体中文语言包。
var zhMessages = map[string]string{
	"app.loading":                "加载中...",
	"input.placeholder":          "输入消息，Enter 发送，Ctrl+J 换行，Ctrl+P 命令，Ctrl+/ 帮助，Ctrl+C 退出",
	"status.hint_help":           "? / Ctrl+/ 帮助",
	"status.connecting":          "connecting...",
	"status.resumed":             "resumed: %s",
	"status.connected":           "connected: %s",
	"status.streaming":           "streaming...",
	"status.disconnected":        "disconnected，2 秒后自动重连...",
	"status.reconnecting":        "reconnecting...",
	"status.edited":              "edited",
	"status.sent":                "sent",
	"status.switching":           "switching to %s",
	"status.error":               "error: %s",
	"status.stop_requested":      "停止请求中... 再次 Ctrl+C 退出",
	"status.new_session":         "new session...",
	"status.search_cleared":      "搜索已清除",
	"status.search_sessions":     "搜索会话: %s",
	"status.find_results":        "找到 %d 条结果",
	"status.model_provider_only": "/model 仅 Provider 直连模式可用（当前 DSH/mock 不支持）",
	"status.current_model":       "当前模型: %s (%s)",
	"status.model_switched":      "已切换模型: %s",
	"status.unknown_command":     "未知命令: %s （输入 /help 查看）",
	"status.channel_closed":      "channel closed",
	"status.diff_accept":         "diff %d accepted",
	"status.diff_reject":         "diff %d rejected",
	"status.apply_confirm":       "⚠️ 逐文件确认：j/k 选择，space 切换，g 应用已选，Esc 取消",
	"status.git_apply_ok":        "git apply ok: %s",
	"status.git_apply_error":     "git apply error: %s",
	"status.git_apply_cancelled": "git apply cancelled",
	"status.rollback":            "rolled back from %s",
	"status.skill_detail":        "skill: %s（Enter/Esc 返回）",
	"status.enabled_skill":       "enabled skill: %s",
	"status.disabled_skill":      "disabled skill: %s",
	"usage.find":                 "用法: /find <关键词>",
	"usage.rename":               "用法: /rename <新标题>",
	"usage.skill_install":        "用法: /skill-install <本地目录|git URL>",
	"usage.skill_sync":           "用法: /skill-sync <manifest-url>",
	"usage.plugin":               "用法: /plugin:<插件名>:<命令> [参数...]",
	"help.commands":              "可用命令: /context /cost /diff /todos /tools /skills /plugin /sessions /search /find /right /split /model /new /rename /export /delete /close /skill-install /skill-sync /help",
	"help.title":                 "帮助",
	"help.basic_title":           "基础命令",
	"help.advanced_title":        "高级命令",
	"help.palette_toggle_hint":   "提示：在命令面板中按 Tab 可切换“基础/全部命令”。",
	"help.close_hint":            "关闭：Esc / ? / q；滚动：j/k 或 ↑/↓。",
	"panel.command_title":        "命令面板  (Esc 关闭, j/k/↑/↓ 选择, Tab 基础/全部, Enter 执行)",
	"panel.no_match":             "(无匹配命令)",
	"panel.sidebar_title":        "Sessions",
	"panel.overview_title":       "Overview (1 ctx, 2 cost, 3 diff, 4 todo, 5 skills)",
	"panel.split_title":          "Split: %s (s 切换到该会话)",
	"panel.no_local_history":     "(no local history)",
	"panel.context_unavailable":  "context 数据暂不可用",
	"panel.context_title":        "Context 用量",
	"panel.cost_unavailable":     "cost 数据暂不可用",
	"panel.cost_title":           "Cost 估算",
	"panel.sessions_empty":       "暂无本地会话",
	"panel.sessions_title":       "本地会话（按数字切换）",
	"panel.search_empty":         "没有匹配的消息",
	"panel.search_title":         "消息搜索结果",
	"panel.plugins_empty":        "暂无插件（可放入 <data-dir>/plugins/）",
	"panel.plugins_title":        "插件",
	"panel.apply_confirm":        "确认 git apply（j/k 选择，space 切换，g 应用，Esc 取消）",
	"panel.diff_empty":           "暂无 Diff 数据",
	"panel.diff_title":           "Diff Review（j/k 移动，a 接受，r 拒绝，p patch，g apply，b 回滚）",
	"panel.todos_empty":          "暂无 Todo 数据",
	"panel.todos_title":          "Todo / 任务计划",
	"panel.tools_empty":          "暂无工具调用",
	"panel.tools_title":          "工具调用",
	"panel.skills_empty":         "暂无 Skill（可在 <data-dir>/skills/*.md 添加本地 skill）",
	"panel.skill_detail_title":   "Skill 详情（Enter/Esc 返回）",
	"panel.skill_no_detail":      "（无详细说明）",
	"panel.skills_title":         "Skills（j/k 选择，space 切换，Enter 详情）",
	"panel.skill_no_desc":        "（无说明）",
}

// enMessages 是英文语言包。
var enMessages = map[string]string{
	"app.loading":                "Loading...",
	"input.placeholder":          "Type a message, Enter to send, Ctrl+J newline, Ctrl+P commands, Ctrl+/ help, Ctrl+C quit",
	"status.hint_help":           "? / Ctrl+/ help",
	"status.connecting":          "connecting...",
	"status.resumed":             "resumed: %s",
	"status.connected":           "connected: %s",
	"status.streaming":           "streaming...",
	"status.disconnected":        "disconnected, auto reconnect in 2s...",
	"status.reconnecting":        "reconnecting...",
	"status.edited":              "edited",
	"status.sent":                "sent",
	"status.switching":           "switching to %s",
	"status.error":               "error: %s",
	"status.stop_requested":      "Stop requested... press Ctrl+C again to quit",
	"status.new_session":         "new session...",
	"status.search_cleared":      "Search cleared",
	"status.search_sessions":     "Searching sessions: %s",
	"status.find_results":        "Found %d result(s)",
	"status.model_provider_only": "/model is only available in direct Provider mode (DSH/mock unsupported)",
	"status.current_model":       "Current model: %s (%s)",
	"status.model_switched":      "Switched model: %s",
	"status.unknown_command":     "Unknown command: %s (type /help to see commands)",
	"status.channel_closed":      "channel closed",
	"status.diff_accept":         "diff %d accepted",
	"status.diff_reject":         "diff %d rejected",
	"status.apply_confirm":       "⚠️ Confirm per file: j/k move, space toggle, g apply selected, Esc cancel",
	"status.git_apply_ok":        "git apply ok: %s",
	"status.git_apply_error":     "git apply error: %s",
	"status.git_apply_cancelled": "git apply cancelled",
	"status.rollback":            "rolled back from %s",
	"status.skill_detail":        "skill: %s (Enter/Esc back)",
	"status.enabled_skill":       "enabled skill: %s",
	"status.disabled_skill":      "disabled skill: %s",
	"usage.find":                 "Usage: /find <keyword>",
	"usage.rename":               "Usage: /rename <new title>",
	"usage.skill_install":        "Usage: /skill-install <local dir|git URL>",
	"usage.skill_sync":           "Usage: /skill-sync <manifest-url>",
	"usage.plugin":               "Usage: /plugin:<plugin>:<command> [args...]",
	"help.commands":              "Commands: /context /cost /diff /todos /tools /skills /plugin /sessions /search /find /right /split /model /new /rename /export /delete /close /skill-install /skill-sync /help",
	"help.title":                 "Help",
	"help.basic_title":           "Basic commands",
	"help.advanced_title":        "Advanced commands",
	"help.palette_toggle_hint":   "Tip: press Tab in the command palette to toggle basic/all commands.",
	"help.close_hint":            "Close: Esc / ? / q; scroll: j/k or ↑/↓.",
	"panel.command_title":        "Command Palette (Esc close, j/k/↑/↓ select, Tab basic/all, Enter run)",
	"panel.no_match":             "(no matching commands)",
	"panel.sidebar_title":        "Sessions",
	"panel.overview_title":       "Overview (1 ctx, 2 cost, 3 diff, 4 todo, 5 skills)",
	"panel.split_title":          "Split: %s (s switch to this session)",
	"panel.no_local_history":     "(no local history)",
	"panel.context_unavailable":  "Context data unavailable",
	"panel.context_title":        "Context Usage",
	"panel.cost_unavailable":     "Cost data unavailable",
	"panel.cost_title":           "Cost Estimate",
	"panel.sessions_empty":       "No local sessions",
	"panel.sessions_title":       "Local sessions (press number to switch)",
	"panel.search_empty":         "No matching messages",
	"panel.search_title":         "Message search results",
	"panel.plugins_empty":        "No plugins (place in <data-dir>/plugins/)",
	"panel.plugins_title":        "Plugins",
	"panel.apply_confirm":        "Confirm git apply (j/k move, space toggle, g apply, Esc cancel)",
	"panel.diff_empty":           "No Diff data",
	"panel.diff_title":           "Diff Review (j/k move, a accept, r reject, p patch, g apply, b rollback)",
	"panel.todos_empty":          "No Todo data",
	"panel.todos_title":          "Todo / Task plan",
	"panel.tools_empty":          "No tool calls",
	"panel.tools_title":          "Tool calls",
	"panel.skills_empty":         "No Skills (add local skills in <data-dir>/skills/*.md)",
	"panel.skill_detail_title":   "Skill details (Enter/Esc back)",
	"panel.skill_no_detail":      "(no details)",
	"panel.skills_title":         "Skills (j/k move, space toggle, Enter details)",
	"panel.skill_no_desc":        "(no description)",
}

// SetLang 设置当前界面语言。
func SetLang(lang Lang) {
	mu.Lock()
	defer mu.Unlock()
	current = lang
}

// CurrentLang 返回当前界面语言。
func CurrentLang() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T 从当前语言包取文本并格式化。缺失 key 会先回退到另一种内置语言，再回退为 key 本身。
func T(key string, args ...any) string {
	mu.RLock()
	lang := current
	mu.RUnlock()

	text, ok := lookup(lang, key)
	if !ok {
		text = key
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// lookup 返回 key 在指定语言中的文本；若该语言缺失则回退到另一种内置语言。
func lookup(lang Lang, key string) (string, bool) {
	if lang == LangEN {
		if s, ok := enMessages[key]; ok {
			return s, true
		}
		if s, ok := zhMessages[key]; ok {
			return s, true
		}
		return "", false
	}
	if s, ok := zhMessages[key]; ok {
		return s, true
	}
	if s, ok := enMessages[key]; ok {
		return s, true
	}
	return "", false
}
