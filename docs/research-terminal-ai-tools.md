# 终端 AI 编程工具 CLI/TUI 调研报告

> 日期：2026-09-02
> 目的：为 agentforge 的 TUI/CLI 设计提供竞品参考，吸收优秀交互，规避已知坑。
> 调研对象：Claude Code、OpenAI Codex CLI、Gemini CLI、Aider、OpenCode、Amp/Goose/Cursor CLI/Warp/Wave、Pi、DeepSeek Harness（DSH）、dsh-tui 生态。

---

## 1. 结论速览

| 工具 | 定位 | 技术栈 | 最值得学 | 最大坑 |
|---|---|---|---|---|
| Claude Code | 终端原生 agentic coding tool | Bun + TypeScript + React/Ink + Yoga | 可观察性、安全分层、Diff/Subagent/Hooks | TUI 重、滚动/复制差、权限弹窗多、长会话卡 |
| OpenAI Codex CLI | 轻量终端编码代理 | Rust + ratatui/crossterm | 终端兼容工程、沙箱、审批、无头模式 | 长会话 resume 重放、日志/磁盘膨胀 |
| Gemini CLI | 开源终端 AI agent | TypeScript + React/Ink | 功能全、主题、会话管理、无障碍 | 内存高、渲染 bug、权限确认烦、停运风险 |
| Aider | 终端 AI 结对编程 | Python + prompt_toolkit + Rich | Git 自动存档、repo map、多模型分工 | 默认直接改文件、缺逐变更确认、配置复杂 |
| OpenCode | 开源终端 AI agent | TypeScript + SolidJS 自研 TUI | client/server、Leader Key、Provider 中立 | 安全问题、功能堆叠、社区治理争议 |
| Pi | 极简终端 coding harness | TypeScript + 自研 pi-tui | 保留 scrollback、差分渲染、模块化 | 默认无权限、新手门槛高、生态乱 |
| DeepSeek Harness | DSH agent 运行时/插件平台 | Cordis 插件体系 | session log 真源、能力 seam、profile 叠加 | 无官方 TUI、版本破坏性变更 |
| dsh-tui 生态 | DSH 社区终端前端 | Ink/React、pi-tui、自研 ANSI 等 | 事件流真源、审批/折叠/恢复 | npm 同名混乱、版本错位、来源合规 |

---

## 2. Claude Code

### 实现要点
- React/Ink + Yoga，classic / fullscreen 两套渲染器。
- 全链路流式输出、增量 Markdown 解析、虚拟滚动。
- 工具调用、结果、拒绝、错误全部可视化。
- 权限分档：manual / acceptEdits / plan / bypass / auto。
- `/rewind`、`/compact`、`--resume`、worktree、Hooks、Subagents、MCP。

### 网友评价
- 优点：复杂重构、生态、规划能力最强。
- 缺点：滚动/复制、长会话输入延迟、权限弹窗疲劳、功能太多。

### 借鉴/避坑
- 学：可观察性、渐进披露、会话安全感、权限解释。
- 避：不要用重型 React 场景图做 TUI、不要破坏原生滚动、不要弹窗抢焦点。

---

## 3. OpenAI Codex CLI

### 实现要点
- Rust + ratatui/crossterm，事件驱动 + 帧调度。
- alternate screen / inline 双模式，保留 scrollback。
- 针对 Zellij、Windows Terminal 做滚动/重排。
- 审批队列、Sandbox、`codex exec` 无头模式、app-server 分层。

### 网友评价
- 优点：性能、终端兼容、性价比。
- 缺点：长会话 resume 重放、日志膨胀、Windows bug。

### 借鉴/避坑
- 学：终端兼容当一等公民、可发现命令、分层架构。
- 避：不要重放整个长会话、不要污染 stdout/stderr、不要低估磁盘和日志。

---

## 4. Gemini CLI

### 实现要点
- TypeScript + React/Ink，组件化程度高。
- 虚拟化历史、主题系统、会话 browser、屏幕阅读器。
- Trusted Folders + Approval Mode + Policy Engine。
- MCP、Skills、Hooks、扩展主题。

