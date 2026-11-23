# AgentForge 项目缺陷与功能完成度评估

评估日期 2026-11-23，基线为本地 `v0.9.0`（`dab34dd`）。

## 结论

**AgentForge 已具备终端聊天客户端和可执行的 CLI Agent 雏形。当前最需要补齐的是操作边界、会话一致性和任务闭环。** 首先处理预演仍能提交、插件隔离承诺和加密会话迁移静默漏数据。随后补齐改动评审、模型上下文和 TUI 的真实执行入口。

| 使用入口 | 当前可用能力 | 完成度边界 |
| --- | --- | --- |
| TUI 聊天 | DSH Web API、Provider 直连、Mock；流式聊天、会话列表、本地历史 | 分屏已改为只读；Provider 模式下清空/压缩/回退会同步后端上下文；DSH 等不支持的后端会提示 |
| `agent` CLI | 静态/LLM 计划、执行、测试状态、范围化 Diff 和 Git 提交 | 预演与提交边界已修复；执行失败后的再规划与工作树级回滚仍缺 |
| TUI `/agent` | 示例计划及审批状态展示 | 执行和测试按键只变更示例状态（未完成真实 Runner 接入） |
| 多 Agent | 并发静态任务；TUI JSON 支持声明依赖 | CLI 示例只验收独立任务；工作区隔离、冲突处理和中断恢复未完成 |
| MCP 与插件 | 手动列出和调用 MCP 工具、TUI 审批；插件安装、校验和权限检查 | Provider 自动工具回路未接通；插件仍以本机受信进程运行，无 OS 级隔离 |
| 会话迁移与多模态 | 明文/加密导入导出；Provider 图片跨轮与离线附件重试 | 图片不写入会话历史（重启后仅离线重试可恢复）；DSH 路径仅支持文字 |

## 修复状态（2026-11-23 复核）

| 编号 | 状态 | 修复要点 | 验证 |
| --- | --- | --- | --- |
| D01 | 已修复 | `--dry-run` 只生成计划，不审批/执行/测试；与 `--commit`/`--push`/`--pr` 组合直接拒绝；`--push`/`--pr` 必须配 `--commit`。 | 临时仓库实测：冲突参数退出码 1，HEAD、分支、工作树均未变化；预演 JSON 为 `phase=Preview`，动作保持 `pending`。 |
| D02 | 已修复 | Diff 覆盖新建/修改/删除/已暂存/未跟踪；按运行前快照区分任务改动与预先存在的脏改动；`--commit` 只提交任务路径，冲突时拒绝自动提交。 | 临时仓库实测：提交仅含 `task.txt`，预先暂存/未跟踪改动保留；修改已脏文件时退出码 1 并拒绝提交。 |
| D03 | 已修复 | CLI 安装提示与文档明确插件以本机进程、当前用户权限运行，无 OS 级文件系统/网络隔离；网络权限插件继续拒绝。 | 源码与文档核对；`market install` 新增信任边界提示。 |
| D04 | 已修复 | 导出/导入同时处理 `.json` 与 `.json.enc`，同 ID 双格式优先加密文件，分别报告明文/加密、跳过与失败数量。 | 新增混合目录、同 ID 双格式、加密导入回读测试并通过。 |
| D05 | 已修复 | `/clear`、`/compact`、回退在 Provider 直连模式下调用 `SyncHistory` 替换后端上下文并落盘；不支持的后端提示 `local view only`。 | Provider 测试确认下一次请求只包含同步后的历史；UI 测试确认命令会下发同步。 |
| D06 | 已修复 | `/split` 改为只读并排查看，普通输入默认只发往当前会话；按 `s` 切换后才向右侧会话发送。 | 移除双发分支；界面与文档标注只读和切换方式。 |
| D07 | 已修复 | 多任务预演不再被审批拦截；执行失败标记 `failed`；跳过测试输出 `test_status=skipped` 且 `test_ok=false`；失败结果也输出 JSON。 | CLI 实测：`multiagent --dry-run` 无 `--yes` 通过；`agent --test '' --json` 为 skipped；失败动作 JSON 为 `failed`。 |
| D08 | 已修复 | 图片绑定到对应 user 消息并在同会话后续追问中继续发送；离线队列持久化附件并在重启重试时恢复；DSH 后端提示不支持图片。 | Provider 测试确认第二轮回传携带原图；队列测试确认图片跨实例持久化。 |

