# agentforge

[中文](README.md) | [English](README.en.md)

一个从零开始、用 Go + Bubble Tea 构建的个人 DSH TUI 客户端。

## 🚀 零基础快速开始

不会编程？也能直接运行。  
👉 从 [docs/QUICKSTART.md](docs/QUICKSTART.md) 开始：下载、安装、启动、连接 AI，一步步照着做即可。

## 截图

> 截图/动图占位：将终端截图或录屏放入 `docs/screenshots/` 后，在此替换为实际图片链接。
> 建议提供：主聊天界面、Diff Review、双会话并排、命令面板。

## 文档

- 零基础快速开始： [docs/QUICKSTART.md](docs/QUICKSTART.md)
- 总览： [docs/overview.md](docs/overview.md)
- 完整使用说明： [docs/usage.md](docs/usage.md)
- 架构说明： [docs/architecture.md](docs/architecture.md)
- 更新日志： [CHANGELOG.md](CHANGELOG.md)

## 设计目标

- 通过 `tui.dsh/v1alpha1` Channel 协议连接 DSH，而不是重新实现 Agent Harness。
- 兼顾性能与长期维护：Go 单二进制、低内存、高渲染性能。
- 已实现：真实 DSH Web API / Channel 接入、流式聊天、多会话工作区、Context/Cost/Diff/Todo 面板、Provider 直连、i18n、安全、企业 RBAC/SIEM、自动化调度、遥测与更新检查，以及 Agent / 上下文工程 / MCP Client / 异步持久化等 P0 基础包。
- 当前聚焦：Agent 主流程、Repo Mode、自然语言执行、Patch-to-PR、Checkpoint 时间线、MCP 权限中心、离线队列与项目记忆。
- 暂停优先投入：企业四件套、插件市场治理、团队协作、MCP Server；保留已有基础代码，不再扩展。

## 项目结构

```text
cmd/agentforge/      入口
cmd/agentforge-automation/  定时/CI 自动化调度入口（加载 JSON Job）
cmd/agentforge-api/  HTTP API / Webhook 服务入口
internal/api/        开放 HTTP API 与 Webhook Notifier
internal/automation/ 轻量任务调度器（Interval/Cron）
internal/enterprise/ 企业基础能力：RBAC、SSO、KMS、SIEM
internal/dshapi/      真实 DSH Web API 客户端与 Backend 适配
internal/client/      DSH Channel WebSocket 客户端
internal/session/     会话状态、持久化与导入/导出/同步（含 AsyncStore 异步存储）
internal/agent/       Agent 执行闭环后端：计划、审批、执行、测试、Diff 评审、回滚状态机
internal/contextx/    长会话上下文窗口管理、压缩摘要、Checkpoint/Rewind
internal/mcp/         MCP Client：JSON-RPC 2.0、HTTP/SSE 与 stdio 传输、工具列表/调用
internal/persist/     通用异步持久化队列（Writer）
internal/share/       会话分享链接（AES-GCM 加密 / 明文）
internal/skill/       Skill 本地安装/仓库克隆/远端 manifest 同步
internal/provider/    直连模型厂商：OpenAI 兼容客户端、代理、路由/故障转移、定价与上下文窗口
internal/secret/      API Key 安全存储：系统 Keychain / 本地 AES-GCM 加密文件
internal/offline/     本地 JSON 离线消息队列
internal/security/    AES-GCM 加密、脱敏、审计日志
internal/plugin/      插件市场/签名/权限/沙箱
internal/cost/        成本估算与预算管理
internal/ui/          Bubble Tea UI
internal/i18n/        轻量国际化语言包（zh / en）
internal/config/      配置与 CLI 参数
internal/telemetry/   可选本地遥测（JSON Lines，脱敏，不联网）
internal/update/      自动更新检查（版本比较 + GitHub Release 查询）
```

## 当前能力

已实现可运行、可连接真实 DSH Web API 的 v0.3.0 功能集（P0/P1/P2 主要能力已并入）：

