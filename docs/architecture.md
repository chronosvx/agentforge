# agentforge 架构

## 分层

```text
cmd/agentforge        入口：解析配置、初始化 store/backend、启动 Bubble Tea
cmd/agentforge-smoke      真实 DSH Web API 连接/发消息/观察事件 的验证工具
internal/config       配置与 CLI 参数
internal/client       DSH tui.dsh/v1alpha1 Channel 客户端（WebSocket，实验性/预留；真实接入以 internal/dshapi 的 DSH Web API 为主）
internal/dshapi       DSH Web API 客户端（/api HTTP + WebSocket 下行事件）
internal/dshapi       还提供 Backend 适配器，把真实 DSH 接入 UI
internal/mcp          MCP 客户端基础：JSON-RPC 2.0 消息、HTTP/SSE 与 stdio 传输、工具列表/调用
internal/mcpserver    MCP Server：注册工具、处理 initialize/tools/list/tools/call、stdio/HTTP 传输
internal/provider     直连厂商层：Provider 抽象、OpenAI 兼容客户端、多 API Key 轮询、路由与故障转移、错误映射、成本定价表、模型上下文窗口注册表、请求幂等与流中断续传
internal/session      会话状态与本地持久化
internal/persist      通用异步持久化队列（Writer）
internal/contextx     长会话上下文窗口管理、压缩摘要、Checkpoint/Rewind
internal/security     本地敏感数据保护：AES-GCM 加解密、日志/状态栏脱敏、审计日志
internal/search       历史消息全文搜索索引（内存、大小写不敏感子串匹配）
internal/memory       项目级记忆本地持久化与关键词/子串检索（预留 Embedder 接口）
internal/skill        Skill 本地扫描、available_skills 解析与合并
internal/plugin       插件运行时基础：插件市场/索引、签名与信任链、权限与沙箱、举报/黑名单/审核状态、外部/内置插件发现、注册、执行与事件钩子
internal/executor     统一命令执行抽象：本地直接执行、Docker 容器执行、SSH 远程执行（未来安全执行后端）
internal/agent        Agent 执行闭环后端：计划、审批、执行、测试、Diff 评审、回滚状态机与动作权限
internal/orchestrator 多 Agent 协作与任务编排：复杂任务拆分为子任务、依赖就绪、并行调度、失败传播与汇总
internal/collab       会话协作：评论、审批记录、操作回放事件的本地持久化
internal/enterprise   企业基础能力：RBAC、SSO 身份认证、本地 KMS、SIEM 事件预留
internal/cost        成本估算与预算管理（Estimator/Budget/BudgetManager）
internal/markdown    Markdown/代码块渲染：ANSI 着色、代码块边框/行号、控制字符清理
internal/ui           Bubble Tea UI（聊天视图、输入框、状态栏、流式渲染）
internal/mock         本地 mock Channel 服务，用于无 DSH 环境下的开发/测试
```

## 包间接口（第一里程碑约定）

### internal/client

`internal/client` 对应实验性/预留的 `tui.dsh/v1alpha1` Channel；真实接入以 `internal/dshapi` 的 DSH Web API 为主。

```go
type Message struct {
    Role    string `json:"role"`
    Content string `json:"content"`
}

type Snapshot struct {
    WireRevision int            `json:"wireRevision"`
    ChannelID    string         `json:"channelId"`
    Version      int64          `json:"version"`
    State        map[string]any `json:"state"`
}

type Event struct {
    Operation string         `json:"operation"`
    Version   int64          `json:"version"`
    State     map[string]any `json:"state,omitempty"`
    Value     any            `json:"value,omitempty"`
}

type Client struct { /* ... */ }

func Dial(ctx context.Context, endpoint string) (*Client, error)
func (c *Client) Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*Snapshot, error)
func (c *Client) Subscribe(ctx context.Context, channelID string, after int64) (<-chan Event, error)
func (c *Client) Invoke(ctx context.Context, channelID, method string, args []any) (any, error)
func (c *Client) Close(ctx context.Context, channelID string) error
func (c *Client) CloseConn() error
```

### internal/mcp（MCP 客户端基础）

```go
type Tool struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type Client struct { /* ... */ }

func New(url string) *Client
func (c *Client) Initialize(ctx context.Context) error
func (c *Client) ListTools(ctx context.Context) ([]Tool, error)
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (any, error)
func (c *Client) Close() error
```

设计要点：

- `New(url)` 根据 URL 自动选择传输：`http://`/`https://` 走 HTTP/SSE（JSON-RPC over HTTP POST，兼容 `text/event-stream` 响应），其余字符串视为 stdio 可执行命令。
- stdio 使用 `os/exec` 启动子进程，按 MCP 约定在 stdin/stdout 上交换换行分隔的 JSON-RPC 2.0 消息；stderr 仅用于诊断，不参与协议解析。
- `Initialize` 发送 MCP `initialize` 请求完成握手；`ListTools` 解析 `tools/list` 返回的 `[]Tool`；`CallTool` 发送 `tools/call` 并返回服务端原始结果（包含 `content`、`isError` 等字段）。
- JSON-RPC 消息封装位于 `internal/mcp` 包内，包含 request/response/error 类型与编解码辅助函数，后续可扩展 notifications、batch 与完整 SSE session 管理。
- 当前为客户端基础能力，未接入 `internal/ui`；后续可在不修改 UI 核心逻辑的前提下，将 MCP 工具映射为内部统一 Tool 描述并接入权限/审计。

### internal/mcpserver（MCP Server 基础）

```go
type Tool = mcp.Tool

type Server struct { /* ... */ }

func New() *Server
func (s *Server) RegisterTool(name string, handler func(args map[string]any) (any, error)) error
func (s *Server) Handle(ctx context.Context, data []byte) []byte
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)
func (s *Server) ServeStdio(ctx context.Context, in io.Reader, out io.Writer) error
```

设计要点：

- `RegisterTool` 注册工具执行函数；工具描述中的 `inputSchema` 默认提供空对象结构，调用方后续可按需扩展描述。
- `Handle` 接收一条 JSON-RPC 2.0 请求字节，处理 `initialize`、`tools/list`、`tools/call`，返回对应响应字节；无 ID 的通知消息不返回响应。
- `tools/call` 支持从 `params.arguments` 读取参数，调用注册的 handler；handler 返回的字符串会被包装为 MCP `content[0].text` 结果，返回 map 且已包含 `content` 时直接透传；handler 返回错误时以 `isError: true` 的 MCP 结果返回，而不是 JSON-RPC 协议错误。
- `ServeHTTP` 提供 HTTP POST JSON-RPC 传输，适合挂载到 `http.Handler`；`ServeStdio` 按 MCP stdio 约定从 stdin 读取换行分隔的 JSON-RPC 并只向 stdout 写 JSON-RPC 响应。
- 该包复用 `internal/mcp` 的 `Tool` 与 JSON-RPC 消息类型，不依赖 `internal/ui`，可独立用于 CLI、测试或服务进程。

### internal/session

