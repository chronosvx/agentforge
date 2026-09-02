# agentforge 使用文档

> 语言：中文（简体）。英文用户请先阅读 [English README](../README.en.md) 或 [English Quickstart](QUICKSTART.en.md)。

> 完全不懂编程？请先看 [QUICKSTART.md](QUICKSTART.md)：零基础下载、安装、启动、连接 AI 指南。

## 前置条件

- Go 1.24+（从源码运行时）
- 至少满足一种后端：
  - 已安装 DSH CLI：`dsh`，可启动 DSH Web 服务；
  - 有模型厂商 API Key（如 DeepSeek `sk-...`），用于 Provider 直连；
  - 或先使用内置 Mock 模式体验。

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
| `--api-url` | 推荐的真实 DSH Web API 地址（HTTP + WebSocket） |
| `--api-token` | DSH Web API 访问令牌；所有 HTTP/WebSocket 请求会附加 `Authorization: Bearer <token>` |
| `--remote` | 启用远程模式；当远程地址非 loopback 且不是 `https`/`wss` 时打印安全警告 |
| `--endpoint` | 实验性 DSH Channel 地址（预留，`tui.dsh/v1alpha1`） |
| `--workspace` | 工作区目录 |
| `--resume` | 恢复指定会话 ID |
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |
| `--mock` | 使用内置 mock server |
| `--cost-limit` | 成本告警阈值（元） |
| `--theme` | 主题：`default` / `catppuccin` / `dracula` / `tokyo-night` / `nord` / `solarized` |
| `--lang` | 界面语言：`zh` / `en`，默认 `zh` |
| `--telemetry` | 启用本地遥测，默认关闭；写入 `<data-dir>/telemetry.log`，不联网 |
| `--no-audit` | 禁用审计日志（默认开启） |
| `--headless` | 非交互模式：不启动 TUI，发送 `--prompt` 后等待流式完成并打印结果 |
| `--prompt <text>` | 与 `--headless` 搭配的提示词文本 |
| `--format <text\|json\|stream-json>` | Headless 输出格式：默认 `text` 打印最终文本；`json` 输出结构化结果；`stream-json` 输出流式 JSON Lines |
| `--no-color` | 关闭 ANSI 颜色输出；适合不支持颜色的终端、CI 日志或管道场景 |
| `--version` | 打印版本并退出 |
| `--provider` | 直连厂商名：`openai` / `deepseek` / `moonshot` / `kimi` / `zhipu` / `qwen` / `ollama` |
| `--provider-key` | 直连厂商 API Key，多个用逗号分隔 |
| `--provider-base-url` | 直连厂商 OpenAI 兼容 Base URL（可选） |
| `--model` | 直连厂商模型名（可选） |
| `--keychain` | 优先使用系统 Keychain/凭据管理器保存/读取 `--provider-key` |
| `--secret-key` | 本地加密文件主密钥（hex/base64），为空时读取 `DSH_SECRET_KEY` |

## 首次运行向导（setup）

安装后可以先运行：

```bash
agentforge setup
```

该向导面向首次运行用户，会读取已有 `<data-dir>/config.json`，以问答方式选择后端（Mock / DSH Web API / Provider 直连）并生成或更新本地配置；本次未覆盖的已有字段会保留。也可用 `--data-dir` 指定数据目录。

## 环境自检（doctor）

安装后可以先运行：

```bash
agentforge doctor
```

它会检查 Go、DSH CLI、Git、GitHub CLI、系统 Keychain 后端、数据目录权限和终端兼容性，帮助你快速定位环境问题；发现问题时会输出 `Suggested fixes`，给出可直接执行的建议修复项。终端兼容性诊断会检查当前终端是否适合运行 TUI（例如颜色/ANSI 支持、宽字符与常见终端能力），当终端不支持颜色时建议配合 `--no-color` 使用。

doctor 子命令常用参数：

| 参数 | 说明 |
|---|---|
| `--data-dir <dir>` | 自定义数据目录（默认 `~/.agentforge`） |
| `--api-url <url>` | 额外检查 DSH Web API 连通性 |
| `--api-token <token>` | DSH Web API 访问令牌；可用 `--dsh-api-token` 作为别名 |

## 项目上下文初始化（project-init）

在项目仓库中为 Agent 生成 `AGENT.md` 项目上下文：

```bash
agentforge project-init
```

该命令会在当前项目目录生成 `AGENT.md`，用于记录项目背景、目录结构、构建/测试方式与代码约定等上下文，供后续 Repo Mode、Agent 任务和项目记忆读取，帮助 Agent 更快理解项目。

## 插件市场 CLI（agentforge market）

`agentforge market` 提供插件市场相关的命令行子命令：