- **DSH 客户端与真实 API**：`internal/dshapi` 接入 DSH Web API（HTTP RPC + WebSocket 事件流），支持 seq 断点续传、自动重连与真实会话恢复；`internal/client` 保留 Channel WebSocket 客户端。
- **TUI 工作台**：Bubble Tea 聊天界面、流式渲染与虚拟滚动/增量渲染、命令面板、多行输入、多会话 Tab/分屏、Context/Cost/Diff/Todo/Tools/Skills/Plugin 面板、会话搜索与历史全文搜索，以及 `/exec` 执行面板。
- **Provider 直连**：`internal/provider` 提供 OpenAI 兼容客户端、多 API Key 轮询、路由/故障转移、错误映射、成本单价表与模型上下文窗口注册表；支持 HTTP/HTTPS/SOCKS5 代理与本地离线消息队列。
- **P0 基础包与 UI 接入**：`internal/agent` 提供 Agent 执行闭环/审批状态机；`internal/contextx` 提供长会话上下文窗口管理、压缩摘要与 Checkpoint/Rewind；`internal/mcp` 提供 MCP Client（JSON-RPC 2.0、HTTP/SSE/stdio、工具列表/调用）；`internal/persist` 与 `session.AsyncStore` 提供异步持久化，`Save` 不阻塞 UI；以上能力已接入 UI（Agent 审批面板、`/checkpoint`、`/rewind`、`/mcp`、`/mcp-call`）。
- **P1 基础与生态**：`internal/orchestrator` 多 Agent 协作与任务编排；`internal/memory` 项目记忆/语义检索；`internal/mcpserver` MCP Server 基础；`internal/collab` 团队协作/回放；`internal/executor` 统一本地/Docker/SSH 命令执行；`internal/plugin` 插件治理（签名、权限、沙箱、审核）。
- **i18n 国际化**：中/英文界面，`--lang zh|en` 切换，默认中文。
- **本地安全**：AES-GCM 加解密、敏感信息脱敏、审计日志，以及 API Key 系统 Keychain / 本地加密文件存储。
- **企业能力**：`internal/enterprise` 提供 RBAC（admin/member/auditor/guest）、SSO 抽象、KMS 与 SIEM；CLI 支持 `--user` / `--roles` 模拟身份，`exec` 前执行 RBAC 校验，关键事件写入 `<data-dir>/siem.log`。
- **自动化调度**：`internal/automation` 提供轻量 Interval/Cron 调度器，`cmd/agentforge-automation` 从 JSON 加载 Job 并触发 Headless 任务，适合定时、CI 与 Webhook 旁路调度。
- **遥测与更新**：可选本地遥测（JSON Lines、脱敏、不联网，默认关闭）；自动更新检查（版本比较 + GitHub Release 查询，仅提示不自动下载）。
- **会话迁移**：`export` / `import` / `sync` / `sync-remote`，支持本地批量迁移、双向同步与可选 AES-GCM 加密的远程同步；`share` / `open` 可把单个会话打包为可分享链接并导入。
- **非交互与远程执行**：`--headless --prompt` 可不启动 TUI 直接调用 Provider/DSH 后端并打印结果；`exec` 子命令复用 `internal/executor` 支持本地、Docker 容器和 SSH 远程命令执行。
- **开放 API / Webhook**：`cmd/agentforge-api` 提供 REST 接口（创建会话、发送 prompt、查询会话、健康检查），支持 Bearer Token 鉴权与异步 Webhook 事件通知；`internal/api` 不依赖 UI 核心逻辑。
- **发布与安装**：多平台 `make release`、`make package` 生成 DMG/tar.gz/zip 安装包、Windows `scripts/install.ps1`、GitHub Actions CI 与 fuzz。

UI 体验细节：

- 已加载插件的 `Manifest.Commands` 会自动注册进命令面板（`/plugin:<插件名>:<命令>`，无冲突时也可用 `/<命令>`）
- 启用鼠标支持：滚轮可滚动聊天 viewport，命令面板/列表面板中滚轮移动光标
- 本地会话持久化节流为最多每 500ms 写一次，退出前强制落盘
- 状态栏使用 `•` / `⚠` / `✖` 等符号前缀，不只依赖颜色
- 窄屏（宽度 < 80）自动隐藏右侧 Overview 面板和分屏

## 内置命令

在输入框输入以下命令并回车（不会发给 Agent）：