```go
type Entry struct {
    ID        string
    Workspace string
    Title     string
    CreatedAt time.Time
    UpdatedAt time.Time
    Messages  []client.Message
}

type Store struct { /* ... */ }

func NewStore(dir string) (*Store, error)
func (s *Store) List() ([]Entry, error)
func (s *Store) Load(id string) (*Entry, error)
func (s *Store) Save(entry Entry) error
func (s *Store) Delete(id string) error

type EncryptedStore struct { /* 包装 Store，落盘为 AES-GCM 密文 */ }

func NewEncryptedStore(dir string, key []byte) (*EncryptedStore, error)
func (s *EncryptedStore) List() ([]Entry, error)
func (s *EncryptedStore) Load(id string) (*Entry, error)
func (s *EncryptedStore) Save(entry Entry) error
func (s *EncryptedStore) Delete(id string) error

type AsyncStore struct { /* 包装 Store，Save 异步，Load/Delete 同步 */ }

func NewAsyncStore(store *Store, buffer int) *AsyncStore
func (a *AsyncStore) List() ([]Entry, error)
func (a *AsyncStore) Load(id string) (*Entry, error)
func (a *AsyncStore) Save(entry Entry) error
func (a *AsyncStore) Delete(id string) error
func (a *AsyncStore) Flush(ctx context.Context) error
func (a *AsyncStore) Close() error
func (a *AsyncStore) Errors() <-chan error
```

设计要点：

- `Store` 默认以明文 JSON 写入 `<data-dir>/sessions/<id>.json`。
- `EncryptedStore` 提供相同 API，使用 `internal/security` 的 AES-GCM 将会话 JSON 加密后写入 `<data-dir>/sessions/<id>.json.enc`，不落明文。
- `EncryptedStore` 兼容旧数据：`Load`/`List` 可读取原有 `<id>.json` 明文文件；保存同名会话后会清理旧明文文件，`Delete` 同时清理两种后缀。
- 加密开关由调用方决定：普通场景继续使用 `Store`，需要本地密文存储时改用 `NewEncryptedStore(dir, key)`，密钥为 16/24/32 字节 AES 密钥。
- `AsyncStore` 通过 `internal/persist` 将 `Save` 放入后台队列，调用方无需等待写盘；`Load`/`Delete` 保持同步，`Delete` 会先等待已提交的 `Save` 完成，避免异步队列打乱删除语义。
- UI 后续可在不修改核心逻辑的前提下，将传入 `Model` 的 `*session.Store` 替换为 `*session.AsyncStore`（或为 `Model` 增加可选异步存储适配），并保留现有节流策略。

### internal/persist

```go
type Writer struct { /* ... */ }

func NewWriter(buffer int) *Writer
func (w *Writer) Submit(save func() error) error
func (w *Writer) Flush(ctx context.Context) error
func (w *Writer) Close() error
func (w *Writer) Errors() <-chan error
```

设计要点：

- `Writer` 是通用异步持久化队列：`Submit` 将任务加入无界队列并立即返回，后台 worker 串行执行，避免并发写盘与 UI 同步阻塞。
- `Flush(ctx)` 等待当前已提交任务全部完成；`Close()` 停止接收新任务并等待队列清空。
- 任务执行错误通过 `Errors()` 通道收集，不影响 worker 继续处理后续任务。
- `buffer` 用于预分配队列容量，不是队列上限；提交不会被缓冲区占满阻塞。

### internal/security

```go
func Encrypt(data, key []byte) ([]byte, error)
func Decrypt(ciphertext, key []byte) ([]byte, error)
func LoadKey(hexOrBase64 string) ([]byte, error)

func Redact(s string) string
func RedactMap(m map[string]any)

type AuditLog struct { /* ... */ }

func NewAuditLog(dataDir string) (*AuditLog, error)
func (a *AuditLog) Append(event string, fields map[string]any) error
```

设计要点：

- `Encrypt` 使用 AES-GCM 返回 `nonce || ciphertext`；`Decrypt` 负责反向解密，密钥必须是 16/24/32 字节。
- `LoadKey` 接受 hex 或 base64（含 URL-safe 与无填充变体）编码的 AES 密钥。
- `Redact` 用于日志/状态栏等展示前脱敏：替换 Bearer Token、常见键值对中的 Key/Token/Secret、已知厂商密钥前缀（`sk-`、`ghp_`、`xoxb-`、`AKIA`、JWT 等）以及高熵长密钥。
- `RedactMap` 递归脱敏 map 中的常见敏感字段（apiKey、token、authorization、key、secret、password 等），可安全用于结构化日志字段。
- `AuditLog` 以 JSON Lines 写入 `<data-dir>/audit.log`，字段写入前自动深拷贝并脱敏；日志文件与目录权限分别使用 `0600`/`0700`。
- 该包为本地加密、脱敏与审计能力的基础工具；接入本地会话加密或 UI 状态栏时不应改动 `internal/ui` 核心逻辑，可在存储层/展示层调用本包。

### internal/search

```go
type Result struct {
    SessionID    string
    MessageIndex int
    Role         string
    Content      string
    Snippet      string
}

type Index struct { /* ... */ }

func (idx *Index) Add(entry session.Entry)
func (idx *Index) AddAsync(entry session.Entry)
func (idx *Index) AddAll(entries []session.Entry)
func (idx *Index) Search(query string, limit int) []Result
```

设计要点：

- 当前为进程内内存索引，零值可用；`Add`/`AddAsync` 对同一会话 ID 会替换旧消息，`AddAll` 按传入顺序批量加入。
- `Add`/`AddAsync` 只更新对应会话并失效该会话的小写缓存，不会重建整个索引，适合大会话场景增量更新。
- `Search` 对消息 `Content` 做大小写不敏感的子串匹配；`limit <= 0` 表示不限制返回数量。
- 命中结果携带会话 ID、消息下标、角色、原文和围绕首个命中位置的 snippet（过长时用 `…` 截断）。
- 后续如需持久化或增量更新，可在该包内增加磁盘索引与增量同步，不影响 `internal/ui` 与 `internal/dshapi`。

### internal/memory

```go
type Entry struct {
    ID        string
    Project   string
    Key       string
    Content   string
    Tags      []string
    UpdatedAt time.Time
}

type MemoryStore struct { /* ... */ }

func NewMemoryStore(dir string) (*MemoryStore, error)
func (s *MemoryStore) Save(ctx context.Context, entry Entry) error
func (s *MemoryStore) Get(ctx context.Context, project, key string) (*Entry, error)
func (s *MemoryStore) List(ctx context.Context, project string) ([]Entry, error)
func (s *MemoryStore) Delete(ctx context.Context, project, key string) error

type Searcher struct { /* ... */ }

func NewSearcher(store *MemoryStore) *Searcher
func (s *Searcher) Search(project, query string, limit int) []Entry

type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

设计要点：

- 项目记忆以 JSON 数组形式保存在 `<data-dir>/memory/<project>.json`，每个文件对应一个项目的全部记忆条目。
- `Save` 使用同项目下 `Key` 作为唯一键，重复保存同一 `Key` 会覆盖旧条目；写入使用临时文件加 rename 保证原子性。
- `Get`/`List`/`Delete` 按项目隔离；`List` 按 `UpdatedAt` 倒序、`Key` 升序返回。
- `Searcher` 提供本地关键词/子串检索：查询按空白拆分为多个关键词，条目需在 `Content`、`Key` 或 `Tags` 中同时包含全部关键词才命中，大小写不敏感，`limit <= 0` 表示不限制。
- `Embedder` 是预留接口，后续可接入本地 embedding/向量检索；当前实现不依赖外部服务。

### internal/contextx（上下文窗口、压缩摘要、Checkpoint/Rewind）

```go
type Message = client.Message

