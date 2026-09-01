# agentforge Roadmap

> 最后更新：2026-08-31（战略聚焦调整：从“功能铺量”转向“终端 AI 开发代理主流程”）

## 当前状态

已实现一个可运行、可连接真实 DSH Web API 的 Go + Bubble Tea TUI 客户端，并完成多轮评审与 P0/P1 修复。当前“下一代 P0 基础包”已完成后端能力，部分能力已接入 UI。

### 已实现能力

| 能力 | 说明 |
|---|---|
| 真实 DSH API | `/api` HTTP RPC + WebSocket 事件流，支持 seq 断点续传 |
| 流式聊天 | assistant chunk 流式渲染、本地持久化、恢复会话 |
| 命令面板 | `Ctrl+P` 模糊搜索并执行命令 |
| 多行输入 | textarea，`Ctrl+J` 换行，`Ctrl+E` 外部编辑器 |
| Diff Review | 接受/拒绝、patch、逐文件 `git apply`、备份、回滚 |
| 成本/上下文 | Cost 面板、`--cost-limit` 告警、Context 面板、`internal/cost` 估算与 Budget |
| Todo / 工具 | Todo 面板、工具调用内联展示 + `/tools` 面板 |
| 会话管理 | `/new`、`/rename`、`/delete`、`/search`、Tab、侧栏、双会话并排 |
| Skill 管理 | `/skills` 本地/远端合并，`/skill-install`、`/skill-sync` |
| 远程多设备 | `--api-token`、`--remote` 安全警告、HTTPS/WSS 支持 |
| 会话迁移 | `export` / `import` / `sync` 子命令 |
| 插件基础 | `internal/plugin`：Manifest、外部可执行插件、内置插件、超时/工作目录限制 |
| Agent 基础 | `internal/agent`：Agent 执行闭环后端状态机，支持计划、审批、执行、测试、Diff 评审、回滚与动作权限 |
| 上下文工程 | `internal/contextx`：长会话上下文窗口管理、压缩摘要、Checkpoint/Rewind 基础 |
| MCP Client | `internal/mcp`：JSON-RPC 2.0、HTTP/SSE 与 stdio 传输、工具列表/调用 |
| 异步持久化 | `internal/persist` + `session.AsyncStore`：通用异步写盘队列，`Save` 异步、`Load`/`Delete` 同步 |
| 发布工程 | `--version`、`make release`、`make check-release`、GitHub Actions |
| 验收 | `scripts/smoke-real.sh` 真实 DSH 全流程验收 |

## 战略聚焦

agentforge 的定位收敛为：

> **一个跑在终端里的、安全可控的 AI 开发代理：能理解项目、能执行任务、能审 Diff、能回滚。**

所有迭代围绕一条主流程展开：

```text
自然语言需求
  → 生成计划
  → 自动执行（本地/Docker/SSH）
  → 产生 Diff
  → 用户审阅/批准
  → 跑测试
  → 提交/回滚
  → 沉淀到项目记忆
```

## 暂停（不再优先投入）

以下方向暂时冻结，保留基础代码但不扩展、不宣传、不接入新 UI：

- **企业四件套**：RBAC / SSO / KMS / SIEM
  - 保留 `internal/enterprise` 现有能力，不再增加企业级功能。
- **插件市场治理**：插件市场 / 签名 / 沙箱 / 审核 / 举报
  - 保留本地插件加载基础，暂停平台化治理。
- **团队协作**：`internal/collab`、评论、多人审批回放
  - 个人终端工具暂不做团队协作。
- **MCP Server**：`internal/mcpserver`
  - 只保留 MCP Client，MCP Server 暂停。

## 继续（把已有能力接入真实主流程）

- **项目记忆**：`internal/memory`
  - 从“独立包”接入 Repo Mode 和会话上下文。
  - 让 Agent 能记住项目结构、历史决策、常见坑。
- **离线队列**：`internal/offline`
  - 接入真实发送链路：断网时消息进队列，恢复后自动补发。
- **更新检查**：`internal/update`
  - 接入主程序启动流程，提示新版本，不自动下载。
- **Agent 主流程**：`internal/agent`
  - 把“计划 → 审批 → 执行 → 测试 → Diff 评审 → 回滚”做成端到端可用体验。

## 当前进度（开发快照）