| 命令 | 作用 |
|---|---|
| `/context` | 打开/关闭 Context 用量面板 |
| `/cost` | 打开/关闭成本估算面板 |
| `/diff` | 打开/关闭 Diff Review 面板（真实 API 可解析 `git diff` 输出，`p` 生成 patch） |
| `/todos` | 打开/关闭 Todo / 任务计划面板 |
| `/tools` | 打开/关闭工具调用记录面板 |
| `/skills` | 打开/关闭 Skill 管理面板 |
| `/plugin` | 打开/关闭插件列表面板 |
| `/sessions` | 打开/关闭本地/远端会话列表；面板中按数字键切换会话 |
| `/search <关键词>` | 过滤会话列表 |
| `/find <关键词>` | 全文搜索历史消息 |
| `/right` | 打开/关闭右侧常驻 Overview 面板 |
| `/split` | 打开/关闭双会话并排视图 |
| `/export` | 导出当前会话为 Markdown/JSON |
| `/rename <标题>` | 重命名当前本地会话 |
| `/delete` | 删除当前本地会话 |
| `/skill-install <本地目录\|git URL>` | 从本地目录或 git 仓库安装/更新 Skill |
| `/skill-sync <manifest-url>` | 从 JSON manifest 同步远端 Skill 仓库清单 |
| `/agent` | 打开/关闭 Agent 执行/审批面板；`/agent new` 新建会话 |
| `/agent-new` | 新建 Agent 执行会话 |
| `/checkpoint <名称>` | 保存当前会话 Checkpoint 到时间线 |
| `/rewind <ID\|名称>` | 回滚到指定 Checkpoint 或时间线快照 |
| `/timeline` | 查看 Checkpoint 时间线 |
| `/fork <标签>` | 从当前会话分叉一个新的时间线分支 |
| `/mcp <url>` | 连接 MCP Server 并列出工具 |
| `/mcp-call <工具> <json>` | 调用 MCP 工具 |
| `/mcp-auto <工具>` | 设置 MCP 工具为自动执行 |
| `/mcp-confirm <工具>` | 设置 MCP 工具为每次确认 |
| `/mcp-deny <工具>` | 禁止 MCP 工具 |
| `/mcp-audit` | 查看最近的 MCP 调用审计 |
| `/remember <键> <值>` | 保存一条项目记忆 |
| `/forget <键>` | 删除一条项目记忆 |
| `/memory [查询]` | 列出或搜索项目记忆 |
| `/exec [命令]` | 打开执行面板，或直接执行命令 |
| `/help` | 显示可用命令 |
| `/close` | 关闭当前 Channel |

如果已加载插件声明了 `commands`，命令面板和输入框还会额外支持这些插件命令，例如 `/plugin:demo:status` 或（无冲突时）`/status`。

## 会话导入 / 导出 / 分享 / 同步

不启动 TUI，直接用命令行批量迁移/备份/分享本地会话：

```bash
# 导出全部会话 JSON 到指定目录
agentforge export --out-dir /path/to/export

# 从导出目录导入全部会话（已存在的 ID 会跳过，不覆盖）
agentforge import --in-dir /path/to/export

# 分享单个会话为加密链接（推荐；也支持 DSH_SHARE_KEY 环境变量）
agentforge share --session <会话ID> --key <hex-or-base64>

# 导入分享链接到本地
agentforge open --url 'dsh-share://...' --key <hex-or-base64>

# 在两个数据目录间按 UpdatedAt 双向同步，较新的覆盖较旧的
agentforge sync --from /path/to/dir-a --to /path/to/dir-b

# 推送全部本地会话到远程 HTTP/对象存储后端（可选 AES-GCM 加密）
agentforge sync-remote --url https://host/sessions --push

# 从远程拉取会话到本地（本地较新的会话会跳过）
agentforge sync-remote --url https://host/sessions --pull --key <hex-or-base64>

# 持续同步：未指定方向时每次先 pull 再 push，默认每 60s 执行一次
agentforge sync-remote --url https://host/sessions --watch

# 持续同步：每 5 分钟只推送
agentforge sync-remote --url https://host/sessions --push --watch --interval 5m
```

