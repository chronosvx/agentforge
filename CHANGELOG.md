# Changelog

本项目所有重要变更记录于此。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased] - 调研驱动改进（最终批）

### 新增

- **TUI 输入框支持 `@` 文件引用与自动补全**：输入 `@` 可触发当前工作区文件路径补全，把文件引用插入消息。
- **全局键位自定义**：支持通过 `<data-dir>/keybindings.json` 自定义 help/palette/history 等全局键位。
- **上下文压缩命令**：新增 `/compact`，将当前会话旧消息压缩为摘要，释放长会话上下文空间。
- **插件市场 CLI**：新增 `agentforge market search/install/list`，支持搜索、安装和列出插件市场条目。
- **doctor 终端兼容性诊断**：`agentforge doctor` 新增终端兼容性诊断。
- **无颜色配置**：新增 `--no-color`，可关闭颜色/ANSI 输出，便于不兼容终端与管道场景。

## [Unreleased] - 调研驱动改进（第五批）

### 新增

- **TUI 清空当前会话**：新增 `/clear` 命令，用于清空当前会话的聊天内容。
- **TUI 状态摘要**：新增 `/status` 命令，用于查看当前状态摘要。
- **项目上下文初始化**：新增 `agentforge project-init` 命令，用于生成 `AGENT.md` 项目上下文。

## [Unreleased] - 调研驱动改进（第四批）

### 新增

- **会话自动命名**：新会话会根据第一条用户消息自动生成标题；使用 `/rename` 手动设置的标题不会被后续持久化覆盖。
- **会话列表按工作区分组**：`/sessions` 面板支持按工作区分组展示，便于在多工作区之间快速定位会话。
- **Headless 流式 JSON Lines 输出**：`--headless --format stream-json` 可将流式结果以 JSON Lines 输出到 stdout，便于脚本和 CI 实时消费。

## [Unreleased] - 调研驱动改进（第三批）

### 新增

- **MCP 工具审批面板**：MCP 工具需要确认时会弹出审批面板，支持 `a` 本次允许、`s` 总是允许、`d` 拒绝本次、`n` 拒绝并记住。
- **主题系统扩展**：`--theme` 新增 `dracula` / `tokyo-night` / `nord` / `solarized`，连同已有 `default` / `catppuccin` 可一键切换。
- **首次运行向导**：新增 `agentforge setup`，以向导方式选择 Mock / DSH Web API / Provider 直连后端并生成/更新本地配置。

## [Unreleased] - 调研驱动改进（第二批）

### 新增

- **会话面板键盘导航**：`/sessions` 会话面板支持 `↑/↓` 移动游标、`Enter` 切换选中会话、`n` 新建会话，并新增 `/resume` 作为 `/sessions` 的别名。
- **Diff Review 批量操作**：Diff 面板支持 `A` 全部接受、`R` 全部拒绝。
- **doctor 建议修复**：`agentforge doctor` 在检查未通过时会输出 `Suggested fixes`，帮助用户快速修复环境问题。

## [Unreleased] - 调研驱动改进

### 新增

- **会话列表 UX**：`/sessions` 面板优先显示会话标题与工作区，底部增加切换/过滤提示。
- **doctor 增强**：`agentforge doctor` 新增 `DSH version` 与 `DSH Web API` 连通性检查，支持 `--api-url` / `--api-token`。
- **Headless JSON 输出**：`--headless --format json` 可输出结构化结果，便于脚本和 CI。
- **协议文档修正**：明确 `tui.dsh/v1alpha1 Channel` 为实验性/预留协议，推荐使用 DSH Web API 作为真实接入方式。

## [Unreleased] - dsh-tui 风格交互增强

### 新增

- **输入历史搜索**：`Ctrl+R` 打开输入历史搜索，支持 `↑/↓` 选择、`Enter` 回填、`Esc` 关闭。
- **会话总览**：`/agentview` 打开会话总览，展示当前会话、最近会话与运行中的多 Agent 任务。
- **时间回溯**：空输入时双击 `Esc` 打开时间回溯，可选取用户消息取回编辑并重发。
- **常驻 HUD**：界面常驻显示上下文 tokens、成本 cost 与生成速度 TPS。

## [Unreleased] - TUI 新手友好调整

### 新增

