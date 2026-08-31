# 截图 / 动图

当前目录用于存放 agentforge 的截图与演示动图。

> 说明：本环境无法直接生成真实终端截图/录屏，以下为捕获方案。

## 推荐工具

- macOS：`screencapture`、CleanShot、Kap
- 终端录制动图：`asciinema`、`vhs`（Charm 出品）
- TUI 输出捕获：`script`

## 捕获步骤

1. 启动 mock 模式：

```bash
go run ./cmd/agentforge --mock
```

2. 演示建议流程：

   - 发送一条消息，展示流式回复
   - `/context` 打开 Context 面板
   - `/cost` 打开成本面板
   - `/diff` 打开 Diff Review
   - `/todos` 打开 Todo
   - `/sessions` 打开会话列表
   - `/right` 打开右侧 Overview
   - `/split` 打开双会话并排

3. 保存文件到本目录：

```text
docs/screenshots/
├── mock-chat.png
├── context-panel.png
├── cost-panel.png
├── diff-review.png
├── todo-panel.png
├── sessions-panel.png
├── right-panel.png
├── split-view.png
└── demo.gif
```

4. 在 `docs/overview.md` 或 README 中引用。

## `script` 捕获终端文本（可选）

```bash
script docs/screenshots/demo.typescript
go run ./cmd/agentforge --mock
# 操作后输入 exit
```

该文件不是图片，但可作为原始演示素材。