- [x] Agent 主流程端到端 Demo：`agentforge agent`
- [x] Repo Mode：`internal/repomode` + `agent --repo`
- [x] 自然语言执行：`agent run/static/llm`
- [x] Patch-to-PR：`internal/gitflow` + `agent --commit/--push/--pr`
- [x] Checkpoint 时间线：`internal/timeline` + UI `/timeline`、`/fork`
- [x] MCP 权限中心：`internal/mcpperm` + UI `/mcp-auto/confirm/deny/audit`
- [x] 离线队列：Provider 后端 + 主流程周期重发
- [x] 更新检查：启动时静默检查
- [x] 项目记忆：UI `/remember`、`/forget`、`/memory`
- [x] 多 Agent 执行基础：`internal/multiagent` + `agentforge multiagent`
- [x] 多 Agent 可视化 UI 编排（依赖树/子任务树：`/multi` JSON 依赖与 `/agents` 面板）
- [x] 真实 LLM 验收：`scripts/acceptance-llm.sh` 已用 DeepSeek 跑通并成功产生代码 Diff（`agent --planner llm --repo --yes`）

## 生产就绪进度

- [x] P0：`doctor`、`init`、根命令 `--help` 子命令列表、无后端友好提示、正式 Release `v0.3.0`、安装脚本支持 Release 下载与 checksum
- [x] P1：`backup` / `restore`、UI 状态栏新版本提示、CI CLI 冒烟测试、多平台 Release pipeline
- [x] P2 部分：`audit`、`config`、`mcp` CLI、`agent/multiagent --json`、`--dry-run`、`make acceptance`、`acceptance-git/multiagent/mcp` 验收脚本
- [x] P2 真实验收：本地 MCP Server、真实 Git Patch-to-PR（临时仓库+PR+清理）、多 Agent 执行
- [ ] P2 剩余：TUI 人工体验测试
- [ ] P2 延期：Linux libsecret 原生后端已移除；Linux Keychain 使用 `secret-tool` CLI 回退

## 新增重点

### P0：核心闭环（先做到“一个人每天都会用”）

1. **Repo Mode：真正懂项目**
   - 自动读取 Git 仓库结构、README、go.mod、测试目录、最近改动。
   - 支持问题：“这个项目怎么跑？”、“哪里改会导致测试挂？”、“这个报错对应哪段代码？”
   - 接入 `internal/memory` 作为项目记忆底座。

2. **自然语言执行**
   - 输入 `/run 帮我看看哪个进程占用了 8080`
   - 先生成命令预览，用户确认后执行。
   - 关键操作记录审计日志，失败可回滚。

3. **Patch-to-PR 工作流**
   - Agent 自动创建分支 → 改代码 → 跑测试 → 生成 commit → 写 PR 描述。
   - 用户在 TUI 里逐文件审 Diff，按 `a/r` 接受/拒绝。
   - 一键 push / 生成 PR。

4. **Checkpoint 时间线**
   - 将 `/checkpoint` / `/rewind` 升级为“会话时间线”。
   - 可回退到任意步骤，也可从某个 checkpoint 分叉出新方案。

### P1：可靠性与安全

5. **MCP 权限中心**
   - MCP 工具分级授权：自动执行 / 每次确认 / 禁止。
   - 文件读写、网络、命令执行分维度控制。
   - 所有工具调用进入审计日志。

6. **离线优先 + 自动重试**
   - 把 `internal/offline` 接进发送链路。
   - 网络恢复后自动补发，状态栏显示待发送数量。

7. **项目记忆落地**
   - 会话关键结论自动沉淀。
   - 跨会话检索：`/remember`、`/forget`、项目知识问答。

### P2：多 Agent 执行

8. **多 Agent 可视化编排**
   - 将 `internal/orchestrator` 接入 UI / 主流程。
   - 展示：主 Agent 拆分子任务、并行状态、失败原因、结果合并。
   - 放在核心闭环稳定之后再做，避免过早复杂度。

## 近期执行建议

下一轮建议按以下顺序推进：

1. **Agent 主流程端到端 Demo**
   - 输入需求 → 生成计划 → 执行 → Diff → 测试 → 提交/回滚。
   - 先把这条链路打通，再谈其他新功能。
2. **Repo Mode**
   - 让 Agent 能感知当前项目，这是“开发工具”和“聊天工具”的分水岭。
3. **自然语言执行**
   - 复用 `internal/executor`，快速形成“AI 终端助手”体验。
4. **Checkpoint 时间线**
   - 在核心闭环基础上增强安全感和可回退性。

## 验收标准

每个阶段完成后，用真实场景验证：

- 在一个真实 Go 仓库里说：“帮我加一个 `--version` 参数”，Agent 能改代码、跑测试、生成 Diff，用户批准后提交。
- 断网时发消息不丢失，恢复后自动补发。
- 让 Agent 调用一个危险 Shell 命令时，必须经过用户确认并留下审计记录。
- 多 Agent 执行时，用户能看清每个子任务的状态和失败原因。