type TokenEstimator func(text string) int64
func DefaultTokenEstimator(text string) int64

type Window struct { /* ... */ }

func NewWindow() *Window
func NewWindowWithEstimator(estimate TokenEstimator) *Window
func (w *Window) Add(message Message)
func (w *Window) Messages() []Message
func (w *Window) Len() int
func (w *Window) TokenCount() int64
func (w *Window) TrimToLimit(limit int64) []Message
func (w *Window) Checkpoint(id string) Checkpoint
func (w *Window) Rewind(cp Checkpoint)
func (w *Window) Compact(limit int64, summarizer Summarizer) ([]Message, error)

type Summary struct {
    Text       string
    TokenCount int64
    CreatedAt  time.Time
    Meta       map[string]any
}

type Summarizer interface {
    Summarize(messages []Message) (Summary, error)
}

type SummarizeFunc func(messages []Message) (Summary, error)
type SimpleSummarizer struct { MaxTokens int64 }
func SummarizeMessages(messages []Message, maxTokens int64) Summary
func (s Summary) AsMessage(role string) Message

type Checkpoint struct {
    ID         string
    CreatedAt  time.Time
    Messages   []Message
    TokenCount int64
    Note       string
}

type CheckpointStore struct { /* ... */ }

func NewCheckpointStore() *CheckpointStore
func NewFileCheckpointStore(dir string) (*CheckpointStore, error)
func (s *CheckpointStore) Save(cp Checkpoint) error
func (s *CheckpointStore) Load(id string) (Checkpoint, error)
func (s *CheckpointStore) Delete(id string) error
func (s *CheckpointStore) List() ([]Checkpoint, error)
```

设计要点：

- 包名使用 `contextx` 而非 `context`，避免与标准库 `context` 冲突。
- `Window` 管理消息列表与 token 估算；默认按约 4 字符/token 的启发式估算，后续可替换为按模型的真实 tokenizer 或厂商 usage。
- `TrimToLimit` 从头部裁剪最旧消息直到不超过 limit，并返回被裁剪消息供摘要/归档；即使最新一条超限也至少保留一条。
- `Summary` 预留 `Summarizer` 接口：当前 `SimpleSummarizer`/`SummarizeMessages` 做拼接与截断，后续可接入 LLM 压缩实现而不改动 `Window`。
- `Window.Compact` 将旧消息压缩为一条 system 摘要消息；摘要仍超限时回退为简单裁剪。
- `Checkpoint` 是不可变窗口快照；`Window.Checkpoint` 生成快照，`Window.Rewind` 恢复快照。
- `CheckpointStore` 默认内存存储；`NewFileCheckpointStore(dir)` 额外以 JSON 原子写入 `<dir>/<id>.json`（文件权限 `0600`），支持跨进程/重启恢复。
- 该包不依赖 `internal/ui`，可被会话存储、provider 请求构造或后续 UI 适配层独立接入。

### internal/config

```go
type Config struct {
    Endpoint  string // 默认 ws://127.0.0.1:18891/agentforge/v1/connection
    Workspace string
    SessionID string
    DataDir   string // 默认 ~/.agentforge
    Mock      bool   // true 时连接内置 mock server
}

func Parse(args []string) (*Config, error)
```

### internal/ui

```go
type Model struct { /* Bubble Tea model */ }

func New(cfg *config.Config, cl *client.Client, store *session.Store) Model
func (m Model) Init() tea.Cmd
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m Model) View() string
```

### internal/plugin（插件系统基础包）

```go
type HookName string

const (
    HookOnMessage       HookName = "on_message"
    HookOnToolResult    HookName = "on_tool_result"
    HookOnDiff          HookName = "on_diff"
    HookOnCost          HookName = "on_cost"
    HookOnSessionSwitch HookName = "on_session_switch"
)

type Manifest struct {
    Name        string
    Version     string
    Description string
    Commands    []string
    Hooks       []string
}

type Plugin struct {
    Manifest   Manifest
    Executable string // 外部可执行文件路径；内置插件为空
}

type Manager struct { /* ... */ }

func NewManager(workDir string) *Manager
func (m *Manager) LoadDir(dir string) error
func (m *Manager) List() []Plugin
func (m *Manager) RunCommand(name string, args []string) (string, error)
func (m *Manager) RegisterBuiltin(name string, fn func(args []string) (string, error)) error
func (m *Manager) RegisterHook(pluginName string, hook HookName, fn func(event map[string]any) error) error
func (m *Manager) Emit(eventName HookName, event map[string]any) []error

type Signature struct {
    PublicKeyID string
    Signature   string // base64 编码的 ECDSA P-256 ASN.1 签名
}

type TrustStore struct { /* ... */ }

func NewTrustStore() *TrustStore
func (ts *TrustStore) AddPublicKey(id, pem string) error
func (ts *TrustStore) VerifyWithTrust(manifestData []byte, sig Signature) error

type MarketIndex struct {
    Name        string
    Version     string
    Description string
    URL         string
    SHA256      string
    Signature   Signature
}

type MarketIndexEntry = MarketIndex

type PublishEntry struct {
    Name        string
    Version     string
    Description string
    URL         string
    SHA256      string
    Signature   Signature
}

type Permissions struct {
    AllowNetwork     bool
    AllowExec        bool
    AllowFileWrite   bool
    AllowReadAnyPath bool
}

func LoadMarketIndex(data []byte) ([]MarketIndex, error)
func FetchMarketIndex(ctx context.Context, url string) ([]MarketIndex, error)
func InstallFromMarket(ctx context.Context, index []MarketIndex, name, destDir string, trustStores ...*TrustStore) (Plugin, error)
func PreparePublish(entry MarketIndexEntry, privateKeyPEM string) (PublishEntry, error)
func AppendToIndex(indexPath string, entry PublishEntry) error
func CheckPermission(perm Permissions, op string) error
func RunSandboxed(ctx context.Context, executable string, args []string, workDir string, timeout time.Duration, perm Permissions) (string, error)

func Sign(data []byte, privateKeyPEM string) (string, error)
func Verify(data []byte, signature, publicKeyPEM string) error

type ReviewStatus string

const (
    StatusPending     ReviewStatus = "pending"
    StatusApproved    ReviewStatus = "approved"
    StatusRejected    ReviewStatus = "rejected"
    StatusBlacklisted ReviewStatus = "blacklisted"
)

type Report struct {
    Plugin    string
    Reason    string
    Reporter  string
    CreatedAt time.Time
}

type ModerationStore struct { /* ... */ }

