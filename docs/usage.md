# agentforge 使用文档

> 语言：中文（简体）。英文用户请先阅读 [English README](../README.en.md) 或 [English Quickstart](QUICKSTART.en.md)。

> 完全不懂编程？请先看 [QUICKSTART.md](QUICKSTART.md)：零基础下载、安装、启动、连接 AI 指南。

## 前置条件

- Go 1.24+
- 本机已安装 DSH CLI：`dsh`
- 需要能启动 DSH Web 服务

## 快速开始

### 1. Mock 模式（无需真实 DSH）

```bash
go run ./cmd/agentforge --mock
```

### 2. 连接真实 DSH

先启动 DSH Web：

```bash
dsh --profile web --no-open --port 0
```

输出类似：

```text
dsh web: http://127.0.0.1:54695
```

再启动 TUI：

```bash
go run ./cmd/agentforge --api-url http://127.0.0.1:54695
```

### 3. 恢复会话

```bash
go run ./cmd/agentforge --api-url http://127.0.0.1:54695 --resume <sessionId>
```

### 4. 切换界面语言

默认界面为中文；使用 `--lang en` 可切换到英文：

```bash
go run ./cmd/agentforge --mock --lang en
```

## 远程部署与认证

DSH Web API 默认监听本机 loopback。远程访问时建议通过 SSH 隧道或 HTTPS/WSS 暴露。

### 使用 SSH 隧道（推荐，最简单）

在远程主机启动 DSH Web 后，在本地建立隧道：

```bash
ssh -L 18891:127.0.0.1:18891 user@dsh-host
# 另开一个终端
go run ./cmd/agentforge --api-url http://127.0.0.1:18891 --api-token <token>
```

隧道把远程 `127.0.0.1:18891` 映射到本地，因此 URL 仍为 loopback，不需要 TLS；但推荐仍使用 token 做服务端认证。

### 直连 HTTPS/WSS

若 DSH Web API 已通过反向代理提供 HTTPS/WSS：

```bash
go run ./cmd/agentforge --remote --api-url https://dsh.example.com --api-token <token>
```

### Token 说明

- 客户端会在所有 dshapi HTTP POST 与 WebSocket 握手请求中附加 `Authorization: Bearer <token>`。
- 不要把 token 写进公开脚本或仓库；可通过环境变量或密钥管理工具注入命令行。
- 当 `--remote` 且 URL 不是 `https`/`wss` 也不是 loopback 时，程序会打印安全警告，提示 token 与数据可能明文传输。

## 模型直连代理与离线队列

### 代理

直连国外模型厂商时，可通过 `OpenAIConfig.ProxyURL` 指定 HTTP/HTTPS/SOCKS5 代理；未指定时自动读取 `HTTPS_PROXY`、`HTTP_PROXY`、`NO_PROXY` 环境变量。

```go
p := provider.NewOpenAI(provider.OpenAIConfig{
    BaseURL:  "https://api.openai.com/v1",
    APIKey:   "your-api-key",
    Model:    "gpt-4o-mini",
    ProxyURL: "http://127.0.0.1:7890",
})
```

也支持直接通过环境变量启用代理（不设置 `ProxyURL` 时生效）：

```bash
export HTTPS_PROXY=http://127.0.0.1:7890
export HTTP_PROXY=http://127.0.0.1:7890
export NO_PROXY=localhost,127.0.0.1
```

### 离线队列

`internal/offline.Queue` 基于本地 JSON 文件暂存待发送消息，适合在网络不可用时保存用户输入，恢复后再补发：

```go
q := offline.NewQueue("/path/to/data-dir/offline.json")

// 入队
if err := q.Enqueue("session-id", "用户消息"); err != nil {
    // 处理错误
}

// 查看待发送消息
items, err := q.Pending()
if err != nil {
    // 处理错误
}

// 发送成功后移除
if err := q.Remove(items[0].ID); err != nil {
    // 处理错误
}
```

队列文件格式为 JSON，`Item` 包含 `ID`、`SessionID`、`Message`、`CreatedAt`。

### 厂商 API Key 安全存储

Provider 直连模式支持把 `--provider-key` 保存到系统 Keychain / 凭据管理器，避免每次启动都输入 API Key：

```bash
# 首次保存：--provider-key 会写入系统 Keychain
agentforge --provider deepseek --provider-key sk-xxx --keychain

# 之后启动：只指定厂商，自动从系统 Keychain 读取
agentforge --provider deepseek --keychain
```

平台支持：

- **macOS**：调用 `security add-generic-password` / `find-generic-password` / `delete-generic-password`。
- **Windows**：调用 PowerShell 与 Windows Credential Manager（`CredWrite` / `CredRead` / `CredDelete`）。
- **Linux**：需要安装 `secret-tool`（通常由 `libsecret-tools` 提供）；未安装时返回明确的“不支持”错误。

如果不使用系统 Keychain，也可以使用本地加密文件存储：

```bash
# 生成或准备一个 16/24/32 字节 AES 密钥（hex 或 base64）
export DSH_SECRET_KEY=$(openssl rand -hex 32)

# 首次保存
agentforge --provider deepseek --provider-key sk-xxx --secret-key "$DSH_SECRET_KEY"

# 之后读取
agentforge --provider deepseek --secret-key "$DSH_SECRET_KEY"
# 或使用环境变量
DSH_SECRET_KEY="$DSH_SECRET_KEY" agentforge --provider deepseek
```

本地加密文件位于 `<data-dir>/secrets.json`，内容使用 AES-GCM 加密，文件权限为 `0600`。`--keychain` 开启时会优先使用系统 Keychain；未开启但提供了 `--secret-key` 或 `DSH_SECRET_KEY` 时使用本地加密文件。

