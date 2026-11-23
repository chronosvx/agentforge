# AgentForge 桌面 GUI 方案

> 状态：方案评审稿（v0.1）
> 目标：先做 macOS 桌面 GUI，未来平滑覆盖 Windows / Linux / 手机端。

## 1. 背景

AgentForge 目前是 Go + Bubble Tea 的终端 TUI/CLI 客户端。核心能力（Agent 主流程、Repo Mode、MCP、记忆、时间线、多 Agent）已经沉淀在 Go 侧。

现在需要一个桌面 GUI，第一版先适配 macOS，但技术栈必须为以下场景留路：

- macOS 桌面
- Windows 桌面
- Linux 桌面
- 手机端（iOS / Android）

## 2. 选型目标

| 维度 | 要求 |
|---|---|
| 跨平台 | 同一套 UI 代码尽量覆盖桌面 + 移动端 |
| 复用现有资产 | 最大化复用 Go 核心，不重写 Agent / MCP / 记忆 / 安全逻辑 |
| 开发效率 | 不要引入过重的前端工程（但仍接受现代 UI 框架） |
| 性能与体积 | 桌面端启动快、内存可控，移动端可裁剪 |
| 长期维护 | 个人项目可维护，避免多套客户端同步成本 |

## 3. 候选方案对比

| 方案 | macOS 体验 | Windows/Linux | 手机端 | 复用 Go 核心 | 体积/性能 | 结论 |
|---|---|---|---|---|---|---|
| 原生 SwiftUI | 最佳 | 需另做 | 需另做 | 通过进程调用 | 小/好 | 未来多端成本高，不推荐作为主栈 |
| Wails v2 (Go + WebView) | 好 | 好 | 不支持 | 直接调用 Go | 中/中 | 桌面够用，但移动端无解 |
| Tauri 2 (Rust + Web) | 好 | 好 | 支持但较新 | 通过本地服务/进程 | 小/好 | 可行，但引入 Rust，且移动端生态未完全成熟 |
| Fyne (纯 Go) | 一般 | 一般 | 实验性 | 直接调用 Go | 大/中 | UI 表现力有限，移动端不现实 |
| Flutter (Dart) | 好 | 好 | 好（iOS/Android 同一套） | 通过本地 HTTP/WebSocket 调用 Go | 中/好 | **最符合“桌面 + 手机”的统一路线** |
| Compose Multiplatform (Kotlin) | 好 | 好 | 好（iOS/Android 同一套） | 通过本地服务/FFI | 中/好 | 也可行；若团队更熟 Kotlin/JVM 可考虑 |

### 3.1 为什么推荐 Flutter

1. **一套代码覆盖五端**：macOS / Windows / Linux / iOS / Android 都是官方支持目标，正好匹配“先 macOS，未来 Windows/Linux/手机”的需求。
2. **UI 表达力强**：适合做 Agent 运行面板、流式日志、时间线、记忆列表这类动态界面；也方便后续做移动端触控适配。
3. **生态成熟**：Flutter 桌面已 stable，移动端是主场；国内开发者和社区资料也多。
4. **不绑定 Apple 生态**：不会像 SwiftUI 那样把未来 Windows/Linux/手机锁死。
5. **Go 核心继续发光**：AgentForge 最有价值的是 Go 侧的业务逻辑，GUI 只做“壳 + 交互”，Go 通过本地服务暴露能力，避免用 Dart 重写业务。

### 3.2 为什么不选 Wails / 纯 Go GUI

- Wails 只解决桌面，手机端要另起炉灶，违背“一套 UI 尽量覆盖”的目标。
- Fyne 的桌面原生感、复杂列表/图表/流式渲染能力和移动端支持都不如 Flutter。

### 3.3 为什么不首选 Tauri

- Tauri 2 已支持 iOS/Android，但移动端仍偏新；且引入 Rust 工具链，和现有 Go 生态叠加后维护成本更高。
- 如果未来确定只做桌面，Tauri 是很好的备选；但当前需求明确包含手机端，Flutter 更稳。

## 4. 推荐架构：Flutter GUI + Go 本地服务

```text
┌────────────────────────────┐        ┌──────────────────────────────┐
│  Flutter App (macOS 首版)  │        │  Go Core / agentforge 核心    │
│                            │  HTTP  │                              │
│  - Agent 运行面板          │ ◄────► │  - agentflow / agent         │
│  - 记忆/时间线浏览          │  JSON  │  - multiagent / mcp          │
│  - 连接/状态/设置           │  SSE   │  - memory / timeline         │
│                            │  WS    │  - session / security        │
└────────────────────────────┘        └──────────────────────────────┘
         UI 层（Dart）                  Daemon 层（Go，本地 loopback）
```

### 4.1 为什么用“本地服务”而不是“直接把 Go 编译进 App”

- **避免双份业务实现**：GUI 不需要理解 Agent 状态机、MCP、加密会话等细节，只消费 API。
- **进程隔离更安全**：Go daemon 继续跑在用户环境里，访问 Git/Shell/Keychain；GUI 崩溃不影响任务执行。
- **移动端天然可行**：以后手机 App 可以连接同一台电脑上的 daemon（局域网/隧道），或后续再演进为嵌入式模式。
- **CLI/TUI 继续共存**：`agentforge` 仍是终端主力；GUI 只是另一个前端。