func NewModerationStore() *ModerationStore
func (s *ModerationStore) AddReport(r Report) error
func (s *ModerationStore) Reports(plugin string) []Report
func (s *ModerationStore) Status(name string) ReviewStatus
func (s *ModerationStore) SetStatus(name string, status ReviewStatus) error
func (s *ModerationStore) Blacklist(name string) error
func (s *ModerationStore) Unblacklist(name string) error
func (s *ModerationStore) IsBlacklisted(name string) bool
```

设计要点：

- 外部插件：扫描 `<dir>` 下 `*.dsh-plugin.json` 清单，并加载同目录同名可执行文件（或清单中 `executable` 指定的文件）。
- 内置插件：通过 `RegisterBuiltin` 注册纯 Go 函数，例如 `echo`。
- 执行外部插件：直接 `os/exec`，不经过 shell；工作目录固定为 `Manager` 的 `workDir`；默认 30s 超时，可用 `SetTimeout` 调整。
- 事件钩子：内置插件通过 `RegisterHook` 订阅；外部插件在 manifest 的 `hooks` 字段中声明支持的事件。`Emit` 会调用所有已注册钩子，并对外部插件以 `--hook <hookName>` + JSON 事件参数直接 exec（不经过 shell）调用。
- 错误收集：`Emit` 收集所有钩子错误并继续执行，不中断后续钩子。
- 事件钩子运行时已提供，UI 接入后续进行。
- 市场/仓库索引：`MarketIndex` 描述插件名、版本、说明、下载地址、SHA256 与可选签名。`LoadMarketIndex` 支持顶层数组或 `{"plugins":[...]}` 两种 JSON 格式；`FetchMarketIndex` 支持 HTTP(S) URL、`file://` 与本地文件路径。
- 插件安装：`InstallFromMarket` 将插件安装到 `destDir/<name>`，支持 HTTP(S) 插件包（zip、tar、tar.gz、原始可执行文件）或 git 仓库克隆。SHA256 非空时，HTTP 包校验下载文件本身，git 仓库校验克隆后排除 `.git` 的确定性文件树哈希；校验失败会中止安装。
- 签名与信任链：`Sign`/`Verify` 使用 ECDSA P-256 对条目数据的 SHA-256 摘要签名/验签；`TrustStore` 维护可信公钥 ID 到 PEM 公钥的映射。调用 `InstallFromMarket` 时若传入非空 `TrustStore`，安装前必须通过签名校验；未知公钥、缺失签名或签名不匹配都会中止安装。
- 签名发布：`PreparePublish` 从包 URL（支持 HTTP(S)、`file://`、本地路径）计算 SHA-256，对去掉 `signature` 字段的规范 JSON 调用 `Sign` 生成 ECDSA P-256 签名，并自动以公钥 DER 的 SHA-256 指纹作为 `PublicKeyID`。`AppendToIndex` 将条目写入市场索引 JSON，支持顶层数组与 `{"plugins":[...]}` 两种格式，并按插件名去重/更新。
- 权限模型：`Permissions` 默认全拒绝（最小权限）。`CheckPermission` 对 `network/net`、`exec/execute`、`file_write/write`、`read_any_path/read` 做简单校验。
- 沙箱执行：`RunSandboxed` 直接 `os/exec` 启动可执行文件，不经过 shell；限制工作目录为 `workDir`；支持超时。当前不实现真实网络沙箱：默认拒绝网络，且若 `AllowNetwork=true` 会返回 `ErrNetworkSandboxUnsupported`，避免在无隔离保证时放行网络。
- 市场治理：`ModerationStore` 保存举报记录（`Report`），维护 `pending/approved/rejected/blacklisted` 审核状态；`Blacklist`/`Unblacklist` 控制黑名单，`IsBlacklisted` 可供安装或调用前做前置检查。

### internal/executor（统一命令执行抽象）

```go
type Spec struct {
    Command string
    Args    []string
    WorkDir string
    Env     []string
    Timeout time.Duration
}

type Result struct {
    Stdout   string
    Stderr   string
    ExitCode int
}

type Executor interface {
    Run(ctx context.Context, spec Spec) (Result, error)
}

type LocalExecutor struct{}
func NewLocalExecutor() *LocalExecutor
func (e *LocalExecutor) Run(ctx context.Context, spec Spec) (Result, error)

type DockerExecutor struct {
    Image   string
    BinPath string // 可选，测试或非 PATH 安装时覆盖 docker 可执行文件
}
func NewDockerExecutor(image string) *DockerExecutor
func (e *DockerExecutor) Available() bool
func (e *DockerExecutor) Run(ctx context.Context, spec Spec) (Result, error)

type SSHExecutor struct {
    Host    string
    User    string
    Port    int
    BinPath string // 可选，测试或非 PATH 安装时覆盖 ssh 可执行文件
}
func NewSSHExecutor(host, user string, port int) *SSHExecutor
func (e *SSHExecutor) Available() bool
func (e *SSHExecutor) Run(ctx context.Context, spec Spec) (Result, error)
```

设计要点：

- `LocalExecutor` 直接使用 `os/exec` 启动 `Command`，不经过 shell；支持工作目录、环境变量覆盖与超时。
- `DockerExecutor` 使用 `docker run --rm -v <workdir>:/work -w /work <image> <cmd>` 在一次性容器中执行；`Spec.Env` 通过 `-e` 传入容器；`Available()` 同时检查 docker CLI 与 daemon 是否可达。
- `SSHExecutor` 使用 `ssh <host> -- <cmd>` 在远程主机执行；支持 `User`（`user@host`）与 `Port`（`-p`）；`Available()` 检查 ssh CLI 与 Host 是否已配置。
- 该包定位为未来安全执行后端：本地、容器、SSH 可作为可替换后端，上层只需面向 `Executor` 接口编程；当前不修改 `internal/ui` 核心逻辑。

### internal/agent（Agent 执行闭环与审批引擎）

```go
type Phase string

const (
    PhasePlanning         Phase = "Planning"
    PhaseAwaitingApproval Phase = "AwaitingApproval"
    PhaseExecuting        Phase = "Executing"
    PhaseTesting          Phase = "Testing"
    PhaseReviewingDiff    Phase = "ReviewingDiff"
    PhaseRollingBack      Phase = "RollingBack"
    PhaseDone             Phase = "Done"
    PhaseFailed           Phase = "Failed"
)

type ActionType string

const (
    ActionTypeEditFile   ActionType = "edit_file"
    ActionTypeRunCommand ActionType = "run_command"
    ActionTypeApplyDiff  ActionType = "apply_diff"
    ActionTypeRollback   ActionType = "rollback"
)

type ApprovalPolicy int

const (
    AllowAll        ApprovalPolicy = iota
    RequireApproval
    DenyAll
)

type Action struct {
    ID          string
    Type        ActionType
    Description string
    Command     string
    Path        string
    Content     string
    // 运行时状态
    Status            ActionStatus
    Output            string
    TestOK            bool
    RollbackRequested bool
    RolledBack        bool
}

type RollbackState string

const (
    RollbackNone      RollbackState = "none"
    RollbackRequested RollbackState = "requested"
    RollbackCompleted RollbackState = "completed"
)

type PermissionRule struct {
    ActionType      ActionType
    Allow           bool
    RequireApproval bool
}

type PermissionSet struct {
    Rules []PermissionRule
}

func NewPermissionSet(rules ...PermissionRule) *PermissionSet
func (s *PermissionSet) AddRule(rule PermissionRule)
func (s *PermissionSet) Check(action Action) (allowed bool, requireApproval bool)

type AgentSession struct {
    Phase         Phase
    Actions       []Action
    ApprovalQueue []string
    Policy        ApprovalPolicy
    Permissions   *PermissionSet
    Diff          string
    RollbackState RollbackState
}

func NewAgentSession(policies ...ApprovalPolicy) *AgentSession
func (s *AgentSession) SetPermissionSet(ps *PermissionSet)
func (s *AgentSession) Plan(actions []Action)
func (s *AgentSession) NextPendingAction() *Action
func (s *AgentSession) Approve(id string) error
func (s *AgentSession) Reject(id string) error
func (s *AgentSession) MarkExecuted(id, output string) error
func (s *AgentSession) MarkTested(id string, ok bool) error
func (s *AgentSession) RequestRollback(id string) error
func (s *AgentSession) Rollback(id string) error
func (s *AgentSession) SetDiff(diff string)
func (s *AgentSession) PendingApprovals() []Action
```

