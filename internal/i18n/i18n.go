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
	"app.welcome_title":          "👋 欢迎使用 agentforge",
	"app.welcome_subtitle":       "直接输入消息就能开始，无需配置。",
	"app.welcome_examples_title": "试试输入：",
	"app.welcome_examples":       "帮我看看这个项目的结构 / 给 README 加一个快速开始",
	"app.welcome_shortcuts":      "常用：Ctrl+P 命令面板 · ? 帮助 · Ctrl+C 停止/退出",
	"footer.shortcuts":           "Enter 发送 · Ctrl+P 命令 · ? 帮助 · Ctrl+C 停止/退出",
	"footer.help_hint":           "帮助已打开 · Esc/q 关闭 · j/k 滚动",
	"footer.palette_hint":        "命令面板 · Esc 关闭 · Enter 执行 · Tab 基础/全部",
	"footer.panel_focus_hint":    "面板模式 · j/k/↑/↓ 选择 · Esc 返回输入 · Tab 切换焦点",
	"footer.panel_open_hint":     "Tab 进入面板 · Esc 关闭面板",
	"input.placeholder":          "输入消息，Enter 发送，Ctrl+J 换行，Ctrl+P 命令，Ctrl+R 历史，Ctrl+/ 帮助，Ctrl+C 退出",
	"history.title":              "输入历史搜索",
	"history.empty":              "暂无输入历史",
	"history.no_match":           "没有匹配的历史",
	"history.hint":               "Enter 回填 · ↑/↓ 选择 · Esc 关闭",
	"mention.title":              "文件引用",
	"mention.empty":              "没有匹配的文件",
	"mention.hint":               "Enter 插入 · j/k/↑/↓ 选择 · Esc/q 关闭",
	"keybindings.loaded":         "已加载键位配置",
	"agentview.title":            "会话总览",
	"agentview.current":          "当前会话",
	"agentview.recent":           "最近会话",
	"agentview.running":          "运行中的多 Agent 任务",
	"agentview.empty":            "暂无会话",
	"agentview.hint":             "Enter 切换 · ↑/↓ 选择 · Esc 返回",
	"rewind.armed":               "再按一次 Esc 打开时间回溯",
	"rewind.title":               "时间回溯",
	"rewind.empty":               "没有可回溯的用户消息",
	"rewind.hint":                "↑/↓ 选择 · Enter 取回编辑 · Esc 取消",
	"rewind.picked":              "已取回消息，可修改后重发",
	"hud.tokens":                 "tokens: %s",
	"hud.cost":                   "cost: %s",
	"hud.tps":                    "tps: %.1f",
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
	"status.cleared":             "已清空当前会话",
	"status.summary":             "会话: %s · 工作区: %s · 消息: %d · tokens: %s · cost: %s",
	"status.compacted":           "已压缩 %d 条旧消息",
	"status.compact_empty":       "没有可压缩的消息",
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
	"help.shortcuts_title":       "全局快捷键",
	"help.key_send":              "Enter 发送消息",
	"help.key_newline":           "Ctrl+J 在输入框换行",
	"help.key_edit":              "Ctrl+E 用编辑器编辑当前输入",
	"help.key_palette":           "Ctrl+P 打开命令面板（支持模糊搜索）",
	"help.key_help":              "? 或 Ctrl+/ 打开/关闭帮助",
	"help.key_stop":              "Ctrl+C 停止生成，再按一次退出",
	"help.key_focus":             "Tab 在输入框与面板之间切换焦点",
	"help.key_back":              "Esc 返回上一级 / 关闭面板",
	"help.key_sessions":          "Ctrl+←/→ 切换会话",
	"help.key_scroll":            "↑/↓、PgUp/PgDn、鼠标滚轮滚动消息",
	"help.key_history":           "Ctrl+R 搜索输入历史",
	"help.key_agentview":         "/agentview 打开会话总览",
	"help.key_rewind":            "空输入双击 Esc 时间回溯",
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
	"panel.sessions_empty":       "暂无会话\n输入 /new 创建一个新会话",
	"panel.sessions_title":       "本地会话（按数字切换）",
	"panel.sessions_workspace":   "工作区: %s",
	"panel.sessions_hint":        "Enter 切换 · n 新建 · 数字快速切换 · /search 过滤",
	"panel.search_empty":         "没有匹配的消息\n试试其他关键词，或输入 /search 清空过滤",
	"panel.search_title":         "消息搜索结果",
	"panel.plugins_empty":        "暂无插件\n可将插件放到 <data-dir>/plugins/ 后输入 /plugin",
	"panel.plugins_title":        "插件",
	"panel.apply_confirm":        "确认 git apply（j/k 选择，space 切换，g 应用，Esc 取消）",
	"panel.diff_empty":           "暂无 Diff 数据\n让 Agent 完成一次修改后，这里会出现可审阅的变更",
	"panel.diff_title":           "Diff Review（j/k 移动，a 接受，r 拒绝，p patch，g apply，b 回滚）",
	"panel.todos_empty":          "暂无 Todo 数据\n让 Agent 生成任务计划后，这里会显示待办",
	"panel.todos_title":          "Todo / 任务计划",
	"panel.tools_empty":          "暂无工具调用\nAgent 调用工具后会在这里显示",
	"panel.tools_title":          "工具调用",
	"panel.skills_empty":         "暂无 Skill\n可在 <data-dir>/skills/*.md 添加本地 skill，或输入 /skill-install 安装",
	"panel.skill_detail_title":   "Skill 详情（Enter/Esc 返回）",
	"panel.skill_no_detail":      "（无详细说明）",
	"panel.skills_title":         "Skills（j/k 选择，space 切换，Enter 详情）",
	"panel.skill_no_desc":        "（无说明）",
	"mcp.approval_title":         "MCP 工具审批",
	"mcp.approval_hint":          "a 本次允许 · s 总是允许 · d 拒绝本次 · n 拒绝并记住 · Esc 取消",
	"mcp.approval_tool":          "工具: %s",
	"mcp.approval_args":          "参数: %s",
	"mcp.approval_pending":       "等待审批",
	"mcp.approved_once":          "已批准本次调用",
	"mcp.always_allowed":         "已允许并记住",
	"mcp.denied_once":            "已拒绝本次调用",
	"mcp.denied_and_remembered":  "已拒绝并记住",
}

