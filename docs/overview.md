# agentforge 总览

> 语言：中文（简体）。英文用户请先阅读 [English README](../README.en.md)。

## 界面布局（文字版示意）

```text
┌──────────────────────────┬──────────────────────────────────────────────┐
│ Sessions                 │ [session-xxx] [session-yyy]                   │
│ ▶ session-xxx            │──────────────────────────────────────────────│
│   session-yyy            │ ◆ user                                       │
│                          │   你好                                       │
│                          │ ◆ assistant                                  │
│                          │   这是流式回复...                             │
│                          │──────────────────────────────────────────────│
│                          │ > 输入消息，Enter 发送，Ctrl+C 退出            │
│                          │ connecting... | session: ... | /help         │
└──────────────────────────┴──────────────────────────────────────────────┘
```

## 已实现功能一览

| 区域/功能 | 说明 |
|---|---|
| 主聊天区 | 用户/助手消息流式显示 |
| 左侧 Sessions | 多会话 Tab / 当前会话标记 |
| 右侧 Overview | 1 Context / 2 Cost / 3 Diff / 4 Todo |
| Diff Review | 接受/拒绝、patch、逐文件 `git apply`、备份、回滚 |
| 双会话并排 | `/split`，`[` / `]` 切换，Enter 同时发送 |
| 底部命令 | `/context` `/cost` `/diff` `/todos` `/sessions` `/right` `/split` `/help` |

## 快速体验

```bash
# mock 模式
go run ./cmd/agentforge --mock

# 真实 DSH
dsh --profile web --no-open --port 0
go run ./cmd/agentforge --api-url http://127.0.0.1:<port>
```