## 命令行参数

| 参数 | 说明 |
|---|---|
| `--api-url` | 真实 DSH Web API 地址 |
| `--api-token` | DSH Web API 访问令牌；所有 HTTP/WebSocket 请求会附加 `Authorization: Bearer <token>` |
| `--remote` | 启用远程模式；当远程地址非 loopback 且不是 `https`/`wss` 时打印安全警告 |
| `--endpoint` | DSH tui channel WebSocket 地址（预留） |
| `--workspace` | 工作区目录 |
| `--resume` | 恢复指定会话 ID |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |
| `--mock` | 使用内置 mock server |
| `--cost-limit` | 成本告警阈值（元） |
| `--theme` | 主题：`default` / `catppuccin` |
| `--lang` | 界面语言：`zh` / `en`，默认 `zh` |
| `--telemetry` | 启用本地遥测，默认关闭；写入 `<data-dir>/telemetry.log`，不联网 |
| `--no-audit` | 禁用审计日志（默认开启） |
| `--user` | 模拟企业用户身份（可选），用于 Headless/CLI 场景；配合 `--roles` 可触发 RBAC 与 SIEM 用户标识 |
| `--roles` | 模拟用户角色，逗号分隔（可选），例如 `admin,member`；配合 `--user` 使用 |
| `--headless` | 非交互模式：不启动 TUI，发送 `--prompt` 后等待流式完成并打印结果 |
| `--prompt <text>` | 与 `--headless` 搭配的提示词文本 |
| `--version` | 打印版本并退出 |
| `--provider` | 直连厂商名：`openai` / `deepseek` / `moonshot` / `kimi` / `zhipu` / `qwen` / `ollama` |
| `--provider-key` | 直连厂商 API Key，多个用逗号分隔 |
| `--provider-base-url` | 直连厂商 OpenAI 兼容 Base URL（可选） |
| `--model` | 直连厂商模型名（可选） |
| `--keychain` | 优先使用系统 Keychain/凭据管理器保存/读取 `--provider-key` |
| `--secret-key` | 本地加密文件主密钥（hex/base64），为空时读取 `DSH_SECRET_KEY` |

## 非交互 Headless 模式

`--headless` 让程序不启动 TUI，直接通过当前配置的 Provider/DSH 后端发送 prompt，等待流式完成后把最终结果打印到 stdout。适合脚本、CI 和管道调用。

```bash
# Provider 直连模式
agentforge --headless --provider deepseek --provider-key sk-xxx --prompt "hello"

# 使用 DSH Web API
agentforge --headless --api-url http://127.0.0.1:54695 --api-token <token> --prompt "hello"

# 使用内置 mock server 验证
agentforge --headless --mock --prompt "hello"

# 指定模型
agentforge --headless --provider deepseek --provider-key sk-xxx --model deepseek-chat --prompt "hello"
```

说明：

- `--headless` 与 `--prompt` 可放在其他参数之前或之后；`--prompt` 也支持 `--prompt=hello` 形式。
- 非交互模式仍会执行与 TUI 相同的后端连接、密钥解析和审计初始化；可通过 `--no-audit` 关闭审计。
- 可通过 `--user alice --roles admin,member` 模拟企业用户身份；该身份会写入 SIEM 事件。
- 收到 `SIGINT` / `SIGTERM` 时会取消等待并退出。

## Agent 主流程 / 多 Agent

Agent 主流程 Demo：自然语言需求 → 计划 → 执行 → Diff → 测试 → 提交/回滚。

### CLI：`agentforge agent`

```bash
# 静态规划器：仅支持 run/edit/test 指令
agentforge agent --prompt "run go test ./..." --yes

# 仓库感知 + 自然语言：必须使用 LLM 规划器（需要 API Key）
agentforge agent --prompt "帮我看看这个项目怎么跑" --repo --planner llm --provider deepseek --provider-key sk-xxx --yes

# 自动提交 / 推送 / 创建 PR
agentforge agent --prompt "edit README.md hello" --yes --commit --push --pr --pr-title "docs: update readme"
```

`agentforge agent` 常用参数：

| 参数 | 说明 |
|---|---|
| `--prompt <text>` | 自然语言需求（必填） |
| `--planner static\|llm` | 规划器：`static` 只支持 `run` / `edit` / `test`；`llm` 需要 `--provider` |
| `--repo` | 扫描当前 Git 仓库并添加上下文 |
| `--workdir <dir>` | 工作目录，默认 `.` |
| `--yes` | 自动批准所有 Agent 动作 |
| `--test <cmd>` | 测试命令，默认 `go test ./...`；传空字符串可禁用测试 |
| `--timeout <dur>` | 单条命令超时，默认 `5m` |
| `--commit` | 成功后自动创建分支并提交 |
| `--push` | 提交后推送到 origin |
| `--pr` | 提交后创建 PR（需要 `gh` CLI） |
| `--pr-title <title>` | PR 标题 |
| `--pr-body <body>` | PR 描述 |
| `--provider <name>` | LLM 厂商（`--planner llm` 必填） |
| `--provider-key <key>` | LLM API Key（`--planner llm` 必填） |
| `--model <model>` | LLM 模型（可选） |
| `--base-url <url>` | OpenAI 兼容 Base URL（可选） |

> ⚠️ **安全警告**：`--yes` 会自动批准所有动作。与 `--commit`、`--push`、`--pr` 一起使用时，会在没有逐条确认的情况下创建提交、推送到远端并创建 PR。请只在可信仓库或 CI 中使用，并确保工作区已备份或可回滚。