// enMessages 是英文语言包。
var enMessages = map[string]string{
	"app.loading":                "Loading...",
	"app.welcome_title":          "👋 Welcome to agentforge",
	"app.welcome_subtitle":       "Start by typing a message — no configuration needed.",
	"app.welcome_examples_title": "Try:",
	"app.welcome_examples":       "explain this project / add a quick start to README",
	"app.welcome_shortcuts":      "Common: Ctrl+P commands · ? help · Ctrl+C stop/quit",
	"footer.shortcuts":           "Enter send · Ctrl+P commands · ? help · Ctrl+C stop/quit",
	"footer.help_hint":           "Help open · Esc/q close · j/k scroll",
	"footer.palette_hint":        "Command palette · Esc close · Enter run · Tab basic/all",
	"footer.panel_focus_hint":    "Panel mode · j/k/↑/↓ select · Esc back to input · Tab toggle focus",
	"footer.panel_open_hint":     "Tab focus panel · Esc close panel",
	"input.placeholder":          "Type a message, Enter to send, Ctrl+J newline, Ctrl+P commands, Ctrl+R history, Ctrl+/ help, Ctrl+C quit",
	"history.title":              "Input history search",
	"history.empty":              "No input history",
	"history.no_match":           "No matching history",
	"history.hint":               "Enter fill · ↑/↓ select · Esc close",
	"mention.title":              "File reference",
	"mention.empty":              "No matching files",
	"mention.hint":               "Enter insert · j/k/↑/↓ select · Esc/q close",
	"keybindings.loaded":         "Keybindings loaded",
	"agentview.title":            "Agent View",
	"agentview.current":          "Current session",
	"agentview.recent":           "Recent sessions",
	"agentview.running":          "Running multi-agent tasks",
	"agentview.empty":            "No sessions",
	"agentview.hint":             "Enter switch · ↑/↓ select · Esc back",
	"rewind.armed":               "Press Esc again to rewind",
	"rewind.title":               "Rewind",
	"rewind.empty":               "No user messages to rewind",
	"rewind.hint":                "↑/↓ select · Enter edit · Esc cancel",
	"rewind.picked":              "Message restored, edit and resend",
	"hud.tokens":                 "tokens: %s",
	"hud.cost":                   "cost: %s",
	"hud.tps":                    "tps: %.1f",
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
	"status.cleared":             "Cleared current session",
	"status.summary":             "Session: %s · Workspace: %s · Messages: %d · Tokens: %s · Cost: %s",
	"status.compacted":           "Compacted %d old messages",
	"status.compact_empty":       "No messages to compact",
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
	"help.shortcuts_title":       "Global shortcuts",
	"help.key_send":              "Enter send message",
	"help.key_newline":           "Ctrl+J newline in input",
	"help.key_edit":              "Ctrl+E edit current input in $EDITOR",
	"help.key_palette":           "Ctrl+P open command palette (fuzzy search)",
	"help.key_help":              "? or Ctrl+/ toggle help",
	"help.key_stop":              "Ctrl+C stop generation, press again to quit",
	"help.key_focus":             "Tab toggle focus between input and panels",
	"help.key_back":              "Esc go back / close panel",
	"help.key_sessions":          "Ctrl+←/→ switch sessions",
	"help.key_scroll":            "↑/↓, PgUp/PgDn, mouse wheel to scroll",
	"help.key_history":           "Ctrl+R search input history",
	"help.key_agentview":         "/agentview open session overview",
	"help.key_rewind":            "Double-Esc on empty input to rewind",
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
	"panel.sessions_empty":       "No sessions\nType /new to create one",
	"panel.sessions_title":       "Local sessions (press number to switch)",
	"panel.sessions_workspace":   "Workspace: %s",
	"panel.sessions_hint":        "Enter switch · n new · number quick switch · /search filter",
	"panel.search_empty":         "No matching messages\nTry another keyword, or type /search to clear filters",
	"panel.search_title":         "Message search results",
	"panel.plugins_empty":        "No plugins\nPlace plugins in <data-dir>/plugins/ and type /plugin",
	"panel.plugins_title":        "Plugins",
	"panel.apply_confirm":        "Confirm git apply (j/k move, space toggle, g apply, Esc cancel)",
	"panel.diff_empty":           "No Diff data\nAfter the agent makes changes, review them here",
	"panel.diff_title":           "Diff Review (j/k move, a accept, r reject, p patch, g apply, b rollback)",
	"panel.todos_empty":          "No Todo data\nAfter the agent creates a task plan, it will appear here",
	"panel.todos_title":          "Todo / Task plan",
	"panel.tools_empty":          "No tool calls\nAgent tool calls will appear here",
	"panel.tools_title":          "Tool calls",
	"panel.skills_empty":         "No Skills\nAdd local skills in <data-dir>/skills/*.md, or use /skill-install",
	"panel.skill_detail_title":   "Skill details (Enter/Esc back)",
	"panel.skill_no_detail":      "(no details)",
	"panel.skills_title":         "Skills (j/k move, space toggle, Enter details)",
	"panel.skill_no_desc":        "(no description)",
	"mcp.approval_title":         "MCP tool approval",
	"mcp.approval_hint":          "a allow once · s always allow · d deny once · n deny and remember · Esc cancel",
	"mcp.approval_tool":          "Tool: %s",
	"mcp.approval_args":          "Args: %s",
	"mcp.approval_pending":       "Waiting for approval",
	"mcp.approved_once":          "Approved this call",
	"mcp.always_allowed":         "Allowed and remembered",
	"mcp.denied_once":            "Denied this call",
	"mcp.denied_and_remembered":  "Denied and remembered",
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