设计要点：

- 状态机覆盖 `Planning -> AwaitingApproval -> Executing -> Testing -> ReviewingDiff`，失败或拒绝进入 `Failed`，回滚经 `RollingBack` 后进入 `Done`。
- `Plan` 重置会话并载入动作；`NextPendingAction` 按顺序取出下一个待处理动作：需要审批时进入 `AwaitingApproval` 并加入审批队列，否则直接进入 `Executing`。
- `Approve`/`Reject` 操作审批队列中的动作；拒绝会使会话进入 `Failed`。
- `MarkExecuted` 记录执行输出并进入测试；`MarkTested(id, true)` 继续闭环，`MarkTested(id, false)` 进入 `Failed`，可由 `RequestRollback`/`Rollback` 完成回滚。
- `PermissionSet` 默认拒绝；`Check` 按规则添加顺序首个匹配生效。`ApprovalPolicy` 与权限集组合：`DenyAll` 直接拒绝，`AllowAll` 不要求审批但受权限集约束，`RequireApproval` 对所有允许的动作要求审批。
- 本包不依赖 `internal/ui`，是后续 TUI/CLI/服务端接入执行闭环的后端基础。

### internal/collab（会话协作与回放）

```go
type Comment struct {
    ID        string
    SessionID string
    Author    string
    Content   string
    CreatedAt time.Time
}

type Approval struct {
    ID        string
    SessionID string
    ActionID  string
    Approver  string
    Decision  string
    CreatedAt time.Time
}

type ReplayEvent struct {
    Seq       int
    Type      string
    Data      any
    CreatedAt time.Time
}

type CollabStore struct { /* ... */ }

func NewStore(dir string) (*CollabStore, error)
func NewCollabStore(dir string) (*CollabStore, error)
func (s *CollabStore) AddComment(comment Comment) error
func (s *CollabStore) AddApproval(approval Approval) error
func (s *CollabStore) AddReplayEvent(sessionID string, event ReplayEvent) error
func (s *CollabStore) ListComments(sessionID string) ([]Comment, error)
func (s *CollabStore) ListApprovals(sessionID string) ([]Approval, error)
func (s *CollabStore) Replay(sessionID string) ([]ReplayEvent, error)
```

设计要点：

- 每个会话的评论、审批记录与回放事件统一保存在 `<data-dir>/collab/<session>.json`，文件为 JSON 对象，包含 `comments`、`approvals`、`replayEvents` 三个数组。
- `AddComment`/`AddApproval` 从实体自带的 `SessionID` 定位会话文件；`AddReplayEvent` 因 `ReplayEvent` 不携带会话 ID，由调用方显式传入 `sessionID`。
- `ListComments`/`ListApprovals` 按添加顺序返回；`Replay` 按 `Seq` 升序返回，相同 `Seq` 保持写入顺序，便于稳定回放。
- 写入使用临时文件加 rename 的原子方式，目录权限 `0700`，文件由 `os.CreateTemp` 以 `0600` 创建；会话 ID 会校验，防止路径穿越。
- 本包不依赖 `internal/ui`，可被 TUI、CLI 或后续团队服务端独立接入。

### internal/enterprise（企业基础能力：RBAC/SSO/KMS/SIEM 预留）

```go
type Role string

const (
    RoleAdmin   Role = "admin"
    RoleMember  Role = "member"
    RoleAuditor Role = "auditor"
    RoleGuest   Role = "guest"
)

func KnownRoles() []Role
func ParseRole(s string) (Role, error)

type Policy struct { /* ... */ }

func NewPolicy() *Policy
func DefaultPolicy() *Policy
func (p *Policy) Allow(role Role, actions ...string)
func (p *Policy) Can(role Role, action string) bool
func (p *Policy) Actions(role Role) []string

type RBAC struct { /* ... */ }

func NewRBAC(policy *Policy) *RBAC
func (r *RBAC) Check(user *User, action string) bool
func (r *RBAC) CheckRole(role Role, action string) bool

type User struct {
    ID    string
    Name  string
    Email string
    Roles []Role
}

type IdentityProvider interface {
    Authenticate(ctx context.Context, token string) (*User, error)
}

type StaticProvider struct { /* ... */ }

func NewStaticProvider() *StaticProvider
func (p *StaticProvider) AddUser(token string, user *User) error
func (p *StaticProvider) RemoveUser(token string)
func (p *StaticProvider) Authenticate(ctx context.Context, token string) (*User, error)

type KeyManager interface {
    Encrypt(ctx context.Context, plain []byte) ([]byte, error)
    Decrypt(ctx context.Context, cipher []byte) ([]byte, error)
}

type LocalKMS struct { /* ... */ }

func NewLocalKMS(masterKey []byte) (*LocalKMS, error)
func NewLocalKMSFromConfig(encoded string) (*LocalKMS, error)
func (k *LocalKMS) Encrypt(ctx context.Context, plain []byte) ([]byte, error)
func (k *LocalKMS) Decrypt(ctx context.Context, cipher []byte) ([]byte, error)

type SIEMEvent struct {
    Time     time.Time
    User     string
    Action   string
    Resource string
    Result   string
}

type SIEMSink interface {
    Emit(event SIEMEvent) error
}

type LogSink struct { /* ... */ }

func NewLogSink(dataDir string) (*LogSink, error)
func (s *LogSink) Path() string
func (s *LogSink) Emit(event SIEMEvent) error
```

设计要点：