```bash
# 搜索插件市场中的插件
agentforge market search <关键词>

# 列出插件市场条目
agentforge market list

# 安装指定插件
agentforge market install <插件名>
```

说明：

- `search`：按关键词在插件市场索引中搜索可用插件。
- `list`：列出当前配置或默认市场索引中的全部插件。
- `install`：按插件名从市场索引安装插件到本地插件目录（通常为 `<data-dir>/plugins/`）。
- 签名校验、权限与沙箱策略仍沿用 `internal/plugin` 的既有插件治理能力。

## 非交互 Headless 模式

`--headless` 让程序不启动 TUI，直接通过当前配置的 Provider/DSH 后端发送 prompt，并把结果输出到 stdout。默认等待流式完成后打印最终文本；也可通过 `--format` 切换为结构化 JSON 或流式 JSON Lines。适合脚本、CI 和管道调用。

```bash
# Provider 直连模式
agentforge --headless --provider deepseek --provider-key sk-xxx --prompt "hello"

# 使用 DSH Web API
agentforge --headless --api-url http://127.0.0.1:54695 --api-token <token> --prompt "hello"

# 使用内置 mock server 验证
agentforge --headless --mock --prompt "hello"

# 指定模型
agentforge --headless --provider deepseek --provider-key sk-xxx --model deepseek-chat --prompt "hello"

# 结构化 JSON 输出
agentforge --headless --provider deepseek --provider-key sk-xxx --prompt "hello" --format json

# 流式 JSON Lines 输出（每行一条 JSON，便于实时消费）
agentforge --headless --provider deepseek --provider-key sk-xxx --prompt "hello" --format stream-json
```

说明：

- `--headless` 与 `--prompt` 可放在其他参数之前或之后；`--prompt` 也支持 `--prompt=hello` 形式；`--format` 同样支持 `--format=stream-json`。
- `--format` 可选 `text`（默认）、`json`、`stream-json`。`stream-json` 会把流式增量/事件逐行输出为 JSON Lines，便于管道实时处理。
- 非交互模式仍会执行与 TUI 相同的后端连接、密钥解析和审计初始化；可通过 `--no-audit` 关闭审计。
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
| `--data-dir` | 本地数据目录，默认 `~/.agentforge` |

本地执行器不调用 shell，因此 `--cmd "echo hi"` 会被拆分为 `echo hi` 后直接执行；如需管道、重定向等 shell 特性，请显式调用 `sh -c`（例如 `--cmd "sh -c 'echo hi | cat'"`）。

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

## 本地会话导出 / 导入

以下子命令不会启动 TUI，而是直接读写本地会话 JSON，适合备份、迁移与手动归档。

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

审计字段会尽量只包含 `sessionId`、`channelId`、导出路径、应用文件等非敏感信息；所有字段写入前仍会经过 `RedactMap` 脱敏。

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
| `/diff` | Diff Review 面板（`A` 全部接受、`R` 全部拒绝） |
| `/todos` | Todo / 任务计划面板 |
| `/tools` | 工具调用记录面板 |
| `/skills` | Skill 管理面板（本地 + 远端 available_skills） |
| `/plugin` | 插件列表面板 |
| `/sessions` | 本地/远端会话列表（支持按工作区分组）；面板聚焦后可用 `↑/↓` 移动、`Enter` 切换、`n` 新建 |
| `/resume` | `/sessions` 的别名，用于打开会话列表并恢复/切换会话 |
| `/search <关键词>` | 过滤会话列表 |
| `/find <关键词>` | 全文搜索历史消息 |
| `/right` | 右侧 Overview 面板 |
| `/split` | 双会话并排视图 |
| `/model [模型]` | 查看当前模型；Provider 直连时可切换模型 |
| `/status` | 查看当前状态摘要 |
| `/new` | 新建普通会话 |
| `/clear` | 清空当前会话 |
| `/compact` | 压缩当前会话旧消息为摘要，释放上下文空间 |
| `/export` | 导出当前会话为 Markdown/JSON |
| `/rename <标题>` | 手动重命名当前本地会话；该标题不会被后续自动命名覆盖 |
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
| `/mcp <url>` | 作为 MCP Client 连接 MCP 服务端并列出工具 |
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
| `/agentview` | 打开会话总览（当前会话、最近会话、运行中的多 Agent 任务） |
| `/help` | 打开帮助面板（也可在输入框为空时按 `?`，或按 `Ctrl+/`） |
| `/close` | 关闭当前 Channel |

### 会话管理

- 新会话会根据第一条用户消息自动生成标题；使用 `/rename` 手动改名后，该标题不会被后续自动命名或持久化覆盖。
- `/sessions` 面板会按工作区分组展示会话，便于在多工作区之间快速定位。
- `/clear` 清空当前会话的聊天内容，便于在当前会话中重新开始。
- `/status` 查看当前状态摘要。