分享链接支持通过 `--key` / `--share-key` 或环境变量 `DSH_SHARE_KEY` 提供 AES-GCM 密钥；未提供密钥时生成明文链接（标记 `plain=true`）。远程同步支持通过 `--key` 或环境变量 `DSH_SYNC_KEY` 提供 AES-GCM 密钥；未提供密钥时按明文 HTTP 同步。`--watch` 模式会按 `--interval` 持续同步，适合多设备自动同步，收到 SIGINT（Ctrl+C）或 SIGTERM 后退出。详细说明见 [docs/usage.md](docs/usage.md)。

## 非交互与远程执行

不启动 TUI 的 Headless 对话，以及本地 / 容器 / 远程命令执行：

```bash
# Headless：直接向 Provider/DSH 后端发送 prompt 并打印最终结果
agentforge --headless --provider deepseek --provider-key sk-xxx --prompt "hello"

# 本地执行
agentforge exec --local --cmd "echo hi"

# Docker 容器执行
agentforge exec --docker --image alpine --cmd "echo hi"

# SSH 远程执行
agentforge exec --ssh --host user@host --cmd "echo hi"

# 指定超时与工作目录
agentforge exec --local --cmd "go test ./..." --workdir /path/to/project --timeout 5m

# 企业身份执行：--user/--roles 放在 exec 之前，触发 RBAC 与 SIEM
agentforge --user alice --roles member exec --local --cmd "echo hi"
```

### Agent 主流程 / 多 Agent

Agent 主流程 Demo：自然语言需求 → 计划 → 执行 → Diff → 测试 → 提交/回滚。

```bash
# 静态规划器：run/edit/test 指令
agentforge agent --prompt "run go test ./..." --yes

# 仓库感知：扫描当前仓库后规划
agentforge agent --prompt "帮我看看这个项目怎么跑" --repo --yes

# 使用 LLM 规划器（需要 API Key）
agentforge agent --prompt "帮我加一个 --version 参数" --planner llm --provider deepseek --provider-key sk-xxx --repo --yes

# 自动提交 / 推送 / 创建 PR
agentforge agent --prompt "edit README.md hello" --yes --commit --push --pr --pr-title "docs: update readme"

# 并行执行多个 Agent 任务
agentforge multiagent --tasks "run echo a,run echo b,run echo c" --yes
```

详细参数说明见 [docs/usage.md](docs/usage.md)。

### 自动化调度

`agentforge-automation` 从 JSON 加载 Job，按 Interval/Cron 触发 Headless 任务：

```bash
# 持续调度
go run ./cmd/agentforge-automation --config jobs.json

# 立即执行一次
go run ./cmd/agentforge-automation --config jobs.json --once
```

示例 `jobs.json`：

```json
{
  "jobs": [
    {"id": "daily", "command": "agentforge --headless --mock --prompt \"生成日报\"", "cron": "0 9 * * *"},
    {"id": "smoke", "command": "agentforge --headless --mock --prompt \"冒烟测试\"", "interval": "5s"}
  ]
}
```

调度器会注入 `DSH_AUTOMATION_JOB_ID`、`DSH_AUTOMATION_JOB_NAME` 和 `DSH_DATA_DIR` 环境变量。完整说明见 [docs/usage.md](docs/usage.md)。

快捷键：

| 按键 | 作用 |
|---|---|
| `Ctrl+C` | 第一次停止生成，第二次退出 |
| `Ctrl+J` | 输入框内换行（多行输入） |
| `Ctrl+E` | 用 `$EDITOR` 编辑当前输入 |
| `Ctrl+P` | 打开命令面板 / 模糊搜索 |
| 鼠标滚轮 | 滚动聊天消息；命令面板/列表面板中移动光标 |
| `Ctrl+←` | 切换到上一个会话 Tab |
| `Ctrl+→` | 切换到下一个会话 Tab |
| 右侧 Overview 面板内 `1`~`4` | 打开 Context / Cost / Diff / Todos 详情面板 |
| 双会话并排内 `[` / `]` | 切换右侧会话 |
| 双会话并排内 `s` | 切换到右侧会话并发送消息 |
| Diff 面板内 `j` / `k` | 上/下移动 Diff 光标 |
| Diff 面板内 `a` / `r` | 接受 / 拒绝当前 Diff |
| Diff 面板内 `p` | 把已接受的 Diff 保存为 patch 文件 |
| Diff 面板内 `g` | 两次 `g` 确认后执行 `git apply` 应用到工作区 |

## 开放 API / Webhook