- RBAC 提供最小角色集合（admin/member/auditor/guest）与可扩展 `Policy`。`DefaultPolicy` 为常见企业场景提供默认权限：admin 全量、member 日常读写与审批、auditor 只读审计/SIEM、guest 会话只读；也可用 `NewPolicy` + `Allow` 自定义。
- `RBAC.Check(user, action)` 基于用户 `Roles` 做多角色判断，任一角色允许即通过；服务端内部也可用 `CheckRole` 按单一角色校验。
- SSO 抽象为 `IdentityProvider` 接口；`StaticProvider` 是本地用户表实现，便于测试与单机部署，后续可替换为 OIDC/SAML/LDAP 等云身份提供商。
- KMS 抽象为 `KeyManager` 接口；`LocalKMS` 使用 AES-GCM 和本地主密钥（16/24/32 字节），`NewLocalKMSFromConfig` 支持从 hex/base64 配置读取密钥，与 `internal/security` 密钥格式兼容。未来可新增云 KMS 实现而不改调用方。
- SIEM 抽象为 `SIEMSink` 接口；`LogSink` 以 JSON Lines 追加写入 `<data-dir>/siem.log`，目录权限 `0700`、文件权限 `0600`，事件 Time 为零值时自动使用当前 UTC 时间。
- 本包不依赖 `internal/ui`，为后续接入服务端鉴权、密钥托管与安全审计预留最小接口。

### internal/orchestrator（多 Agent 协作与任务编排）

```go
type Status string

const (
    StatusPending Status = "pending"
    StatusReady   Status = "ready"
    StatusRunning Status = "running"
    StatusDone    Status = "done"
    StatusFailed  Status = "failed"
)

type TaskStatus = Status

const (
    TaskStatusPending TaskStatus = "pending"
    TaskStatusRunning TaskStatus = "running"
    TaskStatusDone    TaskStatus = "done"
    TaskStatusFailed  TaskStatus = "failed"
)

type SubTask struct {
    ID        string
    AgentID   string
    Action    agent.Action
    Status    Status
    DependsOn []string
}

type Task struct {
    ID          string
    Name        string
    Description string
    SubTasks    []SubTask
}

type Summary struct {
    TaskID  string
    Total   int
    Pending int
    Ready   int
    Running int
    Done    int
    Failed  int
    Status  TaskStatus
}

type Orchestrator struct { /* ... */ }

func New() *Orchestrator
func NewOrchestrator() *Orchestrator
func (o *Orchestrator) AddTask(task Task) error
func (o *Orchestrator) NextReady(taskIDs ...string) *SubTask
func (o *Orchestrator) MarkDone(args ...string) error
func (o *Orchestrator) MarkFailed(args ...string) error
func (o *Orchestrator) Status(taskIDs ...string) TaskStatus
func (o *Orchestrator) Summary(taskIDs ...string) Summary
```

设计要点：

- `Orchestrator` 管理一个或多个 `Task`，每个 `Task` 可拆分为多个 `SubTask`，并分配给不同 `AgentID` 对应的 `AgentSession`。
- `SubTask.DependsOn` 声明简单依赖；只有所有依赖子任务都进入 `StatusDone` 后，该子任务才会被 `NextReady` 返回。
- `NextReady` 返回满足依赖条件的子任务，并将其标记为 `StatusRunning`；不传任务 ID 时按任务添加顺序在所有任务中查找，传任务 ID 时只在该任务内查找。
- 没有显式的“开始”方法：`NextReady` 即负责领取并标记运行，随后由调用方把 `SubTask.Action` 交给对应 `AgentSession` 执行，完成后调用 `MarkDone`/`MarkFailed`。
- `MarkFailed` 会级联失败所有直接或间接依赖失败子任务的下游子任务，避免失败链上的子任务继续被调度；与失败链无关的并行子任务不受影响。
- `Status` 返回单个任务或全部任务的总体状态：任一子任务失败为 `failed`，全部完成则为 `done`，有运行/就绪子任务为 `running`，否则为 `pending`。
- `Summary` 返回任务或全局的子任务计数快照（总数、待处理、就绪、运行、完成、失败）与总体状态，便于上层展示和汇总结果。
- 本包不依赖 `internal/ui`，只依赖 `internal/agent` 的 `Action` 类型，是后续 TUI/CLI/服务端接入多 Agent 编排的后端基础。

### internal/cost（成本估算与预算管理）

```go
type Estimator struct {
    InputPerMillion     float64 // 元/百万 tokens
    OutputPerMillion    float64
    CacheReadPerMillion float64
    CacheWritePerMillion float64
}

func NewEstimator() *Estimator
func (e *Estimator) Estimate(inputTokens, outputTokens, cacheReadTokens, cacheWriteTokens int64) float64
func (e *Estimator) EstimateFromUsage(usage map[string]any) float64
func EstimateFromUsageWithPricing(usage map[string]any, pricing provider.Pricing) float64

type Budget struct {
    Total     float64
    Used      float64
    Remaining float64
}

func NewBudget(total float64) *Budget
func (b *Budget) Add(amount float64)
func (b *Budget) Exceeded() bool

type BudgetManager struct { /* ... */ }

func NewBudgetManager() *BudgetManager
func (m *BudgetManager) AddBudget(provider, model string, total float64)
func (m *BudgetManager) Spend(provider, model string, amount float64)
func (m *BudgetManager) Remaining(provider, model string) float64
func (m *BudgetManager) Summary() map[string]float64
```

设计要点：

- 单价默认参考 DeepSeek 常见价格：未命中输入 1.5 元/百万、输出 4.5 元/百万、缓存读取 0.05 元/百万、缓存写入按未命中输入 1.5 元/百万保守估算。
- `EstimateFromUsage` 兼容 DSH `tokenUsage` 字段名（`uncachedInputTokens`、`outputTokens`、`cacheReadTokens`、`cacheWriteTokens`）及常见别名。
- `EstimateFromUsageWithPricing` 接收 `provider.Pricing`：`pay_as_you_go` 按单价计算，`token_plan` 简单返回包月固定成本 `MonthlyFee`。
- `Budget` 可用于按会话/项目维护已用金额与超支判断；当前 UI 尚未接入，后续可在 `internal/dshapi` 或 `internal/ui` 中把 `tokenUsage` 投影转换为成本。
- `BudgetManager` 按 `provider/model` 维度维护多个独立预算，线程安全；`AddBudget` 设置/覆盖预算，`Spend` 记录已用金额（未设置预算时按总额 0 创建以便反映超支），`Remaining` 返回剩余金额，`Summary` 返回 `"provider/model" -> 剩余金额` 的快照。

### internal/provider（直连厂商层）