### 4.2 本地服务形态

在现有 `agentforge` 中新增一个子命令：

```bash
agentforge serve
```

- 只监听 `127.0.0.1`，默认端口例如 `127.0.0.1:48321`（可配置）。
- 启动时生成/读取一次性访问令牌，存到 Keychain/系统钥匙串；GUI 连接时需携带令牌。
- 提供：
  - `GET /api/health`：版本、配置状态、daemon 是否就绪
  - `POST /api/agent/run`：发起 Agent 任务（复用现有 `runAgent` 的输入参数）
  - `GET /api/agent/events`：SSE/WebSocket 流式事件（plan / exec / diff / test / done / error）
  - `GET /api/sessions`、`GET /api/timeline`、`GET /api/memory`
  - 后续按需扩展：MCP 权限确认、Diff 审批、多 Agent 面板等

### 4.3 安全设计

- 只绑定 loopback，不开放局域网（除非未来用户显式开启）。
- 每次启动生成随机 token；令牌可通过 `agentforge serve --print-token` 或 GUI 引导配对写入 Keychain。
- 所有写操作（执行命令、改文件、提交/推送）沿用现有审计、权限、确认机制。
- GUI 本身不保存明文密钥；敏感配置仍由 Go daemon 通过 Keychain 管理。

## 5. 建议项目结构

```text
agentforge/
├── cmd/agentforge/
│   ├── main.go
│   ├── serve.go            # 新增：本地 daemon
│   └── ...
├── internal/
│   ├── server/             # 新增：HTTP/SSE/WS API + 鉴权 + 请求适配
│   ├── agent/              # 已有：Agent 状态机
│   ├── agentflow/          # 已有：执行流
│   ├── memory/             # 已有：记忆
│   ├── timeline/           # 已有：时间线
│   └── ...
├── gui/                    # 新增：Flutter 工程
│   ├── macos/              # Flutter macOS runner
│   ├── windows/            # 后续
│   ├── linux/              # 后续
│   ├── lib/
│   │   ├── main.dart
│   │   ├── app.dart
│   │   ├── api/            # Go daemon 客户端
│   │   ├── models/
│   │   ├── screens/
│   │   │   ├── agent_run.dart
│   │   │   ├── memory_timeline.dart
│   │   │   └── settings.dart
│   │   └── widgets/
│   └── pubspec.yaml
└── docs/
    └── desktop-gui.md      # 本文档
```

## 6. 第一版（MVP）功能范围

根据你的选择，第一版只做两块：

1. **对话 / Agent 运行面板**
   - 输入任务，选择 Repo、Provider、Planner（static/llm）、Agent 类型
   - 发起运行后实时展示：计划、执行日志、Diff、测试结果、完成/失败状态
   - 支持取消/停止、查看最近运行记录

2. **记忆 / 时间线浏览**
   - 查看项目记忆条目（`/memory`）
   - 查看会话检查点时间线（`/timeline`）
   - 从时间线恢复/查看详情

其余（设置、MCP 权限、多 Agent 编排、Diff 审批）作为 v0.6+。

## 7. 里程碑

| 阶段 | 内容 | 交付物 |
|---|---|---|
| M0 | 方案评审通过 | 本文档 |
| M1 | Go daemon API | `agentforge serve` + `internal/server` + 单元测试 |
| M2 | Flutter macOS 骨架 | 可启动的 macOS App，能连接 daemon、显示健康状态 |
| M3 | Agent 运行面板 | 发起任务 + SSE 流式事件展示 |
| M4 | 记忆/时间线 | 两个浏览页面 + 详情/恢复 |
| M5 | 打包与发布 | macOS `.app` / `.dmg`，接入现有 Release 流程 |
| M6 | 跨平台扩展 | Windows / Linux 桌面；再评估手机端连接模式 |

## 8. 风险与开放问题

- **Flutter SDK 尚未安装**：开始 M2 前需要先安装 Flutter SDK（`brew install --cask flutter` 或官方渠道）。
- **macOS 打包/签名/公证**：个人项目可先做未签名 `.app`/`.dmg`；要公开分发需 Developer ID + notarization。
- **手机端连接模式**：手机端大概率不是“手机本地跑 Agent”，而是“连接电脑上的 daemon”或“手机跑轻量客户端 + 云端/家中电脑执行”，M6 再细化。
- **是否保留 TUI**：GUI 是新增前端，不是替代 TUI；TUI 继续作为终端场景主力。
- **API 稳定性**：`internal/server` 需要先定义好版本化 API，避免后续多端前端被破坏。

## 9. 下一步（经确认后执行）

1. 批准本方案。
2. M1：先实现 `agentforge serve` 本地 daemon 和最小 API。
3. M2：安装 Flutter，创建 `gui/` 工程并跑通 macOS 连接。
4. M3/M4：按 MVP 范围实现 Agent 运行面板 + 记忆/时间线。