`agentforge-api` 为 CI/外部程序提供轻量 REST 接口：

```bash
# Provider 直连模式
go run ./cmd/agentforge-api --listen 127.0.0.1:8080 \
  --provider deepseek --provider-key sk-xxx --model deepseek-chat \
  --data-dir ~/.agentforge --api-token secret --webhook-url https://ci.example.com/hook

# 使用内置 mock server 验证
go run ./cmd/agentforge-api --mock --listen 127.0.0.1:8080
```

接口：

- `GET /v1/healthz`：健康检查
- `POST /v1/sessions`：创建会话
- `GET /v1/sessions/{id}`：获取会话
- `POST /v1/sessions/{id}/prompt`：发送 prompt 并返回最终结果

启用 `--api-token` 后，`/v1/*` 请求需携带 `Authorization: Bearer <token>`。配置 `--webhook-url` 后，服务会异步发送 `session.created`、`session.prompt.completed`、`session.prompt.failed` 事件，请求头包含 `X-Webhook-Secret`。完整说明见 [docs/usage.md](docs/usage.md)。

## 开发

```bash
# 使用内置 mock server 启动（无需真实 DSH）
go run ./cmd/agentforge --mock

# 连接真实 DSH Web API（推荐，当前已实测）
go run ./cmd/agentforge --api-url http://127.0.0.1:54695

# 连接真实 DSH TUI Channel（协议已实现，服务端支持情况待确认）
go run ./cmd/agentforge --endpoint ws://127.0.0.1:18891/agentforge/v1/connection

# 恢复会话
go run ./cmd/agentforge --mock --resume session-xxx

# 设置成本告警阈值（元）
go run ./cmd/agentforge --mock --cost-limit 5

# 使用 catppuccin 主题
go run ./cmd/agentforge --mock --theme catppuccin

# 使用英文界面（默认中文）
go run ./cmd/agentforge --mock --lang en

# 启用本地遥测（默认关闭；仅写本地 telemetry.log，不联网）
go run ./cmd/agentforge --mock --telemetry
# 或通过环境变量开启
DSH_TELEMETRY=1 go run ./cmd/agentforge --mock

# 远程直连（HTTPS/WSS + Token）
go run ./cmd/agentforge --remote --api-url https://dsh.example.com --api-token <token>

# 通过 SSH 隧道连接远程 DSH（本地仍为 loopback）
# 先执行: ssh -L 18891:127.0.0.1:18891 user@dsh-host
go run ./cmd/agentforge --api-url http://127.0.0.1:18891 --api-token <token>
```

## 远程部署与 Token

DSH Web API 默认监听本机 loopback。远程访问有两种推荐方式：

- **SSH 隧道**：在本地把远程端口映射到 `127.0.0.1`，URL 保持 loopback，无需额外 TLS；仍建议使用 `--api-token` 做服务端认证。
- **HTTPS/WSS 直连**：通过反向代理暴露 `https://dsh.example.com` 后，使用 `--remote --api-url https://dsh.example.com --api-token <token>` 直连。

客户端会在 dshapi 的 HTTP POST 与 WebSocket 握手请求中自动附加 `Authorization: Bearer <token>`。当 `--remote` 且远程地址不是 loopback 也不是 `https`/`wss` 时，会打印安全警告。

详细说明见 [docs/usage.md](docs/usage.md)。

## API Key 安全存储

Provider 直连模式下，API Key 除了放在 `--provider-key` 命令行中外，也可以保存到系统 Keychain / 凭据管理器：

```bash
# 首次保存 key 到 macOS Keychain / Windows Credential Manager / Linux secret-tool
go run ./cmd/agentforge --provider deepseek --provider-key sk-xxx --keychain

# 之后启动不再需要 --provider-key
go run ./cmd/agentforge --provider deepseek --keychain
```

- macOS 调用 `security`；Windows 调用 PowerShell + Credential Manager；Linux 需要安装 `secret-tool`（libsecret），未安装时会返回明确的“不支持”错误。
- 不想使用系统 Keychain 时，也可以把 Key 加密保存到本地文件 `<data-dir>/secrets.json`，主密钥来自 `--secret-key` 或环境变量 `DSH_SECRET_KEY`：