### 上下文管理

- 当会话上下文接近窗口限制时，可使用 `/compact` 将除最后一条外的旧消息压缩为一条 system 摘要，降低 token 占用并继续对话。
- 压缩后旧消息不再逐条保留；若需要保留更完整的历史，可结合 `/checkpoint`、`/timeline`、`/rewind` 进行时间线保存与回滚。
- `/context` 可查看当前 Context 用量，便于判断是否值得执行 `/compact`。

### MCP 工具审批面板

当 `/mcp-call` 调用一个策略为“每次确认”（`/mcp-confirm`）的 MCP 工具时，会进入审批面板等待决定：

| 按键 | 作用 |
|---|---|
| `a` | 本次允许：只放行这一次调用 |
| `s` | 总是允许：将该工具设为自动执行（等价于 `/mcp-auto <工具>`） |
| `d` | 拒绝本次：拒绝这一次调用 |
| `n` | 拒绝并记住：将该工具设为禁止（等价于 `/mcp-deny <工具>`） |
| `Esc` / `q` | 取消本次调用并关闭面板 |

也可继续使用 `/mcp-approve`、`/mcp-cancel`、`/mcp-auto <工具>`、`/mcp-confirm <工具>`、`/mcp-deny <工具>` 管理 MCP 工具策略。

### 插件命令

如果已加载插件在 manifest 中声明了 `commands`，这些命令会自动注册进命令面板（`Ctrl+P`），并可在输入框直接执行：

- 显式格式：`/plugin:<插件名>:<命令> [参数...]`
- 短别名：当命令不与内置命令冲突时，也可用 `/<命令> [参数...]`

插件命令通过 `plugin.Manager.RunCommand` 异步执行，结果会显示在状态栏。

需要从命令行搜索、列出或安装插件市场条目时，可使用 `agentforge market search/list/install`，详见“插件市场 CLI（agentforge market）”章节。

### UI 交互与性能