- **空聊天欢迎页**：新会话没有消息时显示欢迎/引导页，提示如何开始第一条消息、使用命令面板或打开帮助。
- **底部上下文快捷键栏**：聊天区底部根据当前焦点动态提示可用快捷键（如 `Enter 发送`、`Tab 切换焦点`、`? 帮助`），降低新手学习成本。
- **帮助面板新增“全局快捷键”区块**：集中展示 `Ctrl+P`、`Ctrl+/`、`Tab`、`Esc`、`↑/↓` 等不依赖当前面板的全局快捷键；面板内支持 `↑/↓` 或 `j/k` 滚动。
- **多 Agent 面板导航**：`/agents` 多 Agent 面板支持 `↑/↓` 或 `j/k` 在任务/子任务之间导航。

## [0.4.0] - 2026-09-01

### 新增

- **生产就绪 CLI**：
  - `agentforge doctor`：环境自检
  - `agentforge init`：生成配置文件
  - `agentforge config`：查看配置（密钥打码）
  - `agentforge audit`：查看审计日志
  - `agentforge mcp --url ...`：连接 MCP Server 并列出/调用工具
  - `agentforge backup` / `restore`：数据备份与恢复
- **Agent 体验**：
  - `agent --json` / `multiagent --json`
  - `agent --dry-run` / `multiagent --dry-run`
  - LLM Planner 自动重试，避免只读计划假通过
- **TUI 交互改进**：
  - 启用鼠标支持（`tea.WithMouseCellMotion`），滚轮可滚动聊天 viewport 与命令面板/列表面板
  - 新增帮助面板：`?`（输入框为空时）或 `Ctrl+/` 打开，`Esc` 关闭；`/help` 改为打开帮助面板
  - 面板默认非模态：打开面板不抢占输入焦点，`Tab` 切换输入框/面板焦点，`Esc` 先返回输入再关闭面板
  - 命令面板默认只展示基础命令，`Tab` 切换基础/全部命令，支持 `↑/↓`、`Home/End`、`Ctrl+N` 导航
  - 状态栏精简为紧凑帮助提示 `? / Ctrl+/ help`；输入框占位提示加入 `Ctrl+P` 与 `Ctrl+/`
- **安装与发布**：
  - 安装脚本支持私有仓库 `gh release download`
  - 多平台 Release 构建与 checksum
  - 新增 macOS GUI 预览发布集成：`make release-gui` 生成 App/DMG 与校验和，GitHub Actions 可通过 `build-gui-macos` 手动构建并上传 GUI 预览产物
  - 新增 MIT License
- **安全加固**：
  - 可选加密会话存储 `--encrypt-sessions`
  - Windows 原生 Credential Manager
  - 密钥脱敏与审计增强

### 修复

- 修复 Windows `go vet` unsafe pointer 警告
- 修复跨平台 CI 测试问题
- 修复 Git acceptance 脚本在空仓库上的 PR 创建

## [0.3.0] - 2026-08-31

### 新增

- **个人项目**：项目定位为个人学习与日常使用。
- **P0 基础包与 UI 接入**：
  - `internal/agent`：Agent 执行闭环后端状态机，支持计划、审批、执行、测试、Diff 评审、回滚与动作权限。
  - `internal/contextx`：长会话上下文窗口管理、压缩摘要与 Checkpoint/Rewind。
  - `internal/mcp`：MCP Client（JSON-RPC 2.0、HTTP/SSE 与 stdio 传输、工具列表/调用）。
  - `internal/persist` 与 `internal/session.AsyncStore`：异步持久化，`Save` 不阻塞 UI。
  - UI 已接入：Agent 审批面板、`/checkpoint`、`/rewind`、`/mcp`、`/mcp-call`、AsyncStore 持久化。
- **P1 基础与生态**：
  - `internal/orchestrator`：多 Agent 协作与任务编排。
  - `internal/memory`：项目记忆与语义检索。
  - `internal/mcpserver`：MCP Server 基础。
  - `internal/collab`：团队协作与会话回放。
  - `internal/executor`：本地 / Docker / SSH 统一命令执行抽象。
  - `internal/plugin`：插件治理，含签名、权限、沙箱与审核（moderation）。
- **P2 企业级与自动化**：
  - `internal/enterprise`：RBAC、SSO 抽象、KMS 与 SIEM；`--user` / `--roles` 身份模拟与 `exec` RBAC 校验。
  - `internal/automation` 与 `cmd/agentforge-automation`：Interval/Cron 定时与 CI/Webhook 调度。
  - `internal/api` 与 `cmd/agentforge-api`：REST API（会话创建/发送/查询/健康检查）、Bearer Token 鉴权与异步 Webhook。
  - `--headless --prompt` 非交互执行与 `exec` 子命令，以及 `/exec` UI 面板。
