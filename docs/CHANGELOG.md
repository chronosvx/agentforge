# Changelog

本项目所有重要变更记录于此。

格式基于 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

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
- 新增 `docs/CHANGELOG.md`。
- 新增 `scripts/check-release.sh` 与 Makefile `check-release` 目标，发布前自动检查测试、静态检查、构建与版本一致性。

## [0.1.0] - 2026-08-30

- 首个里程碑版本：实现最小闭环、真实 DSH API 接入、Context/Cost/Diff/Todo 面板、多会话 Tab/分屏/双会话、主题、安装脚本与 GitHub Actions 发布流程。