```go
type PlanType string

const (
    PlanTypePayAsYouGo PlanType = "pay_as_you_go"
    PlanTypeTokenPlan  PlanType = "token_plan"
)

type Pricing struct {
    Provider             string
    Model                string
    InputPerMillion      float64
    OutputPerMillion     float64
    CacheReadPerMillion  float64
    CacheWritePerMillion float64
    Currency             string
    PlanType             PlanType
    MonthlyFee           float64
}

type PricingTable struct { /* ... */ }

func DefaultPricing() Pricing
func NewPricingTable() *PricingTable
func DefaultPricingTable() *PricingTable
func (t *PricingTable) Register(p Pricing)
func (t *PricingTable) Get(provider, model string) (Pricing, bool)
func (t *PricingTable) All() []Pricing

type ModelInfo struct {
    Name              string
    ContextWindow     int64
    MaxOutput         int64
    SupportsTools     bool
    SupportsStreaming bool
}

type ModelRegistry struct { /* ... */ }

func DefaultModelInfo() ModelInfo
func NewModelRegistry() *ModelRegistry
func DefaultModelRegistry() *ModelRegistry
func (r *ModelRegistry) Register(info ModelInfo)
func (r *ModelRegistry) Get(name string) (ModelInfo, bool)
func (r *ModelRegistry) Default() ModelInfo

type KeyRing struct { /* ... */ }

func NewKeyRing(keys ...string) *KeyRing
func (r *KeyRing) Add(key string)
func (r *KeyRing) Next() (string, error)
func (r *KeyRing) MarkFailed(key string)
func (r *KeyRing) Reset(key string)

type Kind string

const (
    RateLimit           Kind = "rate_limit"
    InsufficientBalance Kind = "insufficient_balance"
    AuthError           Kind = "auth_error"
    Timeout             Kind = "timeout"
    Network             Kind = "network"
    BadRequest          Kind = "bad_request"
    ServerError         Kind = "server_error"
    Unknown             Kind = "unknown"
)

type ProviderError struct {
    Kind      Kind
    Message   string
    Retryable bool
}

func MapHTTPError(statusCode int, body string) *ProviderError
func IsRetryable(err error) bool
```

设计要点：

- `Pricing` 描述厂商/模型成本单价：按量付费使用每百万 token 单价，`token_plan` 使用 `MonthlyFee` 作为包月固定成本。
- `PricingTable` 支持注册与查询；`Get` 未精确命中时返回 `DefaultPricing()`（DeepSeek 兜底）并将 bool 置为 false。
- `DefaultPricingTable()` 预置 DeepSeek、OpenAI、Moonshot、Zhipu、Qwen、Ollama 常用模型的估算价格；价格仅为默认估算值，可后续通过 `Register` 覆盖或从配置加载。
- `provider.Pricing` 是 `cost.Pricing` 的类型别名，`Provider` 接口继续通过 `CostEstimator() *cost.Estimator` 暴露默认估算器；上层也可用 `internal/cost.EstimateFromUsageWithPricing` 按 `Pricing` 动态估算。
- `ModelInfo` 描述模型的上下文窗口（`ContextWindow`）、最大输出（`MaxOutput`）与能力标记（`SupportsTools`、`SupportsStreaming`），供 UI 展示/告警与后续上下文管理使用。
- `ModelRegistry` 支持注册与查询；`Get` 未精确命中时返回 `DefaultModelInfo()`（DeepSeek-chat 兜底）并将 bool 置为 false。
- `DefaultModelRegistry()` 预置 DeepSeek、OpenAI、Moonshot/Kimi、Zhipu、Qwen、Ollama 常见模型的上下文窗口信息；数值为默认估算值，可后续通过 `Register` 覆盖或从配置加载。
- `KeyRing` 管理同一厂商的多个 API Key，线程安全；`Next` 按轮询顺序返回可用 Key，并跳过已 `MarkFailed` 的 Key；`Reset` 可让 Key 重新参与轮询。
- 当 KeyRing 为空时返回 `ErrNoKeys`；所有 Key 都失败时返回 `ErrAllKeysFailed`。
- `MapHTTPError` 将常见 HTTP 状态码映射为 `ProviderError`：429→`RateLimit`，401/403→`AuthError`，402 或响应体包含余额/欠费关键词→`InsufficientBalance`，5xx→`ServerError`，其余 4xx→`BadRequest`。
- `ProviderError` 的 `Retryable` 标记用于调用方判断是否应重试：限流、服务端错误、超时和网络错误通常可重试；认证、余额不足和请求参数错误通常不可重试。
- `IsRetryable` 支持 `%w` 包装后的 `*ProviderError`，便于在现有错误链中复用。

#### Provider 抽象与 OpenAI 兼容客户端

```go
type Message struct {
    Role       string
    Content    string
    ToolCalls  []ToolCall
    ToolCallID string
}

type ToolCall struct {
    ID        string
    Type      string
    Name      string
    Arguments string
}

type Usage struct {
    InputTokens      int64
    OutputTokens     int64
    CacheReadTokens  int64
    CacheWriteTokens int64
}

type ChatRequest struct {
    Model       string
    Messages    []Message
    Tools       []Tool
    Stream      bool
    Temperature *float64
    Headers     map[string]string // 额外 HTTP header，不进入 JSON body
}

type ChatResponse struct {
    Content      string
    ToolCalls    []ToolCall
    Usage        *Usage
    FinishReason string
}

type StreamEventType string

const (
    StreamEventText     StreamEventType = "text"
    StreamEventToolCall StreamEventType = "tool_call"
    StreamEventUsage    StreamEventType = "usage"
    StreamEventDone     StreamEventType = "done"
    StreamEventError    StreamEventType = "error"
)

type StreamEvent struct {
    Type     StreamEventType
    Text     string
    ToolCall *ToolCall
    Usage    *Usage
    Err      error
}

type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)
    Name() string
    Model() string
    CostEstimator() *cost.Estimator
}

type OpenAIConfig struct {
    BaseURL        string
    APIKey         string
    Model          string
    Timeout        time.Duration
    IdempotencyKey string // 可选：为请求附加 Idempotency-Key header
}

func NewOpenAI(cfg OpenAIConfig) Provider
```

设计要点：

- `Provider` 是直连模型厂商的统一抽象，支持非流式 `Chat` 与流式 `StreamChat`；`CostEstimator` 返回单价估算器。
- `NewOpenAI` 实现 OpenAI 兼容协议：`Chat` 请求 `POST {BaseURL}/chat/completions`，`StreamChat` 解析 SSE `data:` 帧。
- 流式解析支持 `delta.content`、`delta.tool_calls`、`usage` 与 `[DONE]`；工具调用参数按 `index` 累积后通过 `tool_call` 事件输出。
- `OpenAIConfig.BaseURL` 可配置为任意 OpenAI 兼容根地址：OpenAI（`https://api.openai.com/v1`）、DeepSeek（`https://api.deepseek.com`）、Moonshot（`https://api.moonshot.cn/v1`）、智谱（`https://open.bigmodel.cn/api/paas/v4`）、Ollama（`http://localhost:11434/v1`）等。
- `APIKey` 为空时（如本地 Ollama）不发送 `Authorization` 头。
- HTTP 错误统一通过 `MapHTTPError` 映射为 `*ProviderError`，便于上层按 `IsRetryable` 决定重试。
- `ChatRequest.Headers` 用于携带请求级额外 header（如 `X-Request-Id`、`X-Resume-Offset`），不会进入 JSON 请求体。
- `Chat`/`StreamChat` 自动生成 `X-Request-Id`；配置 `IdempotencyKey` 后同时发送 `Idempotency-Key`，用于减少重复扣费。

#### 请求幂等与流中断续传