- **UI 与体验**：虚拟滚动 / 增量渲染，命令面板、插件命令注册、鼠标滚轮、持久化节流等体验优化。

### 修复与改进

- 完善 P0/P1 评审发现的高优先级问题：竞态、路径安全、身份校验、跨平台构建与测试稳定性。
- 持续提升测试覆盖、多平台 CI、fuzz 与整体稳定性。

### 发布工程

- 版本号统一提升至 `0.3.0`。
- README 当前能力列表同步至 v0.3.0。
- 发布版本为 `v0.3.0`。
- 新增本地 tag `v0.3.0`。

## [0.2.0] - 2026-08-30

### 新增

- **Provider 直连**：`internal/provider` 提供 OpenAI 兼容客户端、多 API Key 轮询/错误映射、路由故障转移、成本单价表与模型上下文窗口注册表；支持 HTTP/HTTPS/SOCKS5 代理（含环境变量代理）与本地 JSON 离线消息队列。
- **i18n 国际化**：新增 `internal/i18n`，支持中/英文界面，`--lang zh|en` 切换，默认中文。
- **本地安全**：新增 `internal/security`，提供 AES-GCM 加解密、敏感信息脱敏（Bearer/Key/Token/Secret 等）与审计日志（`<data-dir>/audit.log`，JSON Lines）；本地会话/补丁/备份目录与文件权限同步收紧。
- **遥测与自动更新检查**：新增 `internal/telemetry` 可选本地遥测（JSON Lines、脱敏、不联网，默认关闭）；新增 `internal/update` 自动更新检查（语义化版本比较 + GitHub Latest Release 查询，仅提示不自动下载）。
- **Windows 安装与 fuzz**：新增 `scripts/install.ps1` Windows 安装脚本（源码构建或 GitHub Release 下载，可加入用户 PATH）；Makefile 新增 `fuzz` 目标覆盖 `internal/provider`、`internal/search`、`internal/session`。
- **会话导入 / 导出 / 同步**：`export` / `import` / `sync` / `sync-remote` 子命令已完成，支持本地批量迁移、双向同步，以及可选 AES-GCM 加密的远程 HTTP/对象存储同步。
- **Skill 管理**：新增 `/skills` 面板，展示本地 `<data-dir>/skills/*.md` 与远端 `available_skills`，支持 `j`/`k` 选择、`Space` 启停、`Enter` 查看详情，并自动合并本地与远端 Skill。
- **远程多设备支持**：`--api-token` 会在 HTTP POST 与 WebSocket 握手请求中自动附加 `Authorization: Bearer <token>`；`--remote` 对非 loopback 且非 `https`/`wss` 的地址打印安全警告，支持 HTTPS/WSS 直连远程 DSH。
- **命令面板**：`Ctrl+P` 打开命令面板，支持前缀、包含、子序列模糊搜索并执行命令。
- **多行输入**：输入框升级为 textarea，`Ctrl+J` 换行，`Ctrl+E` 使用 `$EDITOR` 编辑当前输入。
- **会话管理增强**：新增 `/new` 新建会话、`/rename` 重命名当前会话、`/search` 过滤会话列表。
- **事件可靠性**：Channel 事件 seq 补齐、历史 backfill 与按 seq 去重，断点续传更稳定。
- **工具调用展示**：工具调用内联展示；Channel 事件背压改为阻塞投递。
- **面板体验**：面板最大高度限制与窄屏最小高度保护。
- **全流程验收**：真实 DSH Web API 冒烟脚本支持创建/复用会话、发送 prompt、观察 mux、拉取 History。
- **CI 增强**：增加覆盖率收集与产物上传；多平台 test/vet/build 持续集成。

### 修复与改进

- 修复多角色评审发现的高优先级问题：路径穿越、终端注入、竞态、Init 状态、文件权限等。
- 完善 `--version`、`Ctrl+C` 停止生成、请求超时、自动重连、工具面板、导出/删除等 P0/P1 能力。
- 持续提升测试覆盖与整体稳定性。

### 发布工程

- 版本号统一提升至 `0.2.0`。
- 新增根目录 `CHANGELOG.md`。
- 新增 `scripts/check-release.sh` 与 Makefile `check-release` 目标，发布前自动检查测试、静态检查、构建与版本一致性。

## [0.1.0] - 2026-08-30

- 首个里程碑版本：实现最小闭环、真实 DSH API 接入、Context/Cost/Diff/Todo 面板、多会话 Tab/分屏/双会话、主题、安装脚本与 GitHub Actions 发布流程。