## 一、项目缺陷

> 以下为修复前基线记录；D01-D08 的修复状态与验证见上一节。

P0 表示涉及写操作越界或关键安全承诺，应在扩大使用前处理。P1 表示常用工作流的正确性或数据可靠性问题。P2 表示场景限制或低频行为。实测使用临时目录和合成数据；源码确认的条目给出调用路径。

| 编号 | 优先级 | 证据 | 问题、影响与验收条件 |
| --- | --- | --- | --- |
| D01 | P0 | CLI 实测 | **预演仍可提交。** 临时仓库中，`agent --dry-run --commit` 跳过了计划编辑，但改变了 HEAD，并把预先存在的无关文件提交。Runner 返回后，CLI 仍进入 Git 阶段。验收时，带有提交、推送或 PR 选项的预演应拒绝冲突参数或始终只输出预览，分支、工作树、索引和 HEAD 均保持原状态。[参数与提交阶段](../cmd/agentforge/agent.go#L190) |
| D02 | P1 | CLI 实测与源码 | **评审和提交的改动范围不一致。** 新建文件后工作树出现未跟踪项，CLI 的 Diff 仍为空。当前 Diff 使用普通 `git diff`，也遗漏仅暂存的改动，提交则使用 `git add -A`。CLI 打印 Diff 后可直接进入提交流程。验收时，新建、修改、删除均可见，预先存在的改动与本任务改动分开，实际提交和获批范围一致。[Diff](../internal/agentflow/agentflow.go#L498) · [提交范围](../internal/gitflow/gitflow.go#L106) · [CLI 流程](../cmd/agentforge/agent.go#L183) |
| D03 | P0 | 源码确认 | **插件缺少文档所暗示的运行隔离。** 安装校验、权限检查和超时已有实现。运行路径设置工作目录后启动本机进程，拒绝声明网络权限的插件，但没有施加网络或文件系统隔离。安装校验无法提供运行期隔离。验收时，CLI 和文档明确插件的本机执行信任边界；如要允许隔离执行，则由受支持的平台实际执行并验证权限约束。[插件入口](../internal/plugin/plugin.go#L354) · [运行约束](../internal/plugin/sandbox.go#L27) · [现有文档](usage.md#L258) |
| D04 | P1 | CLI 路径实测 | **加密会话导出静默漏数据。** 通过 `EncryptedStore` 创建并确认可读的一条会话，`export` 返回成功并报告导出 0 条。导出只枚举 `.json`，导入也只检查明文同名文件。验收覆盖明文、加密、混合目录和同 ID 两种格式，并明确失败和跳过数量。现有整目录 `backup` / `restore` 是另一条路径，本轮未验收其加密恢复。[导出入口](../cmd/agentforge/export_import.go#L36) · [导入查重](../internal/session/sync.go#L110) · [导出枚举](../internal/session/sync.go#L239) |
| D05 | P1 | TUI 命令与模拟 Provider 联测 | **`/clear` 和 `/compact` 没有改变下一次模型请求。** 三条合成历史在 UI 中分别变为 0 条和 2 条，两种情况下下一次 Provider 请求都携带原三条历史及新消息。命名回退也在 TUI 本地改写，后端语义需单独核验。验收时捕获实际发给模型的消息，确认清空、压缩、回退和重启恢复符合命令语义；对不支持的后端给出明确提示。[清空](../internal/ui/model_commands.go#L565) · [压缩](../internal/ui/model_compact.go#L9) · [回退](../internal/ui/model_timeline.go#L37) · [Provider 请求](../internal/provider/backend.go#L378) |
| D06 | P1 | 源码确认 | **启用双会话分屏后，普通输入同时发往主会话和副会话。** 使用文档将它描述为并排视图。对已选定副会话的用户，这会改变消息投递范围，也可能触发两份生成用量。验收时，默认发送目标在界面中可见；如果保留双发，需显式选择并显示目标，且两个会话的响应可独立验收。DSH 当前 Backend 仅维护一个活动会话和事件流，本轮未联测双路回显。[发送分支](../internal/ui/model.go#L1148) · [DSH 会话状态](../internal/dshapi/backend.go#L18) · [分屏说明](usage.md#L582) |
| D07 | P1 | CLI 实测与源码 | **多任务预演缺省退出失败，跳过验证也会报告测试通过。** `multiagent --dry-run` 在未加 `--yes` 时被拒绝，因为审批分支在预演分支之前，CLI 也未提供审批回调。另一次 `agent --test '' --json` 实测返回 `test_ok: true`。执行失败目前也会先将动作标为 executed。验收时，预演只读取计划，并在 CLI 与事件里区分计划、执行失败、未运行、跳过和通过。[多任务入口](../cmd/agentforge/multiagent.go#L50) · [审批与预演](../internal/agentflow/agentflow.go#L187) · [执行失败状态](../internal/agentflow/agentflow.go#L235) · [跳过测试分支](../internal/agentflow/agentflow.go#L282) |
| D08 | P2 | 模拟 Provider 实测与源码 | **图片只随首轮请求发送。** 模拟 Provider 首轮收到一张图，第二轮追问只收到以前的文字。当前离线重放结构只有消息文本；失败入队时也只写入文字。验收时明确同图追问和附件重试的产品语义，若只支持单轮则在界面提示并避免将无图重发显示为完整重试。[图片生命周期](../internal/provider/backend.go#L387) · [失败排队](../internal/provider/backend.go#L415) · [队列结构](../internal/provider/offline.go#L18) |

## 二、项目缺失什么

| 优先级 | 缺口 | 建议交付与验收 |
| --- | --- | --- |
| P0 | **工作区变更归属与发布授权** | 为 Agent 写操作建立任务级基线和变更清单，分开计划、编辑、评审、提交和远端发布。用已有暂存、未暂存及未跟踪合成文件的仓库验证，失败或取消后能识别本任务写入，也能保全原有改动。覆盖可选的任务隔离工作区。 |
| P0 | **本机执行信任模型** | 明确 TUI、CLI、插件和执行器分别在哪里运行、使用哪类权限、何时批准。把插件签名及完整性校验与运行隔离分别验收。CLI Agent 目前默认使用本地执行器，命令检查应按这一边界描述。[默认执行器](../internal/agentflow/agentflow.go#L101) · [命令执行路径](../internal/agentflow/agentflow.go#L316) |
| P1 | **会话与任务的恢复契约** | 明确 UI、Provider 和 DSH 各自持有的状态。补齐落盘错误提示、任务重启恢复、重试及取消语义。Provider 写会话时丢弃存储错误，UI 结束阶段也不向调用者返回写入失败。会话加密配置只覆盖会话存储，Provider 离线消息仍进入独立的明文 JSON 队列，应在数据目录说明中明确这一范围。用写失败、进程中断、恢复及重发覆盖这条链路。[Provider 持久化](../internal/provider/backend.go#L648) · [退出流程](../cmd/agentforge/main.go#L169) · [离线队列](../cmd/agentforge/dial.go#L38) |
| P1 | **可验证的仓库感知** | 将项目规则、有界文件读取与修改校验接入 Agent 主流程。当前 Repo Mode 扫描已包含多个字段，实际送入规划的仓库摘要主要是路径、名称、测试目录和近期变更。应使用带项目规则与可验证需求的小仓库，检查计划是否遵循约束。[仓库扫描](../internal/repomode/repomode.go#L61) · [规划上下文](../cmd/agentforge/agent.go#L23) |
| P1 | **按入口和后端划分的验收矩阵** | 现有三平台 CI、Race、覆盖率、Vet 与 Linux CLI 冒烟可作为基线。补上 DSH 模拟契约、双会话、Provider 上下文和附件、加密导出恢复、Git 脏工作区、失败重试，以及发布包安装和更新路径。真实 DSH、Provider 和 MCP 可作为独立验收。多 Agent 脚本的注释称覆盖依赖任务，实际只提交两个独立任务。[CI](../.github/workflows/ci.yml#L9) · [多任务验收脚本](../scripts/acceptance-multiagent.sh#L3) |
| P2 | **版本化能力清单** | README、使用文档和 Changelog 应标注稳定、部分接入、示例、实验和历史状态，并覆盖每个后端。当前 README 仍引用现树没有的 `internal/persist` 与 `internal/orchestrator`，项目中对应代码分别位于 `internal/session` 和 `internal/multiagent`；Changelog 缺少与 `v0.9.0` 对应的独立章节。[README](../README.md#L63) · [Changelog](../CHANGELOG.md#L7) |

项目当前以 **Go 单二进制、TUI / CLI 和 DSH Web API** 为主。历史 Changelog 中出现过 REST 服务、企业治理、MCP Server、团队协作和定时调度；本轮在当前目录与入口中未见这些能力。它们应从当前能力清单中剔除或明确标为历史状态，是否恢复取决于产品目标。[历史条目](../CHANGELOG.md#L114)

## 三、未完成或仅部分接入的功能

状态标记：`未完成` 表示入口尚未接通；`部分接入` 表示已有可用路径但存在明确缺口；`实验` 表示非稳定协议。

| 功能 | 状态 | 已实现 | 仍待完成 |
| --- | --- | --- | --- |
| TUI Agent 审批与执行 | 未完成 | 审批状态机和面板；独立 CLI 有 Runner | `/agent` 创建固定示例，执行和测试键只标记状态。接入真实计划、执行、测试、评审、取消及任务结果。[TUI 示例](../internal/ui/model_agent.go#L9) · [CLI Runner](../cmd/agentforge/agent.go#L132) |
| CLI LLM Agent | 部分接入 | 模型生成 JSON 动作计划；Runner 顺序执行、测试状态与范围化 Diff 已接通 | 执行结果驱动的再规划、失败恢复和工作树级回滚尚缺。执行阶段失败直接返回。[LLM 规划](../internal/agentflow/llmplanner.go#L75) · [执行失败路径](../internal/agentflow/agentflow.go#L235) |
| 多 Agent 任务编排 | 部分接入（工作区隔离未完成） | Runner 按依赖并发调度；TUI 支持带依赖的 JSON；预演不再需要审批 | CLI 只接收静态任务并共享 `--workdir`；TUI 两种输入均用静态规划器。缺少工作区冲突处理和持久化重试；CLI 依赖输入及 TUI 运行中取消也未接通。[调度](../internal/multiagent/multiagent.go#L83) · [CLI](../cmd/agentforge/multiagent.go#L40) · [TUI](../internal/ui/model_multi.go#L41) |
| Provider 直连 MCP 工具回路 | 未完成 | OpenAI 兼容层解析流式工具调用，MCP 有手动操作与审批 | 聊天 Backend 没有消费工具调用事件，也未将工具结果带回下一轮。DSH 事件投影与此路径分开验收。[Provider 事件](../internal/provider/openai.go#L309) · [聊天循环](../internal/provider/backend.go#L428) · [手动调用](../internal/ui/model_mcp.go#L75) |
| 项目说明、记忆与长上下文 | 部分接入 | `project-init` 生成 `AGENT.md`；记忆可写入、删除和按关键词查询；TUI 有摘要、检查点与回退；`/clear`、`/compact`、回退会同步 Provider 请求历史 | 规划入口没有加载生成的项目说明；向量检索接口仍预留，记忆查询结果只显示在 TUI。TUI 检查点使用内存存储，底层文件存储未接入。[项目说明生成](../cmd/agentforge/project_init.go#L14) · [关键词检索](../internal/memory/search.go#L8) · [TUI 记忆](../internal/ui/model_memory.go#L60) · [检查点存储](../internal/contextx/checkpoint.go#L29) |
| Provider 故障转移与会话同步 | 部分接入 | Provider 有路由包与独立故障转移组件；会话包有远端同步函数和本地异步存储 | 常规启动只初始化一个 Provider；未找到可配置的多 Provider 故障转移入口或远端同步 CLI / TUI 入口。DSH 远端会话列表是独立功能。[Provider 启动](../cmd/agentforge/dial.go#L136) · [故障转移组件](../internal/provider/failover.go#L15) · [远端同步](../internal/session/cloud.go#L14) |
| 图片场景 | 部分接入 | `/image` 支持多种图片扩展名；图片随对应 user 消息跨轮发送；离线队列保存附件；DSH 后端给出不支持提示 | TUI 当前按空格切出路径，含空格文件名不能直接输入；图片不写入会话历史，重启后仅离线重试可恢复。[TUI 解析](../internal/ui/model_commands.go#L754) · [DSH 发送](../internal/dshapi/backend.go#L161) |
| DSH Channel 协议 | 实验 | 有客户端与 Mock 测试 | 文档已明确为实验性接口。真实后端验收以 DSH Web API 为先。[设计约定](../README.md#L57) |

## 建议修复顺序

1. **先处理边界。** 修 D01 和 D03，给预演、提交和插件运行建立可回归的行为约定。
2. **再处理数据和改动。** 优先修 D04、D02、D05 和 D06；用本地 Git、加密会话以及可捕获请求的 Provider mock 锁住新旧数据路径。
3. **完成用户入口。** 先收敛 D07 的状态语义，再让 TUI 使用真实 Runner，并为工作区隔离、运行中取消、失败恢复建立可执行验收。多模态、自动 MCP 回路与高级记忆依照使用优先级接入。

## 修复后复核（2026-11-23）

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 25 个包全部通过，1 个包无测试文件。 |
| `go vet ./...`、`go build -o /tmp/agentforge-fix-bin ./cmd/agentforge` | 退出码均为 0。 |
| `go test -race ./internal/agentflow ./internal/gitflow ./internal/provider ./internal/session ./internal/ui` | 5 个包通过。 |
| `make acceptance` | 多任务静态执行与本地 Git 提交流程通过；真实 LLM、MCP、远端推送与 PR 按脚本条件跳过。 |
| 定向 CLI 验收（临时仓库） | D01：`--dry-run --commit` 退出码 1，HEAD/分支/工作树不变，预演 JSON 为 `Preview`。D02：只提交 `task.txt`，预先暂存与未跟踪改动保留；任务修改已脏文件时拒绝提交。D07：`multiagent --dry-run` 无需 `--yes` 通过；`--test ''` 为 `skipped`；失败动作 JSON 为 `failed`。 |
| 定向 Go 测试 | D04 覆盖混合明文/加密目录、同 ID 双格式与加密回读；D05 确认同步后下一次请求使用新历史；D08 确认跨轮携带原图且队列图片跨实例持久化。 |

## 修复前基线验证

| 检查 | 结果 |
| --- | --- |
| `go test ./...` | 25 个包通过，1 个包无测试文件。首次在默认沙箱中被本机回环端口绑定限制，允许本地测试服务监听后，用隔离 HOME 和离线依赖缓存通过。 |
| `go vet ./...` 与 `go build -o /tmp/agentforge-audit-bin ./cmd/agentforge` | 退出码均为 0，本机二进制报告 `agentforge 0.9.0`。构建过程中有 Go 模块元数据缓存写入权限提示。 |
| `go test -race ./internal/agentflow ./internal/contextx ./internal/gitflow ./internal/multiagent ./internal/ui` | 5 个包通过。 |
| `make acceptance` | 本地 Git 提交及两个并行静态任务通过。真实 LLM、MCP、远端 Git 推送与 PR 跳过。 |
| 临时 CLI 与 Go overlay 定向验证 | 复现预演提交、Diff 漏新文件、多任务预演被拒绝、跳过测试报告成功、加密会话漏导出、会话上下文未同步和跨轮图片缺失。定向断言失败即为对应缺陷证据，测试文件放在临时目录。 |

本轮通过 Go 测试、CLI 和模拟 Provider 评估当前本地源码。真实 DSH、外部模型、插件运行隔离、跨平台安装和发布产物、远端推送及性能上限未实测。优先级依据个人开发工作流及数据影响排序。仓库交付为本报告，工作区现有 `AGENTS.md` 保留。