```bash
export DSH_SECRET_KEY=$(openssl rand -hex 32)
go run ./cmd/agentforge --provider deepseek --provider-key sk-xxx --secret-key "$DSH_SECRET_KEY"
go run ./cmd/agentforge --provider deepseek --secret-key "$DSH_SECRET_KEY"
```

## 可选遥测与自动更新检查

- 遥测默认关闭。通过 `--telemetry` 或 `DSH_TELEMETRY=1` 开启后，事件以 JSON Lines 写入 `<data-dir>/telemetry.log`，字段写入前会脱敏；不联网、不收集敏感信息。
- 自动更新检查目前提供 `internal/update` 的版本比较与 GitHub Latest Release 查询能力，只提示不自动下载；后续可接入启动时提示。

## 模型直连代理与离线队列

直连模型厂商的 `internal/provider` 支持 HTTP/SOCKS 代理，并提供本地 JSON 离线消息队列。

### 代理

- 在 `OpenAIConfig.ProxyURL` 中显式指定代理，支持 `http://`、`https://`、`socks5://` 与 `socks5h://`。
- 未指定 `ProxyURL` 时，自动读取 `HTTPS_PROXY`、`HTTP_PROXY`、`NO_PROXY` 环境变量。

```go
p := provider.NewOpenAI(provider.OpenAIConfig{
    BaseURL:  "https://api.openai.com/v1",
    APIKey:   "your-api-key",
    Model:    "gpt-4o-mini",
    ProxyURL: "http://127.0.0.1:7890",
})
```

### 离线队列

`internal/offline.Queue` 基于本地 JSON 文件暂存待发送消息：

```go
q := offline.NewQueue("/path/to/data-dir/offline.json")

_ = q.Enqueue("session-id", "用户消息")
items, _ := q.Pending()
_ = q.Remove(items[0].ID)
```

## 真实 DSH API 验证工具

```bash
# 先启动 DSH Web（任意空闲端口）
dsh --profile web --no-open --port 0

# 然后用 smoke 工具连接并发送 prompt
go run ./cmd/agentforge-smoke --url http://127.0.0.1:<port> --prompt "你好" --watch 8s
```

## 测试

```bash
go test ./...
go test -race ./internal/client ./internal/mock
make fuzz
```

## 安装

### 从发布包安装

从 GitHub Releases 下载对应平台的安装包：

- **macOS**：下载 `agentforge-darwin-<arch>.dmg`（若 Release 仅提供 `.zip`，则解压后使用）。打开 DMG 后将 `agentforge` 复制到 `/usr/local/bin` 或 `~/bin`：

  ```bash
  sudo cp /Volumes/agentforge-darwin-<arch>/agentforge /usr/local/bin/
  ```

- **Linux**：下载 `agentforge-linux-<arch>.tar.gz`：

  ```bash
  tar -xzf agentforge-linux-<arch>.tar.gz
  sudo install -m 0755 agentforge-linux-<arch>/agentforge /usr/local/bin/
  ```

- **Windows**：下载 `agentforge-windows-<arch>.zip`，解压后运行 `agentforge-windows-<arch>\agentforge.exe`；也可以把该目录加入 PATH。

### 从源码安装

```bash
# 方式一：安装到 ~/.local/bin
make install-local
# 或 PREFIX=/usr/local make install-local

# 方式二：Go 安装
make install

# Windows：构建/下载到 %LOCALAPPDATA%\agentforge\bin
powershell -ExecutionPolicy Bypass -File scripts\install.ps1
# 指定版本并加入用户 PATH
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Version 0.3.0 -AddToPath
```

## 发布构建

```bash
# 1. 构建多平台二进制到 dist/
make release

# 2. 打包到 dist/packages/
#    - Linux:   agentforge-linux-<arch>.tar.gz
#    - macOS:   agentforge-darwin-<arch>.dmg（无 hdiutil 时回退 .zip）
#    - Windows: agentforge-windows-<arch>.zip
make package

# 也可以一步完成：
make release && make package
```

产物位于 `dist/`（二进制）与 `dist/packages/`（安装包）。

## 依赖

- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Lip Glow](https://github.com/charmbracelet/lipgloss)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [coder/websocket](https://github.com/coder/websocket)

## 说明

本项目为个人项目，主要用于个人学习与日常使用。