### 网友评价
- 优点：免费额度、1M 上下文、功能全。
- 缺点：内存高、渲染闪烁、权限反复确认、Google 停运。

### 借鉴/避坑
- 学：组件化、多模式、会话恢复、主题。
- 避：不要低估 Ink 渲染坑、不要用反复弹窗代替安全、不要配置地狱。

---

## 5. Aider

### 实现要点
- Python + prompt_toolkit + Rich，终端聊天 REPL。
- Git 自动提交作为存档点。
- repo map、多模型分工、`/undo`、watch 模式。

### 网友评价
- 优点：轻量、Git 原生、多模型。
- 缺点：默认直接改文件、缺逐变更确认、配置复杂。

### 借鉴/避坑
- 学：Git 存档心智、上下文透明、多模型分工。
- 避：默认先预览再应用、撤销按步骤、避免配置地狱。

---

## 6. OpenCode

### 实现要点
- TypeScript + SolidJS 自研 TUI。
- client/server 架构，TUI/Web/Desktop/IDE 共用后端。
- Leader Key + 命令面板 + 斜杠命令。
- 权限 once / always / reject，Provider 中立。

### 网友评价
- 优点：开源、模型无关、TUI 可玩性高。
- 缺点：安全问题、功能堆叠、社区治理争议。

### 借鉴/避坑
- 学：分层架构、默认安全、过程可视化、命令面板。
- 避：不要先上线后补安全、不要盲目堆功能、不要被单厂商卡脖子。

---

## 7. Amp / Goose / Cursor CLI / Warp / Wave

### 各工具要点
- **Amp**：`@` 文件引用、线程同步、全屏审阅、AGENT.md。
- **Goose**：Rust 核心 + CLI/TUI/Electron，MCP/本地模型支持好，但多 UI 栈维护割裂。
- **Cursor CLI**：Plan/Ask/Agent 渐进授权、sandbox、会话恢复。
- **Warp / Wave**：把 AI 集成进终端，块状 UI 创新但 Electron 重、反肌肉记忆。

### 借鉴/避坑
- 学：Plan/Ask/Agent、@引用、线程恢复、执行可视化、可中断。
- 避：不要多套 UI 技术栈并行、不要依赖不活跃输入组件、不要在流式渲染上偷懒、不要忽略 IME/宽字符。

---

## 8. Pi Coding Agent

### 身份
- 主仓库：earendil-works/pi，作者 Mario Zechner。
- MIT 开源，极简终端 coding harness，默认只给 read/write/edit/bash。
- 智谱只是 Provider 之一，不是作者。

### 实现要点
- TypeScript + 自研 pi-tui。
- 保留终端 scrollback 的聊天流 + 差分渲染。
- 会话 JSONL tree，支持 `/tree`、`/fork`。
- 组件化、模块化：pi-ai / pi-agent-core / pi-tui / pi-coding-agent。
- 提供 TUI、print、JSON Lines、RPC 多种模式。

### 借鉴/避坑
- 学：保留 scrollback、差分渲染、默认能力少而清晰、中文 IME 处理、模块化。
- 避：默认无权限、新手门槛高、扩展生态乱、发版过快、缺少 i18n/读屏、错误可见性差。

---

## 9. DeepSeek Harness（DSH）

### 定位
- 官方仓库：deepseek-ai/deepseek-harness。
- “Everything is a Plugin”，基于 Cordis 的 agent harness。
- 官方主要交互面是 `dsh web`；**官方已删除内置 TUI**。

### 关键事实
- 官方 CLI 入口：`dsh web`、`headless`、`sdk`、`acp`。
- 外部前端推荐接入 **DSH Web API**：
  - HTTP `POST /api/*` 做 unary RPC。
  - WebSocket `/api/events.mux` / `/api/events.host` 做下行事件。
  - 审批/提问通过 `POST /api/respond` 回填。
- **不要押注 `tui.dsh/v1alpha1 Channel`**：当前官方仓库/文档未检索到该协议。