### CLI：`agentforge multiagent`

```bash
agentforge multiagent --tasks "run echo a,run echo b,run echo c" --yes
```

`--tasks` 使用逗号分隔多个任务提示词；`--workdir`、`--yes`、`--test`、`--timeout` 等参数与 `agent` 子命令类似。

### TUI：`/multi` 与 `/agents`

`/multi` 支持逗号分隔的多个提示词，也支持 JSON 数组来声明任务依赖。输入 `/multi` 后会先准备好任务并打开面板，再输入 `/multi-run` 才会真正开始执行：

```text
/multi run echo a,run echo b,run echo c

/multi [{"id":"a","prompt":"run echo a"},{"id":"b","prompt":"run echo b","deps":["a"]}]
/multi-run
```

`/agents` 打开/关闭多 Agent 面板，面板会展示子任务依赖树、状态与输出。

## 开放 API / Webhook 服务

`agentforge-api` 提供轻量 HTTP API，让外部程序、CI 或 SDK 可以创建会话、发送 prompt、查看结果，并通过 Webhook 接收异步事件通知。服务复用 `internal/session`、`internal/provider`、`internal/dshapi` 等后端能力，不修改 TUI 核心逻辑。

```bash
# Provider 直连模式
go run ./cmd/agentforge-api --listen 127.0.0.1:8080 \
  --provider deepseek --provider-key sk-xxx --model deepseek-chat \
  --data-dir ~/.agentforge --api-token secret --webhook-url https://ci.example.com/hook

# 连接真实 DSH Web API
go run ./cmd/agentforge-api --listen 127.0.0.1:8080 \
  --api-url http://127.0.0.1:54695 --dsh-api-token <dsh-token> \
  --api-token secret --webhook-url https://ci.example.com/hook

# 使用内置 mock server 验证
go run ./cmd/agentforge-api --mock --listen 127.0.0.1:8080
```

### 启动参数

| 参数 | 说明 |
|---|---|
| `--listen` | HTTP 监听地址，默认 `127.0.0.1:8080` |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |
| `--api-token` | 开放 API 的 Bearer Token；为空时不启用鉴权 |
| `--webhook-url` | Webhook 接收地址；为空时不发送通知 |
| `--webhook-secret` | Webhook 签名 Secret，写入 `X-Webhook-Secret` |
| `--provider` | 直连厂商名：`openai` / `deepseek` / `moonshot` / `kimi` / `zhipu` / `qwen` / `ollama` |
| `--provider-key` | 直连厂商 API Key，多个用逗号分隔 |
| `--provider-base-url` | 直连厂商 OpenAI 兼容 Base URL（可选） |
| `--model` | 直连厂商模型名（可选） |
| `--api-url` | 真实 DSH Web API 地址 |
| `--dsh-api-token` | 上游 DSH Web API 访问令牌 |
| `--endpoint` | DSH tui channel WebSocket 地址 |
| `--mock` | 使用内置 mock server |
| `--workspace` | 默认工作区目录 |