```go
const RequestIDHeader = "X-Request-Id"
const IdempotencyKeyHeader = "Idempotency-Key"

func RequestID() string
func WithRequestID(req ChatRequest, id string) ChatRequest

var ErrResumeUnsupported = errors.New("provider: resume unsupported")

type StreamResumer struct { /* ... */ }

func NewStreamResumer(p Provider) *StreamResumer
func (r *StreamResumer) Record(ev StreamEvent)
func (r *StreamResumer) EventIndex() int
func (r *StreamResumer) CharOffset() int
func (r *StreamResumer) Offset() int
func (r *StreamResumer) Reset()
func (r *StreamResumer) Resume(ctx context.Context, req ChatRequest, lastOffset int) (<-chan StreamEvent, error)
```

设计要点：

- `RequestID` 使用 `crypto/rand` 生成 UUID v4 风格请求 ID；`WithRequestID` 把 ID 写入 `ChatRequest.Headers["X-Request-Id"]`，由 OpenAI 客户端透传为 HTTP header。
- `StreamResumer` 记录已收到的 SSE 事件索引（`EventIndex`）与文本字符偏移（`CharOffset`/`Offset`），调用方可在断流后把偏移量作为 `lastOffset` 传给 `Resume`。
- `Resume` 要求底层 Provider 实现可选的 `ResumeStream`；OpenAI 兼容客户端通过 `X-Resume-Offset` header 发起续传。不支持续传的 Provider 返回 `ErrResumeUnsupported`。
- 当前续传是尽力而为：服务端若不支持 `X-Resume-Offset`，会按普通流或错误响应处理，由上层决定是否降级为完整重发。

#### 厂商迁移与会话消息转换

```go
type providerMessage = Message

var ErrUnsupportedProvider = errors.New("provider: unsupported provider for session conversion")

func ToOpenAIMessages(msgs []providerMessage) []map[string]any
func ConvertSessionForProvider(provider string, messages []map[string]any) ([]map[string]any, error)
```

设计要点：

- `providerMessage` 复用 `provider.Message`，作为厂商迁移的通用消息载体。
- `ToOpenAIMessages` 把通用消息转为 OpenAI 兼容 `messages`：`system/user` 保留 `role/content`；`assistant` 展开 `ToolCalls` 为 `tool_calls`（`id/type/function.name/function.arguments`）；`tool` 写入 `tool_call_id` 关联 assistant 的工具调用。
- `ConvertSessionForProvider` 目前支持所有 OpenAI 兼容厂商（`openai`、`deepseek`、`moonshot/kimi`、`zhipu`、`qwen`、`ollama`），直接返回原 messages；遇到暂不支持的厂商返回 `ErrUnsupportedProvider`，避免发送不兼容格式。
- 该转换是厂商/模型迁移的前置步骤：同一会话历史可先转成通用 `providerMessage`，再按目标厂商转换后发送。

#### 路由与故障转移

```go
type ProviderWithPriority struct {
    Provider Provider
    Priority int
    Weight   int
}

type Router struct { /* ... */ }

var ErrNoProvider = errors.New("provider: no provider available")

func NewRouter() *Router
func (r *Router) Add(p Provider, priority int)
func (r *Router) AddWeighted(p Provider, priority, weight int)
func (r *Router) SetRetryAfter(d time.Duration)
func (r *Router) Providers() []ProviderWithPriority
func (r *Router) Next(ctx context.Context) (Provider, error)
func (r *Router) MarkFailed(p Provider)
func (r *Router) Reset(p Provider)

type FailoverRouter struct { /* ... */ }

func NewFailoverRouter(args ...any) *FailoverRouter
func NewFailoverRouterWithProviders(providers ...Provider) *FailoverRouter
func (f *FailoverRouter) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
func (f *FailoverRouter) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error)

type AggregateError struct {
    Errors []error
}
```

设计要点：

- `Router` 默认按 `Priority` 从高到低选择厂商；同一优先级内按轮询（round-robin）分配。
- `MarkFailed` 会暂时屏蔽失败厂商，`Next` 自动跳过；默认冷却 30 秒后自动恢复，可通过 `SetRetryAfter` 调整，也可用 `Reset` 手动恢复。
- `FailoverRouter` 包装 `Router` 和多个 `Provider`：`Chat`/`StreamChat` 依次尝试，遇到 `*ProviderError`（或 `IsRetryable` 为 true 的错误）切换到下一个；全部失败时返回 `*AggregateError`。
- `StreamChat` 只在流建立前切换：一旦 `Provider.StreamChat` 成功返回 channel，后续流内错误由调用方处理，不再切换厂商。
- `ErrNoProvider` 表示当前没有可用厂商；`AggregateError` 可通过 `errors.As`/`errors.Is` 检查其中的原始错误。

### internal/markdown（Markdown/代码块渲染包）

```go
func Render(text string) string
func Sanitize(text string) string
```

设计要点：

- 零新增依赖，不依赖外部 Markdown 解析库；输出为带 ANSI 颜色的纯文本，适合 Bubble Tea / Lip Gloss 使用。
- `Render` 支持标题、行内代码、围栏代码块（边框 + 行号）、列表、引用和分隔线。
- `Sanitize` 去除终端控制字符（保留换行与 Tab），`Render` 渲染前会自动调用，防止转义注入。
- 当前仅提供渲染工具，UI 接入后续进行。

## 第一里程碑验收

1. `go run ./cmd/agentforge --mock` 启动后显示聊天界面。
2. 输入文本回车后，界面显示用户消息和一条 mock assistant 流式回复。
3. 会话写入 `~/.agentforge/sessions/`。
4. 重启后可用 `--resume <id>` 恢复历史。
5. `go test ./...` 通过。

## 已扩展能力

- `/context`：Context 用量面板（totalTokens、cacheHits、uncachedInput、output、breakdown）
- `/cost`：成本估算面板（estimatedYuan、currency）；估算与预算逻辑见 `internal/cost`
- `/diff`：Diff Review 面板（真实 API 解析 `tool/result` 中的 `git diff` 输出；`j/k` 移动，`a/r` 接受/拒绝）
- `/todos`：Todo / 任务计划面板（解析 `tool/call` 中的 `todo_write`）
- `/skills`：Skill 管理面板（扫描 `<data-dir>/skills/*.md`，解析会话中的 `<available_skills>`；`j/k` 选择，`Space` 启用/禁用，`Enter` 查看详情）
- `/sessions`：本地/远端会话列表；按数字切换会话
- 项目记忆/长期学习：`internal/memory` 提供 `<data-dir>/memory/<project>.json` 本地持久化与关键词/子串检索（预留 `Embedder` 接口）
- 顶部 Tab 工作区：`Ctrl+←` / `Ctrl+→` 切换最近会话
- `/help`、`/close`：命令帮助与关闭 Channel
- mock server 会随消息增长模拟 context/cost 数据，方便 UI 开发与验证

## 真实 DSH Web API

`internal/dshapi` 实现了 DSH 实际使用的浏览器传输协议：

- unary RPC：`POST /api/<method>`，信封 `client-request` / `server-response`
- 下行事件：`GET /api/events.mux` 与 `/api/events.host`（WebSocket，非 SSE）

已实测：

```bash
dsh --profile web --no-open --port 0
go run ./cmd/agentforge-smoke --url http://127.0.0.1:<port> --prompt "你好"
```

UI 现在支持 `--api-url` 直接连接真实 DSH Web API。