- 启动时启用 Bubble Tea 鼠标支持（`tea.WithMouseCellMotion`）。
- 空聊天欢迎页：新会话还没有消息时，聊天区会显示欢迎/引导页，提示如何开始第一条消息、使用命令面板或打开帮助，降低新手启动成本。
- 底部上下文快捷键栏（原状态栏精简）：聊天区底部状态栏不再列出大量斜杠命令，而是根据当前输入框/面板焦点动态提示可用操作（例如 `Enter 发送`、`Tab 切换焦点`、`? 帮助`），让新手不需要先背快捷键。
- 鼠标滚轮可以滚动聊天消息 viewport；命令面板、Diff/Search/Plugins/Skills 等列表面板中滚轮会移动光标。
- 帮助面板：输入框为空时按 `?`，或按 `Ctrl+/` 打开；按 `Esc` 关闭。`/help` 现在会打开该帮助面板，而不再只是显示状态栏提示。帮助面板新增“全局快捷键”区块，集中展示 `Ctrl+P`、`Ctrl+/`、`Tab`、`Esc`、`↑/↓` 等不依赖当前面板的全局快捷键；面板内支持 `↑/↓` 或 `j/k` 滚动。
- 输入历史搜索：`Ctrl+R` 打开输入历史搜索，支持 `↑/↓` 选择、`Enter` 回填、`Esc` 关闭。
- 文件引用自动补全：输入 `@` 会触发当前工作区文件路径补全，选择后把 `@<路径>` 插入输入框，便于在消息中引用文件。
- 全局键位自定义：可通过 `<data-dir>/keybindings.json` 自定义 help/palette/history 等全局键位；文件使用 JSON 对象，键名对应功能（如 `help`、`palette`、`history`），值为 Bubble Tea 风格按键（如 `ctrl+p`、`ctrl+r`、`ctrl+/`）。未配置项保持默认。
- 会话总览：`/agentview` 打开会话总览，展示当前会话、最近会话与运行中的多 Agent 任务。
- 时间回溯：空输入时双击 `Esc` 打开时间回溯，可选取用户消息取回编辑并重发。
- 常驻 HUD：界面常驻显示上下文 tokens、成本 cost 与生成速度 TPS。
- `/clear` 清空当前会话聊天内容，便于快速开始新一轮对话。
- `/status` 查看当前状态摘要。
- 会话自动命名：新会话会根据第一条用户消息自动生成标题；使用 `/rename` 手动设置的标题不会被后续自动命名或持久化覆盖。
- `/sessions` 会话面板：支持按工作区分组；面板聚焦后可用 `↑/↓` 移动游标、`Enter` 切换到选中的会话、`n` 直接新建会话；`/resume` 是 `/sessions` 的别名，便于快速进入恢复/切换会话流程。
- Diff Review 面板支持 `A` 全部接受、`R` 全部拒绝，适合一次性处理整批 Diff。
- MCP 工具审批面板：当 MCP 工具需要确认时，审批面板支持 `a` 本次允许、`s` 总是允许、`d` 拒绝本次、`n` 拒绝并记住。
- 主题系统：支持 `default` / `catppuccin` / `dracula` / `tokyo-night` / `nord` / `solarized`，可通过 `--theme` 选择。
- 面板默认非模态：打开面板不会抢占输入焦点。按 `Tab` 可在“输入框 / 面板”之间切换焦点；`Esc` 先返回输入框焦点，再关闭面板。
- 命令面板（`Ctrl+P`）默认只展示基础命令以降低学习成本；在面板中按 `Tab` 可切换“基础命令 / 全部命令”。支持 `↑/↓`、`Home/End`、`Ctrl+N` 导航。
- 多 Agent 面板（`/agents`）也支持 `↑/↓` 或 `j/k` 导航：面板聚焦后可用方向键或 `j/k` 在任务/子任务之间移动。
- 输入框占位提示现在会提到 `Ctrl+P`（命令）、`Ctrl+R`（历史）和 `Ctrl+/`（帮助）。
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
| `Ctrl+P` | 打开命令面板 / 模糊搜索（默认只显示基础命令；`Tab` 切换基础/全部） |
| `Ctrl+R` | 搜索输入历史 |
| `@` | 触发文件引用与路径自动补全 |
| `/compact` | 压缩当前会话旧消息 |
| `/agentview` | 打开会话总览 |
| `/clear` | 清空当前会话 |
| `/status` | 查看当前状态摘要 |
| 空输入双击 `Esc` | 时间回溯 |
| 常驻 HUD | 显示上下文 tokens / 成本 cost / TPS |
| `?`（输入框为空时）或 `Ctrl+/` | 打开帮助面板 |
| `Esc` | 关闭帮助/面板；先返回输入框焦点 |
| `Tab` | 在输入框与当前面板之间切换焦点 |
| `↑` / `↓`、`Home` / `End`、`Ctrl+N` | 在命令面板中导航 |
| 帮助面板 `↑` / `↓` 或 `j` / `k` | 滚动帮助面板内容 |
| 多 Agent 面板 `↑` / `↓` 或 `j` / `k` | 在多 Agent 任务/子任务列表中导航 |
| 会话面板 `↑` / `↓` | 在会话列表中移动游标 |
| 会话面板 `Enter` | 切换到当前选中的会话 |
| 会话面板 `n` | 新建会话 |
| MCP 审批面板 `a` | 本次允许当前 MCP 调用 |
| MCP 审批面板 `s` | 总是允许该 MCP 工具（自动执行） |
| MCP 审批面板 `d` | 拒绝本次 MCP 调用 |
| MCP 审批面板 `n` | 拒绝并记住（禁止该 MCP 工具） |
| MCP 审批面板 `Esc` / `q` | 取消当前 MCP 调用并关闭面板 |
| 鼠标滚轮 | 滚动聊天消息；命令面板/列表面板中移动光标 |
| `Ctrl+←` / `Ctrl+→` | 切换会话 Tab |
| 右侧面板 `1`~`5` | 打开 Context / Cost / Diff / Todos / Skills |
| 双会话并排 `[` / `]` | 切换右侧会话 |
| 双会话并排 `s` | 跳到右侧会话 |
| Diff 面板 `j` / `k` | 移动 |
| Diff 面板 `a` / `r` | 接受 / 拒绝 |
| Diff 面板 `A` / `R` | 全部接受 / 全部拒绝 |
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
├── sessions/       # 本地会话：默认 <id>.json；启用加密存储后为 <id>.json.enc
├── skills/         # 本地 Skill markdown（*.md）
├── plugins/        # 本地插件
├── patches/        # 生成的 patch 文件
├── backups/        # git apply 前备份
├── keybindings.json # 自定义全局键位（可选；help/palette/history 等）
├── secrets.json    # 本地加密 API Key（AES-GCM，0600；仅在启用 --secret-key/DSH_SECRET_KEY 时生成）
├── audit.log       # 审计日志（JSON Lines，自动脱敏）
└── telemetry.log   # 可选遥测日志（JSON Lines，自动脱敏；默认不生成）
```

## 注意事项

- `git apply` 前会先 `git apply --check`，失败不会改动文件
- 应用前会自动备份涉及文件，可用 `b` 回滚
- Skill 启用/禁用状态为当前进程内切换；本地 Skill 默认启用，可用 frontmatter `enabled: false` 设置初始状态
- 当前真实 DSH 走 Web API（`internal/dshapi`，HTTP + WebSocket）；`tui.dsh/v1alpha1` Channel 为实验性/预留协议，非官方稳定接入方式。