### HTTP 接口

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/v1/healthz` | 健康检查，无需鉴权 |
| `POST` | `/v1/sessions` | 创建会话 |
| `GET` | `/v1/sessions/{id}` | 获取会话详情 |
| `POST` | `/v1/sessions/{id}/prompt` | 发送 prompt 并返回最终结果 |

启用 `--api-token` 后，`/v1/*` 请求需要携带：

```text
Authorization: Bearer <api-token>
```

### 示例

```bash
# 创建会话
curl -X POST http://127.0.0.1:8080/v1/sessions \
  -H 'Authorization: Bearer secret' \
  -H 'Content-Type: application/json' \
  -d '{"workspace":"/path/to/project","title":"CI build"}'

# 发送 prompt
curl -X POST http://127.0.0.1:8080/v1/sessions/sess-xxxx/prompt \
  -H 'Authorization: Bearer secret' \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"请修复测试"}'

# 查看会话
curl http://127.0.0.1:8080/v1/sessions/sess-xxxx \
  -H 'Authorization: Bearer secret'
```

### Webhook 事件

配置 `--webhook-url` 后，服务会异步发送 JSON POST 请求：

```text
POST <webhook-url>
Content-Type: application/json
X-Webhook-Secret: <webhook-secret>
```

事件体：

```json
{
  "event": "agent.approval.required",
  "payload": {
    "sessionId": "sess-xxxx",
    "actionId": "action-0",
    "phase": "AwaitingApproval"
  },
  "time": "2025-01-01T00:00:00Z"
}
```

当前事件：

- `session.created`：创建会话
- `session.prompt.completed`：prompt 执行完成
- `session.prompt.failed`：prompt 执行失败
- `agent.approval.required`：Agent 进入人工审批等待，payload 为审批快照（至少包含 `sessionId` 与待审批动作信息）

## 本地 / 容器 / 远程执行（exec）

`exec` 子命令复用 `internal/executor`，在不启动 TUI 的情况下直接执行命令，适合沙箱/远程执行场景。

```bash
# 本地执行（不经过 shell，命令会按 shell 风格拆分为程序与参数）
agentforge exec --local --cmd "echo hi"

# Docker 容器执行
agentforge exec --docker --image alpine --cmd "echo hi"

# SSH 远程执行
agentforge exec --ssh --host user@host --cmd "echo hi"

# 指定工作目录、环境变量、超时
agentforge exec --local --cmd "go test ./..." --workdir /path/to/project --env GOFLAGS=-mod=mod --timeout 5m

# SSH 指定用户和端口
agentforge exec --ssh --host example.com --user alice --port 2222 --cmd "uname -a"

# 企业身份执行：--user/--roles 放在 exec 子命令之前，触发 RBAC 与 SIEM
agentforge --user alice --roles member exec --local --cmd "echo hi"
```

参数说明：

| 参数 | 说明 |
|---|---|
| `--local` | 本地执行模式 |
| `--docker` | Docker 容器执行模式；需要同时提供 `--image` |
| `--ssh` | SSH 远程执行模式；需要同时提供 `--host` |
| `--cmd` | 要执行的命令行字符串，内部会拆分为程序与参数（必填） |
| `--image` | Docker 镜像名（`--docker` 必填） |
| `--host` | SSH 主机或 `user@host`（`--ssh` 必填） |
| `--user` | SSH 用户名（可选） |
| `--port` | SSH 端口（可选） |
| `--workdir` | 工作目录；Docker 模式会挂载到容器 `/work` 并作为工作目录 |
| `--env` | 环境变量 `KEY=VALUE`，可重复传入 |
| `--timeout` | 执行超时，例如 `5s`、`1m`；不设置表示不超时 |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge`；用于写入 SIEM 事件 |

本地执行器不调用 shell，因此 `--cmd "echo hi"` 会被拆分为 `echo hi` 后直接执行；如需管道、重定向等 shell 特性，请显式调用 `sh -c`（例如 `--cmd "sh -c 'echo hi | cat'"`）。

企业身份与 RBAC：`exec` 子命令本身保留 `--user` 作为 SSH 用户名；若要模拟企业用户身份，请把 `--user <企业用户>` 与 `--roles <角色列表>` 放在 `exec` 之前。指定了角色时会使用 `DefaultPolicy` 校验 `exec.run` 权限（admin/member 允许，auditor/guest 拒绝），并在 `<data-dir>/siem.log` 写入 `exec.run` 事件。

### TUI 内的 `/exec` 执行面板

在 TUI 中输入 `/exec` 可打开“沙箱/远程执行 UI 面板”。面板会显示执行器类型、命令、状态、stdout/stderr 与退出码。

```text
/exec
/exec local -- echo hi
/exec docker --image alpine -- echo hi
/exec ssh --host user@host -- echo hi
/exec --executor docker --image busybox --cmd "echo hi"
```

- `/exec` 单独输入：打开面板，默认使用 `local` 执行器；面板打开后直接输入普通命令并按回车即可执行。
- `/exec <type> [options] -- <command>`：指定执行器并立即执行。
- 支持 `--executor`、`--image`、`--host`、`--user`、`--port`、`--workdir`、`--env`、`--timeout`，与 CLI `exec` 子命令保持一致。
- 执行通过异步 `tea.Cmd` 完成，不会阻塞 TUI；结果会渲染在面板中，同时状态栏给出摘要。
- 在面板中按 `Esc` 可关闭面板。

## 会话导入 / 导出 / 同步

以下子命令不会启动 TUI，而是直接读写本地会话 JSON，适合多设备迁移与备份。

### 导出全部会话

```bash
agentforge export --out-dir /path/to/export
# 指定数据目录
agentforge export --data-dir ~/.agentforge --out-dir /path/to/export
```

把 `<data-dir>/sessions/` 下的所有会话导出为 JSON 文件到 `--out-dir`，文件名是 `<会话ID>.json`。

### 导入全部会话

```bash
agentforge import --in-dir /path/to/export
# 指定数据目录
agentforge import --data-dir ~/.agentforge --in-dir /path/to/export
```

从 `--in-dir` 读取 JSON 文件并写入 `<data-dir>/sessions/`。如果某个会话 ID 已存在，会跳过该文件、不覆盖本地数据；每个文件的处理结果都会打印。

### 分享单个会话链接

```bash
# 加密分享（推荐）
agentforge share --session <会话ID> --key <hex-or-base64>

# 使用环境变量提供分享密钥
DSH_SHARE_KEY=<hex-or-base64> agentforge share --session <会话ID>

# 不提供密钥时生成明文链接（plain=true，仅适合非敏感会话）
agentforge share --session <会话ID>
```

`share` 会读取 `<data-dir>/sessions/<会话ID>.json`，序列化后打包成 `dsh-share://<payload>` 链接：

- 提供 `--key` / `--share-key` 或设置 `DSH_SHARE_KEY` 时，使用 AES-GCM 加密，密钥为 hex 或 base64 编码的 16/24/32 字节值；
- 未提供密钥时生成明文 base64 链接，并在载荷中标记 `plain=true`。

### 导入分享链接

```bash
# 导入加密分享链接
agentforge open --url 'dsh-share://...' --key <hex-or-base64>

# 使用环境变量提供分享密钥
DSH_SHARE_KEY=<hex-or-base64> agentforge open --url 'dsh-share://...'

# 明文分享链接无需密钥
agentforge open --url 'dsh-share://...'

# 指定数据目录
agentforge open --url 'dsh-share://...' --data-dir ~/.agentforge
```

`open` 会解析并解密分享链接，然后写入 `<data-dir>/sessions/`。如果本地已存在相同会话 ID，会跳过且不覆盖本地数据。

分享参数说明：

| 参数 | 说明 |
|---|---|
| `--session` | 要分享的本地会话 ID（仅 `share`） |
| `--url` | 分享链接，形如 `dsh-share://...`（仅 `open`） |
| `--key` | AES-GCM 分享密钥，hex 或 base64 编码的 16/24/32 字节值；不传时读取环境变量 `DSH_SHARE_KEY`；也可用 `--share-key` 作为别名 |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |
| `--no-audit` | 禁用审计日志（默认开启） |

### 同步两个数据目录

```bash
agentforge sync --from /path/to/dir-a --to /path/to/dir-b
```

按 `UpdatedAt` 做双向同步：

- 只存在于 `--from` 的会话复制到 `--to`；
- 只存在于 `--to` 的会话复制到 `--from`；
- 两侧都存在时，较新的覆盖较旧的。

命令会打印每个被同步会话的 ID 和操作方向（`src->dst` 或 `dst->src`）。

以上目录参数既可以是包含 `sessions/` 子目录的数据目录，也可以是直接指向 `sessions/` 的目录。

### 同步远程 HTTP/对象存储后端

```bash
# 推送全部本地会话到远程
agentforge sync-remote --url https://host/sessions --push

# 从远程拉取会话到本地
agentforge sync-remote --url https://host/sessions --pull

# 指定数据目录
agentforge sync-remote --url https://host/sessions --data-dir ~/.agentforge --pull

# 持续同步：未指定方向时每次先 pull 再 push，默认每 60s 执行一次
agentforge sync-remote --url https://host/sessions --watch

# 持续同步：每 5 分钟只推送
agentforge sync-remote --url https://host/sessions --push --watch --interval 5m

# 持续同步：每 30 秒只拉取
agentforge sync-remote --url https://host/sessions --pull --watch --interval 30s
```

参数说明：

| 参数 | 说明 |
|---|---|
| `--url` | 远程存储 base URL，形如 `https://host/sessions`；Put/Get 使用 `{url}/{id}`，List 使用 `{url}?list=1` 并期望返回 JSON 字符串数组 |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |
| `--key` | AES-GCM 同步密钥，hex 或 base64 编码的 16/24/32 字节值；不传时读取环境变量 `DSH_SYNC_KEY` |
| `--push` | 上传所有本地会话到远程 |
| `--pull` | 下载远程会话到本地；已存在且本地 `UpdatedAt` 比远程更新的会话会跳过 |
| `--watch` | 持续同步模式；循环执行同步直到收到 SIGINT（Ctrl+C）或 SIGTERM 退出 |
| `--interval` | `--watch` 模式下的同步间隔，默认 `60s`；支持 `30s`、`5m`、`1h` 等 Go duration 格式 |

加密是可选的：提供 `--key` 或设置 `DSH_SYNC_KEY` 后，上传前会用 AES-GCM 加密，下载后自动解密；未提供密钥时使用明文 HTTP 同步。远程 HTTP 服务需要实现：

- `PUT {url}/{id}`：保存会话原始字节；
- `GET {url}/{id}`：读取会话原始字节；
- `GET {url}?list=1`：返回 `["id1","id2",...]` 这样的 JSON 数组。

## 本地安全能力

`internal/security` 提供本地敏感数据保护基础能力，供会话加密、状态栏/日志脱敏与审计日志使用。`internal/secret` 在此基础上提供 API Key 安全存储（系统 Keychain / 本地 AES-GCM 加密文件），使用方式见上文“厂商 API Key 安全存储”。

### 本地会话加密存储

`internal/session.EncryptedStore` 是本地会话存储的可选加密开关。它与 `session.Store` API 一致（`Save`/`Load`/`List`/`Delete`），但会把会话 JSON 使用 AES-GCM 加密后写入 `<data-dir>/sessions/<id>.json.enc`，磁盘上不保存明文。

```go
import "github.com/chronosvx/agentforge/internal/session"
import "github.com/chronosvx/agentforge/internal/security"

key, err := security.LoadKey(hexOrBase64Key) // 16/24/32 字节
store, err := session.NewEncryptedStore(dataDir, key)
if err != nil {
    // 处理错误
}
if err := store.Save(entry); err != nil {
    // 处理错误
}
loaded, err := store.Load("session-id")
```

- 新保存的会话写入 `<id>.json.enc`；同名旧版 `<id>.json` 明文文件会在保存后自动清理。
- `Load`/`List` 兼容旧版明文文件，方便从历史数据平滑迁移。
- 密钥错误时 `Load` 会返回解密失败错误；请妥善保管密钥。
- 普通未启用加密的场景仍使用 `session.NewStore`，继续写 `<id>.json` 明文文件。

### AES-GCM 加解密

```go
import "github.com/chronosvx/agentforge/internal/security"

key, err := security.LoadKey(hexOrBase64Key) // 16/24/32 字节
ciphertext, err := security.Encrypt(plain, key)
plain, err := security.Decrypt(ciphertext, key)
```

- 密文格式为 `nonce || ciphertext`，每次加密使用随机 nonce。
- `LoadKey` 支持 hex、标准/URL-safe base64（含无填充变体）。

### 脱敏

```go
safe := security.Redact("Authorization: Bearer sk-...")
// safe = "Authorization: Bearer ***"

security.RedactMap(fields) // 递归脱敏 apiKey/token/authorization/key/secret 等字段
```

`Redact` 会替换 Bearer Token、常见键值对中的 Key/Token/Secret、已知厂商密钥前缀（`sk-`、`ghp_`、`xoxb-`、`AKIA`、JWT 等）以及高熵长密钥。状态栏/日志在展示前应调用脱敏，避免 Key/Token 泄露。

### 审计日志

```go
audit, err := security.NewAuditLog(dataDir)
err = audit.Append("session.saved", map[string]any{
    "sessionId": "s1",
    "token":     "should-be-redacted",
})
```

审计日志写入 `<data-dir>/audit.log`，每行一条 JSON：

```json
{"time":"...","event":"session.saved","fields":{"sessionId":"s1","token":"***"}}
```

字段写入前会自动脱敏，且不会修改调用方传入的 map。

#### UI 审计覆盖范围

TUI 在以下关键操作发生时写入审计事件（通过 `--no-audit` 可整体关闭）：

| 事件 | 触发操作 |
|---|---|
| `message.sent` | 发送聊天消息 |
| `session.switch` | 切换会话 |
| `session.new` | 新建会话 |
| `session.export` | 导出当前会话（`/export`） |
| `session.delete` | 删除当前会话（`/delete`） |
| `diff.apply` | 执行 `git apply` |
| `session.share` | CLI `share` 子命令成功生成分享链接 |
| `session.import` | CLI `open` 子命令成功导入分享链接 |

审计字段会尽量只包含 `sessionId`、`channelId`、导出路径、应用文件等非敏感信息；所有字段写入前仍会经过 `RedactMap` 脱敏。CLI `share` / `open` 子命令也支持 `--no-audit` 关闭本次审计。

### SIEM 事件

企业版会在 `<data-dir>/siem.log` 写入 JSON Lines 格式的 SIEM 事件，便于对接集中式安全分析平台：

```json
{"time":"...","user":"alice","action":"app.start","resource":"agentforge","result":"ok"}
```

当前 CLI 已接入以下事件：

| 事件 | 触发操作 |
|---|---|
| `app.start` | 程序启动（Headless/交互模式） |
| `exec.run` | `exec` 子命令执行完成或 RBAC 拒绝 |
| `headless.complete` | Headless 任务完成（成功/失败） |

`--user` / `--roles` 提供模拟用户身份，会作为 `user` 字段写入 SIEM；未指定时 `user` 为空字符串。

## 自动化调度（agentforge-automation）

`cmd/agentforge-automation` 从 JSON 配置加载 Job，并按 Interval/Cron 调度执行命令。命令通常是 `agentforge --headless --prompt ...`，也可以是任意 shell 命令。

```bash
# 按配置持续调度
go run ./cmd/agentforge-automation --config jobs.json

# 只立即执行一次所有启用的 Job
go run ./cmd/agentforge-automation --config jobs.json --once

# 只打印将执行的命令，不真正执行
go run ./cmd/agentforge-automation --config jobs.json --dry-run
```

`jobs.json` 示例：

```json
{
  "jobs": [
    {
      "id": "daily-report",
      "name": "每日报告",
      "command": "agentforge --headless --mock --prompt \"生成今日项目报告\"",
      "cron": "0 9 * * *",
      "enabled": true
    },
    {
      "id": "ci-smoke",
      "name": "CI 冒烟",
      "command": "agentforge --headless --mock --prompt \"运行冒烟测试\"",
      "interval": "5s",
      "enabled": true
    }
  ]
}
```

字段说明：

| 字段 | 说明 |
|---|---|
| `id` | 任务 ID（必填） |
| `name` | 任务名称（可选，默认取 `id`） |
| `command` | 要执行的 shell 命令（必填） |
| `cron` | 5 段 cron 表达式，例如 `0 9 * * *`（与 `interval` 二选一） |
| `interval` | Go duration 字符串，例如 `5s`、`1h`（与 `cron` 二选一） |
| `enabled` | 是否启用，默认 `true` |

调度器会向子进程注入 `DSH_AUTOMATION_JOB_ID`、`DSH_AUTOMATION_JOB_NAME` 与 `DSH_DATA_DIR` 环境变量，供命令或审计使用。

## 可选遥测与自动更新检查

### 本地遥测

遥测默认关闭。以下任一方式开启：

```bash
# 命令行参数
agentforge --telemetry

# 环境变量
DSH_TELEMETRY=1 agentforge
```

开启后，`internal/telemetry.Record` 会把事件以 JSON Lines 追加到 `<data-dir>/telemetry.log`：

```json
{"time":"...","event":"app.started","fields":{"version":"0.2.0"}}
```

- 所有 `fields` 写入前会深拷贝并调用 `security.RedactMap` 脱敏，不会修改调用方 map。
- 遥测只写本地文件，不联网、不收集 API Key、Token、消息内容等敏感信息。
- 文件与目录权限分别为 `0600` / `0700`。

### 自动更新检查

`internal/update` 提供：

- `Check(currentVersion, latestVersion string) (bool, error)`：语义化版本比较，返回是否有更新。
- `LatestFromGitHub(owner, repo string) (string, error)`：查询 GitHub 最新 Release 的 `tag_name`，仅用于未来接入启动提示。

当前不会自动下载或安装新版本，只负责“检查并提示”。

## 内置命令

| 命令 | 作用 |
|---|---|
| `/context` | Context 用量面板 |
| `/cost` | 成本面板 |
| `/diff` | Diff Review 面板 |
| `/todos` | Todo / 任务计划面板 |
| `/tools` | 工具调用记录面板 |
| `/skills` | Skill 管理面板（本地 + 远端 available_skills） |
| `/plugin` | 插件列表面板 |
| `/sessions` | 本地/远端会话列表 |
| `/search <关键词>` | 过滤会话列表 |
| `/find <关键词>` | 全文搜索历史消息 |
| `/right` | 右侧 Overview 面板 |
| `/split` | 双会话并排视图 |
| `/model [模型]` | 查看当前模型；Provider 直连时可切换模型 |
| `/new` | 新建普通会话 |
| `/export` | 导出当前会话为 Markdown/JSON |
| `/rename <标题>` | 重命名当前本地会话 |
| `/delete` | 删除当前本地会话 |
| `/skill-install <本地目录\|git URL>` | 从本地目录或 git 仓库安装/更新 Skill |
| `/skill-sync <manifest-url>` | 从 JSON manifest 同步远端 Skill 仓库清单 |
| `/agent` | 打开/关闭 Agent 执行/审批面板；`/agent new` 新建会话 |
| `/agent-new` | 新建 Agent 执行会话 |
| `/agent-reset` | 重置 Agent 执行会话 |
| `/checkpoint <名称>` | 保存当前会话 Checkpoint 到时间线 |
| `/rewind <ID\|名称>` | 回滚到指定 Checkpoint 或时间线快照 |
| `/timeline` | 查看 Checkpoint 时间线 |
| `/fork <标签>` | 从当前会话分叉一个新的时间线分支 |
| `/mcp <url>` | 连接 MCP Server 并列出工具 |
| `/mcp-call <工具> <json>` | 调用 MCP 工具 |
| `/mcp-approve` | 批准当前挂起的 MCP 调用 |
| `/mcp-cancel` | 取消当前挂起的 MCP 调用 |
| `/mcp-auto <工具>` | 设置 MCP 工具为自动执行 |
| `/mcp-confirm <工具>` | 设置 MCP 工具为每次确认 |
| `/mcp-deny <工具>` | 禁止 MCP 工具 |
| `/mcp-audit` | 查看最近的 MCP 调用审计 |
| `/remember <键> <值>` | 保存一条项目记忆 |
| `/forget <键>` | 删除一条项目记忆 |
| `/memory [查询]` | 列出或搜索项目记忆 |
| `/exec [local\|docker\|ssh] [options] -- <命令>` | 打开沙箱/远程执行面板并运行命令 |
| `/multi <提示词1,提示词2,...>` 或 `/multi [{"id":"a","prompt":"...","deps":["b"]}]` | 准备多 Agent 任务（JSON 形式支持依赖）并打开面板；用 `/multi-run` 执行 |
| `/multi-run` | 执行 `/multi` 已准备的多 Agent 任务 |
| `/agents` | 打开/关闭多 Agent 面板 |
| `/help` | 帮助 |
| `/close` | 关闭当前 Channel |

### 插件命令

如果已加载插件在 manifest 中声明了 `commands`，这些命令会自动注册进命令面板（`Ctrl+P`），并可在输入框直接执行：

- 显式格式：`/plugin:<插件名>:<命令> [参数...]`
- 短别名：当命令不与内置命令冲突时，也可用 `/<命令> [参数...]`

插件命令通过 `plugin.Manager.RunCommand` 异步执行，结果会显示在状态栏。

### 插件市场签名发布

插件作者可以使用 `scripts/publish-plugin.sh`（或 `cmd/agentforge-plugin-publish`）为插件包计算 SHA-256、使用 ECDSA P-256 私钥签名，并把条目写入市场索引 JSON。

先生成 ECDSA P-256 私钥（同时导出公钥用于信任库）：

```bash
openssl ecparam -name prime256v1 -genkey -noout -out publisher-key.pem
openssl ec -in publisher-key.pem -pubout -out publisher-pub.pem
```

发布插件到本地市场索引：

```bash
./scripts/publish-plugin.sh \
  my-plugin \
  1.0.0 \
  https://example.com/my-plugin.zip \
  ./publisher-key.pem \
  ./market.json \
  "我的插件"
```

等价地，也可以直接调用 Go 命令：

```bash
go run ./cmd/agentforge-plugin-publish \
  my-plugin 1.0.0 https://example.com/my-plugin.zip \
  ./publisher-key.pem ./market.json "我的插件"
```

或通过 Makefile：

```bash
make publish-plugin \
  NAME=my-plugin \
  VERSION=1.0.0 \
  URL=https://example.com/my-plugin.zip \
  KEY=./publisher-key.pem \
  INDEX=./market.json
```

说明：

- 包 URL 支持 `http(s)://`、`file://` 和本地文件路径；发布前会计算包本身的 SHA-256。
- 签名对象中的 `publicKeyID` 由公钥 DER 的 SHA-256 指纹自动生成（格式为 `sha256:<hex>`）。
- 市场索引支持顶层数组或 `{"plugins":[...]}` 两种格式；`AppendToIndex` 按插件名去重，同名条目会被更新。
- 消费者启用签名校验时，需要把 `publisher-pub.pem` 以 `publicKeyID` 为 ID 加入 `TrustStore`：

```go
ts := plugin.NewTrustStore()
ts.AddPublicKey(entry.Signature.PublicKeyID, publisherPublicKeyPEM)
plugin.InstallFromMarket(ctx, index, "my-plugin", destDir, ts)
```

### 鼠标与性能

- 启动时启用 Bubble Tea 鼠标支持（`tea.WithMouseCellMotion`）。
- 鼠标滚轮可以滚动聊天消息 viewport；命令面板、Diff/Search/Plugins/Skills 等列表面板中滚轮会移动光标。
- 本地会话持久化采用节流策略：状态变化后最多每 500ms 写盘一次，退出程序前会强制把未落盘数据写入 `<data-dir>/sessions/`。
- 状态栏使用 `•` / `⚠` / `✖` 符号前缀区分普通/告警/错误，避免只依赖颜色传达状态。
- 终端宽度小于 80 列时，自动隐藏右侧 Overview 面板和双会话分屏，优先保证主聊天区可读。

## Skill 安装与同步

`/skill-install` 支持两种来源：

- 本地目录：直接复制该目录下所有 `*.md` 到 `<data-dir>/skills/`。
- Git 仓库 URL：使用 `git clone --depth 1` 克隆到临时目录，再安装其中的 `*.md`。需要本机已安装 `git`。

示例：

```text
/skill-install ./my-skills
/skill-install https://github.com/example/dsh-skills.git
```

`/skill-sync` 拉取一个 JSON manifest，逐个安装清单中的 Skill。manifest 支持顶层数组，也支持 `{"skills":[...]}` 或 `{"items":[...]}`：

```json
[
  {"name": "browser", "url": "https://github.com/example/dsh-skill-browser.git", "description": "浏览器自动化"},
  {"name": "local-ops", "url": "/path/to/local-skills", "description": "本地运维 Skill"}
]
```

manifest URL 可以是 HTTP(S) 地址，也可以是本地文件路径。执行完成后 `/skills` 面板会自动刷新。

## 快捷键

| 按键 | 作用 |
|---|---|
| `Ctrl+C` | 第一次停止生成，第二次退出 |
| `Ctrl+J` | 输入框内换行（多行输入） |
| `Ctrl+E` | 用 `$EDITOR` 编辑当前输入 |
| `Ctrl+P` | 打开命令面板 / 模糊搜索 |
| 鼠标滚轮 | 滚动聊天消息；命令面板/列表面板中移动光标 |
| `Ctrl+←` / `Ctrl+→` | 切换会话 Tab |
| 右侧面板 `1`~`5` | 打开 Context / Cost / Diff / Todos / Skills |
| 双会话并排 `[` / `]` | 切换右侧会话 |
| 双会话并排 `s` | 跳到右侧会话 |
| Diff 面板 `j` / `k` | 移动 |
| Diff 面板 `a` / `r` | 接受 / 拒绝 |
| Diff 面板 `p` | 生成 patch |
| Diff 面板 `g` | 进入/确认 `git apply` |
| Diff 面板 `b` | 回滚最近一次 apply |
| Diff 面板 `Esc` | 取消 apply 确认 |
| Skills 面板 `j` / `k` | 选择 Skill |
| Skills 面板 `Space` | 启用 / 禁用 Skill |
| Skills 面板 `Enter` | 查看 Skill 详情（再次 Enter / Esc 返回） |

## 真实 DSH 交互验收

```bash
./scripts/smoke-real.sh
```

脚本会：

1. 启动 DSH Web
2. 创建会话
3. 发送 prompt
4. 观察 mux 事件流
5. 自动关闭 DSH Web

## 安装

### macOS

从 GitHub Releases 下载 `agentforge-darwin-<arch>.dmg`（`<arch>` 为 `amd64` 或 `arm64`）。打开 DMG 后把 `agentforge` 复制到 `/usr/local/bin` 或 `~/bin`：

```bash
sudo cp /Volumes/agentforge-darwin-<arch>/agentforge /usr/local/bin/
```

如果 Release 只提供 `.zip`（例如在无 `hdiutil` 的 CI 上打包），解压后同样复制 `agentforge-darwin-<arch>/agentforge`：

```bash
unzip agentforge-darwin-<arch>.zip
sudo cp agentforge-darwin-<arch>/agentforge /usr/local/bin/
```

### Linux

从 GitHub Releases 下载 `agentforge-linux-<arch>.tar.gz`，解压并安装：

```bash
tar -xzf agentforge-linux-<arch>.tar.gz
sudo install -m 0755 agentforge-linux-<arch>/agentforge /usr/local/bin/
```

### Windows

从 GitHub Releases 下载 `agentforge-windows-<arch>.zip`，解压后运行 `agentforge-windows-<arch>\agentforge.exe`，也可以把该目录加入 PATH。

也可以使用 PowerShell 安装脚本，脚本会安装到 `$env:LOCALAPPDATA\agentforge\bin`：

```powershell
# 有 Go：从源码构建；没有 Go：尝试下载最新 GitHub Release
powershell -ExecutionPolicy Bypass -File scripts\install.ps1

# 指定版本下载
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -Version 0.2.0

# 安装并加入用户 PATH（新终端生效）
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -AddToPath
```

脚本默认只打印 PATH 提示；使用 `-AddToPath` 才会修改当前用户 PATH。

### 从源码安装

```bash
# 安装到 ~/.local/bin（或 PREFIX=/usr/local）
make install-local

# 或使用 Go 直接安装
make install
```

## 数据目录

默认 `~/.agentforge`：

```text
~/.agentforge/
├── sessions/     # 本地会话：默认 <id>.json；启用加密存储后为 <id>.json.enc
├── skills/       # 本地 Skill markdown（*.md）
├── patches/      # 生成的 patch 文件
├── backups/      # git apply 前备份
├── secrets.json  # 本地加密 API Key（AES-GCM，0600；仅在启用 --secret-key/DSH_SECRET_KEY 时生成）
├── audit.log     # 审计日志（JSON Lines，自动脱敏）
├── siem.log      # SIEM 事件日志（JSON Lines，企业安全事件）
└── telemetry.log # 可选遥测日志（JSON Lines，自动脱敏；默认不生成）
```

## 注意事项

- `git apply` 前会先 `git apply --check`，失败不会改动文件
- 应用前会自动备份涉及文件，可用 `b` 回滚
- Skill 启用/禁用状态为当前进程内切换；本地 Skill 默认启用，可用 frontmatter `enabled: false` 设置初始状态
- 当前真实 DSH 走 Web API；`tui.dsh/v1alpha1` Channel 客户端已实现但需服务端支持