### 对 agentforge 的建议
- Session log 是唯一事实源，UI 只是投影。
- 重连按 generation 先 baseline 再增量。
- 不要自己再造沙箱/审批/agent loop。
- 默认安全、错误可行动、安装/升级做版本检测。

---

## 10. dsh-tui 生态

### 代表项目
| 项目 | 说明 |
|---|---|
| ccch1mneyyy/dsh-TUI | 最热门 Claude Code 风格 DSH TUI，React/Ink/Yoga |
| dsh-tui/dsh-tui | 官方历史 TUI 恢复版，基于 pi-tui |
| huiliyi37/dsh-tianshu-tui | 自研 ANSI 纯展示层 TUI |
| jianxx/dsh-cc | 把 Claude Code 工作流搬到 DSH 的插件栈 |
| agentforce314/dsh-ccTUI | Claude Code 风格 TUI，进程内接 DSH services |

### 共同优点
- session/event 是唯一事实源。
- 审批/提问显式化。
- thinking/工具默认折叠。
- 会话按目录恢复、预览。
- 权限状态高可见。
- 三态投递：Enter=steer、Tab=follow-up、Ctrl+Enter=interrupt。

### 共同坑
- 不要重造 Harness。
- 不要照搬 React/Ink 架构到 Bubble Tea。
- 不要低估终端协议与 Windows 权限默认值。
- 注意 npm 同名误导。
- 注意来源/许可合规。

---

## 11. 对 agentforge 的调整路线

### P0：安全与信任
1. 默认安全：workspace-write/ask，full access 显式二次确认。
2. 审批/提问做成显式面板，支持“本次允许 / 总是允许 / 拒绝并记住”。
3. 弹窗不抢输入焦点，危险命令显示完整命令 + 人类可读风险。
4. Diff 批量审阅：文件列表 + 每个文件 diff + 整体接受/拒绝 + 一键回滚。

### P1：会话与输入体验
1. 会话自动命名、列表、搜索、一键 resume。
2. 会话按工作目录分组，展示 Title / Workspace / UpdatedAt。
3. `/compact` 上下文压缩。
4. 输入框支持 `@` 文件/符号引用、斜杠命令补全、外部编辑器。
5. 提供 inline 模式，保留终端 scrollback 和复制。

### P2：可扩展与工程
1. 主题系统、可自定义键位。
2. 插件/MCP 统一接入。
3. 非交互模式增强：`--format json` / `stream-json`。
4. 首次运行向导：自动检测环境、生成项目上下文、示例任务。
5. 终端兼容矩阵：Windows Terminal / tmux / SSH / 中文 IME / 宽字符。

### 架构红线
- 以 DSH Web API 为主，`tui.dsh/v1alpha1` 仅作为实验性预留。
- Session log 是唯一事实源，本地 UI 状态只是投影。
- UI 与 Agent 核心解耦，保持 Mock / DSH Web API / Provider 多后端。

---

## 12. 参考链接

- Claude Code：https://code.claude.com/docs/en/overview
- Codex CLI：https://github.com/openai/codex
- Gemini CLI：https://github.com/google-gemini/gemini-cli
- Aider：https://github.com/Aider-AI/aider
- OpenCode：https://github.com/sst/opencode
- Pi：https://github.com/earendil-works/pi
- DeepSeek Harness：https://github.com/deepseek-ai/deepseek-harness
- dsh-TUI：https://github.com/ccch1mneyyy/dsh-TUI
- dsh-tui 官方历史恢复：https://github.com/dsh-tui/dsh-tui
- dsh-tianshu-tui：https://github.com/huiliyi37/dsh-tianshu-tui
- jianxx/dsh-cc：https://github.com/jianxx/dsh-cc
- HN 讨论：https://news.ycombinator.com/item?id=46286057
- NoCode.Tech 对比分析：https://www.nocode.tech/article/claude-code-vs-codex-vs-mistral-vibe-10000-reddit-comments-reveal-the-real-winners-hint-its-the-combo
