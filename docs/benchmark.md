# 性能基准与优化

本文记录 agentforge 在超长会话场景下的基准测试与优化结果。

## 环境

- 系统：macOS（Apple M1 Pro）
- Go：go1.24.2 darwin/arm64
- 命令：

```bash
go test ./internal/session ./internal/search ./internal/provider -run '^$' -bench . -benchmem -count=3
```

## 基准场景

- `session`：单会话 1000 条消息的 `Save` / `Load`
- `search`：索引单会话 1000 条消息，以及 `Search` 命中/未命中
- `provider`：用 `strings.Reader` 模拟 1000 个 SSE `data:` 块解析

## Before / After 对比

数值取 3 次运行中的代表值（ns/op 取中位附近）。

### internal/session

| Benchmark | Before | After | 变化 |
|---|---|---|---|
| Save 1000 条消息 | ~6.09 ms/op，~639 KB，~34 allocs | ~1.03 ms/op，~626 KB，~19 allocs | 耗时约 -83%，allocs 约 -44% |
| Load 1000 条消息 | ~1.21 ms/op，~486 KB，~2044 allocs | ~1.22 ms/op，~486 KB，~2044 allocs | 基本不变 |

### internal/search

| Benchmark | Before | After | 变化 |
|---|---|---|---|
| Index 1000 条消息 | ~3.37 µs/op，~33 KB，4 allocs | ~3.47 µs/op，~33 KB，4 allocs | 基本不变 |
| Search 命中（limit=50） | ~592 µs/op，~1.3 KB，5 allocs | ~25.1 µs/op，~1.3 KB，5 allocs | 耗时约 -96% |
| Search 未命中 | ~566 µs/op，0 B，0 allocs | ~8.15 µs/op，0 B，0 allocs | 耗时约 -99% |

### internal/provider

| Benchmark | Before | After | 变化 |
|---|---|---|---|
| SSE 解析 1000 chunks | ~959 µs/op，~877 KB，13005 allocs | ~928 µs/op，~749 KB，11005 allocs | 耗时约 -3%，内存约 -15%，allocs 约 -15% |

## 优化内容

### session.Save：重复写入内存缓存

`internal/session/session.go`

- `Store` 增加内存缓存，记录每个会话最近一次成功写入的 JSON 字节。
- 再次 `Save` 相同内容且文件仍存在时，直接返回，跳过 `CreateTemp` + `Write` + `Sync` + `Rename` 全量写盘。
- 文件被外部删除时仍会重新写入，保持原有“保存后文件一定存在”的语义。
- `Delete` 同步清理缓存。
- 增加 `sync.Mutex` 保护缓存与写盘过程，未改变公开 API。

### persist.Writer + session.AsyncStore：异步落盘

`internal/persist/async.go`、`internal/session/async_store.go`

- 新增通用 `persist.Writer`：`Submit` 将写盘任务加入无界队列并立即返回，后台 worker 串行执行，避免 UI 被同步 `fsync`/`rename` 阻塞。
- 新增 `session.AsyncStore`：包装 `Store`，`Save` 异步提交，`Load`/`Delete` 保持同步；`Delete` 会先等待已提交 `Save` 完成，保证删除语义不被异步队列打乱。
- `Flush(ctx)` 提供显式等待点，退出前可调用 `Close()` 自动 flush 未完成任务。
- 后台写盘错误通过 `Errors()` 通道收集，不会阻塞 worker。
- UI 尚未切换为 `AsyncStore`；后续接入时只需替换 store 实例或增加可选适配，不需要改动 `internal/ui` 核心逻辑。

### search.Index：惰性小写缓存与增量 AddAsync

`internal/search/search.go`

- `Add/AddAsync` 仍保持轻量，只更新对应会话并失效该会话的小写缓存，不会重建整个索引。
- `Search` 首次访问某会话时构建小写内容缓存，后续搜索直接复用，避免每次对全部消息反复 `strings.ToLower`。
- 对同一 `Index` 的多次搜索（例如在 UI 中复用搜索索引）收益显著；一次性索引+搜索的总成本基本不变。
- 保持 `Add`/`AddAsync` 可增量更新、同一会话替换旧消息的既有语义。

### provider SSE 解析：减少字符串转换

`internal/provider/openai.go`

- 使用 `scanner.Bytes()` 替代 `scanner.Text()`，避免每行产生 string。
- 使用 `bytes.HasPrefix` / `bytes.TrimPrefix` / `bytes.TrimSpace` / `bytes.Equal` 处理 `data:` 前缀与 `[DONE]`。
- `json.Unmarshal` 直接消费 `[]byte`，减少一次 `[]byte(data)` 转换。

## UI 性能建议（未改 UI 核心）

- `/find` 目前每次命令都会 `store.List()` 后新建 `search.Index`。如果用户在同一个大会话列表上连续搜索，可在 Model 中缓存最近一次的 `search.Index` 并复用，配合本次惰性小写缓存可获得更好效果。
- 会话落盘已有 `persistInterval`（500ms）节流；本次 `session.Save` 缓存可进一步避免内容未变化时的重复全量写盘。
- 大会话场景可进一步将 `*session.Store` 替换为 `*session.AsyncStore`（`Save` 异步、`Flush`/`Close` 显式落盘），在不改动 `internal/ui` 核心逻辑的前提下把同步写盘移出 UI 事件循环。

## 验证结果

```bash
go test ./...   # 全部通过
go vet ./...    # 通过
go build ./...  # 通过
```
