# agentforge Roadmap

> 最后更新：2026-08-31（P0 基础包已并入，尚未打 tag 发布）

## 当前状态

已实现一个可运行、可连接真实 DSH Web API 的 Go + Bubble Tea TUI 客户端，并完成多轮评审与 P0/P1 修复。当前“下一代 P0 基础包”已完成后端能力，正在进入 UI 接入阶段。

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

## 下一步迭代（按优先级）

### P0：直接提升日常可用性

1. **P0 基础包接入 UI**
   - UI 接入：将 Agent / 上下文 / MCP / 异步存储能力接入 Bubble Tea 工作台
   - Agent 审批面板：展示计划与待审批动作，支持批准/拒绝/回滚
   - Checkpoint/Rewind 命令：`/checkpoint` 保存上下文快照，`/rewind` 回滚到指定检查点
   - MCP 命令：`/mcp` 管理 MCP 工具/服务器，`/mcp-call <工具名> [参数]` 调用工具
   - AsyncStore 接入：将 UI 当前同步 `session.Store` 替换为 `session.AsyncStore`，避免写盘阻塞事件循环
2. **Markdown / 代码块渲染增强**
   - 代码块高亮、行号、折叠
   - 思考过程（reasoning）折叠展示
   - 工具调用卡片化，可展开/收起
3. **真实会话云同步后端**
   - 提供远程存储适配器（HTTP/对象存储/S3 兼容）
   - 会话加密后同步，多设备一致性
4. **成本预测与预算管理**
   - [x] `internal/cost`：Estimator 从 `tokenUsage` 估算成本，Budget 支持会话/项目预算
   - [x] `internal/provider`：Pricing/PricingTable 动态成本单价表，支持按量付费与 Token Plan
   - [ ] 接入 UI / `internal/dshapi`：把 `tokenUsage` 投影转换为 `estimatedYuan`
   - [ ] 趋势图/状态栏持续提示

### P1：重要体验与生态

5. **会话管理完善**
   - 远端会话删除/重命名/归档
   - 会话列表分页、批量操作
   - 双会话并排改为实时双路订阅（当前是本地快照）
6. **历史消息全文搜索**
   - 本地历史索引 + 模糊搜索
   - 跳转到指定消息/seq
7. **鼠标与可访问性**
   - 鼠标点击 Tab/面板/按钮、滚轮滚动
   - 高对比主题、色弱适配、CJK 宽度处理
8. **Skill 市场 / 仓库索引**
   - 远端 manifest registry，支持搜索、一键安装
   - Skill 版本管理与更新检查
9. **性能优化**
   - 持久化节流/异步化，避免每个 chunk 全量写盘
   - 面板增量渲染、大消息分页

### P2：长期演进

10. **远程/多设备运维**
    - SSH 隧道管理 UI
    - 移动端/Web 端同步通知
11. **插件生态**
    - 插件市场、签名校验、沙箱执行
    - 插件权限系统（文件/网络/命令）
12. **分享与协作**
    - 会话录制/回放
    - 分享链接/导出 HTML/PDF
13. **安全加固**
    - 本地数据加密（可选）
    - 敏感信息脱敏展示
14. **发布与分发**
    - 推送下一版本 tag（v0.3.0）触发 GitHub Release
    - 补截图/动图、安装文档、更新提醒

## 近期执行建议

下一轮建议优先做：

1. P0 基础包接入 UI（Agent 审批面板、Checkpoint/Rewind、MCP 命令、AsyncStore 接入）
2. Markdown/代码块渲染增强（P0-2）
3. 成本预测（P0-4）
4. 历史消息搜索（P1-6）

这些可以继续多 Agent 并行：Agent 审批面板、MCP 命令与 AsyncStore 接入可分开，Markdown 渲染与成本预测可分开。
