# agentforge 零基础开箱即用指南

[中文](QUICKSTART.md) | [English](QUICKSTART.en.md)

> 这篇文章是写给“完全不懂编程”的朋友看的。  
> 你不需要会写代码，只需要会：**打开文件夹、双击文件、复制粘贴命令**。  
> 请按顺序从第 1 步开始，遇到不认识的名词可以看文末的“小词典”。

---

## 这是什么？能做什么？

`agentforge` 是一个**在电脑终端里运行的 AI 聊天工作台**。

- 它可以连接 **DeepSeek Harness（简称 DSH）**，用文字界面和 AI 对话。
- 它也可以**直接使用模型厂商的 API Key**（例如 DeepSeek 的 Key），不装 DSH 也能用。
- 它还带有“体验模式”（Mock 模式），**没有 Key 也能先打开看看界面**。

这里的“终端”就是电脑里那个黑色的、可以输入命令的窗口：

- Windows 上叫 **PowerShell** 或 **命令提示符**
- macOS 上叫 **终端（Terminal）**
- Linux 上一般也叫 **终端**

这是一个**个人项目**，主要供作者学习和日常使用。

---

## 需要准备什么？

| 需要 | 说明 |
|---|---|
| 一台电脑 | Windows 10/11、macOS 或 Linux 都可以 |
| 能上网 | 下载安装包、连接 AI 服务都需要网络 |
| 一个终端窗口 | 见上面说明，Windows 用 PowerShell，macOS/Linux 用终端 |
| 可选：DSH | 如果你已经装了 DeepSeek Harness，可以连它 |
| 可选：厂商 API Key | 例如 DeepSeek 的 `sk-...` 密钥；没有的话可以先用体验模式 |

> **API Key 是什么？**  
> 你可以把它理解成“AI 服务的密码/钥匙”。  
> 有了它，程序才能替你去调用 DeepSeek 等厂商的 AI 服务。  
> 它非常私密，**不要发给任何人**。

---

## 第一步：下载 / 安装

程序发布在 GitHub 的 **Releases（发布页）**。  
请打开项目的 GitHub 页面，找到 **Releases**，下载适合你电脑的安装包。

> 如果你不知道选哪个：  
> - Windows 10/11 电脑通常选 **`amd64`**。  
> - 2020 年后买的 Mac（Apple 芯片）通常选 **`arm64`**；更早的 Intel Mac 选 **`amd64`**。  
> - Linux 电脑通常选 **`amd64`**；树莓派等 ARM 设备选 **`arm64`**。

### Windows

1. 下载 **`agentforge-windows-amd64.zip`**（如果是 ARM 版电脑，下载 `...-arm64.zip`）。
2. 右键压缩包 → **全部解压缩**。
3. 解压后会得到一个文件夹，里面有一个 **`agentforge.exe`** 文件。
4. 记住这个文件夹的位置，下一步会用到。

### macOS

1. 下载 **`agentforge-darwin-amd64.dmg`** 或 **`agentforge-darwin-arm64.dmg`**。
   - 如果发布页只有 `.zip`，下载对应的 `.zip` 并双击解压。
2. 双击打开 `.dmg` 文件。
3. 把里面的 **`agentforge`** 拖到 **“应用程序”** 文件夹（也可以放到你记得住的文件夹）。
4. 如果 macOS 提示“无法打开，因为来自身份不明的开发者”，请右键点击文件 → **打开**，再确认一次。

### Linux

1. 下载 **`agentforge-linux-amd64.tar.gz`**（ARM 设备下载 `...-arm64.tar.gz`）。
2. 打开终端，进入下载目录，例如：

   ```bash
   cd ~/Downloads
   ```

3. 解压并安装到系统目录（需要输入你的电脑开机密码）：

   ```bash
   tar -xzf agentforge-linux-amd64.tar.gz
   sudo install -m 0755 agentforge-linux-amd64/agentforge /usr/local/bin/
   ```

   > 如果你不想用 `sudo`，也可以把文件放到自己的目录：  
   > `mkdir -p ~/.local/bin && cp agentforge-linux-amd64/agentforge ~/.local/bin/`

---

## 第二步：启动程序

启动后你会看到一个文字聊天界面。不同系统启动方式如下。

### Windows

- **最简单：双击 `agentforge.exe`**。  
  如果双击后窗口一闪而过，请用下面更可靠的方式：
  1. 打开解压出来的文件夹。
  2. 在文件夹地址栏输入 `powershell` 并按回车（会打开 PowerShell，并且已经在这个文件夹里）。
  3. 输入下面的命令并按回车：

     ```powershell
     .\agentforge.exe
     ```

### macOS

1. 按键盘上的 `Command + 空格`，输入“终端”并回车，打开终端。
2. 如果你把程序拖到了“应用程序”，输入：

   ```bash
   /Applications/agentforge
   ```

   如果你放在“下载”文件夹里没移动，则输入：

   ```bash
   ~/Downloads/agentforge
   ```

3. 按回车，程序就会启动。

### Linux

1. 打开终端。
2. 输入：

   ```bash
   agentforge
   ```

   如果你把程序放到了 `~/.local/bin` 但提示找不到命令，先执行：

   ```bash
   export PATH="$HOME/.local/bin:$PATH"
   agentforge
   ```

3. 按回车，程序就会启动。

启动成功后，你应该能看到类似这样的文字界面：

```text
┌──────────────────────────┬──────────────────────────────────────────────┐
│ Sessions                 │ [session-xxx] [session-yyy]                   │
│ ▶ session-xxx            │──────────────────────────────────────────────│
│                          │ ◆ user                                       │
│                          │   你好                                       │
│                          │ ◆ assistant                                  │
│                          │   这是 AI 的回复...                           │
│                          │──────────────────────────────────────────────│
│                          │ > 输入消息，Enter 发送，Ctrl+C 退出            │
└──────────────────────────┴──────────────────────────────────────────────┘
```

---

## 第三步：连接 DeepSeek Harness 或直接使用厂商 Key

程序有两种常见用法，选一种你需要的。

### 方式 A：连接 DeepSeek Harness（DSH）

适合你已经安装并运行了 DSH 的情况。

1. 先打开一个终端，启动 DSH Web 服务：

   ```bash
   dsh --profile web --no-open --port 0
   ```

2. 程序会输出一行类似这样的地址：

   ```text
   dsh web: http://127.0.0.1:54695
   ```

3. 再打开另一个终端，启动 `agentforge` 并连接这个地址：

   - Windows（在解压文件夹的 PowerShell 里）：

     ```powershell
     .\agentforge.exe --api-url http://127.0.0.1:54695
     ```

   - macOS / Linux：

     ```bash
     agentforge --api-url http://127.0.0.1:54695
     ```

   > 注意：`54695` 是例子，请以你电脑上实际显示的端口为准。

### 方式 B：直接使用 DeepSeek API Key（不需要 DSH）

适合你只想用 DeepSeek 官方接口、不安装 DSH 的情况。

1. 打开 DeepSeek 开放平台，注册账号并创建一个 **API Key**（通常长这样：`sk-xxxxxxxx...`）。
2. 复制这个 Key，注意不要复制到多余空格。
3. 在终端里启动程序，把 `sk-你的密钥` 换成你自己的 Key：

   - Windows（在解压文件夹的 PowerShell 里）：

     ```powershell
     .\agentforge.exe --provider deepseek --provider-key sk-你的密钥
     ```

   - macOS / Linux：

     ```bash
     agentforge --provider deepseek --provider-key sk-你的密钥
     ```

4. 看到聊天界面后，直接输入文字发送，就能和 DeepSeek 对话了。

> 小提示：程序默认会使用 DeepSeek 的常用模型 `deepseek-chat`，一般不需要额外设置。  
> 如果你以后想指定模型，可以加 `--model deepseek-chat` 或 `--model deepseek-reasoner`。

### 没有 Key / 没有 DSH 怎么办？

可以先用 **体验模式（Mock）** 打开程序，不需要任何 Key，也不花钱：

- Windows：

  ```powershell
  .\agentforge.exe --mock
  ```

- macOS / Linux：

  ```bash
  agentforge --mock
  ```

体验模式会模拟一个 AI 回复，方便你先熟悉界面。

---

## 常见问题

### 1. 双击后窗口一闪而过 / 打不开

- 请改用**命令行方式**启动（见第二步），这样能看到错误信息。
- 如果是 Windows SmartScreen 提示，点击 **“更多信息” → “仍要运行”**。
- 如果是 macOS 提示“无法验证开发者”，右键 → **打开**。
- 如果是 Linux 提示 `Permission denied`，说明文件没有执行权限，可以运行：

  ```bash
  chmod +x agentforge
  ```

  然后重新启动。

### 2. 提示“不是内部或外部命令” / “command not found”

说明系统找不到这个程序。

- Windows：先进入解压后的文件夹，再用 `.\agentforge.exe`。
- macOS / Linux：检查程序路径是否正确；如果是自己放到 `~/.local/bin`，先执行上面的 `export PATH=...` 命令。

### 3. 连接失败 / 连不上

- **使用 DSH 时**：确认 DSH Web 服务已经启动，并确认 `--api-url` 里的端口和 DSH 输出的端口一致。
- **使用 DeepSeek Key 时**：
  - 检查网络是否正常。
  - 检查 Key 是否复制完整，前后不要有空格。
  - 检查 Key 是否有效、账户是否有余额或权限。
- 如果使用了代理软件，可能需要关闭代理或配置代理后再试。

### 4. 没有 Key 怎么办？

- 先用体验模式 `--mock` 看看界面。
- 想真正对话，可以到 DeepSeek 开放平台注册并创建 API Key。
- 如果只是本地测试，也可以考虑安装 Ollama 等本地模型工具（这部分需要更多电脑知识，暂时不展开）。

### 5. 如何退出程序？

- 按 **`Ctrl + C`**（Windows / macOS / Linux 通用）。
- 如果 AI 正在回复，第一次按是停止生成，再按一次就会退出。
- 如果当前没有正在生成的内容，按一次通常就会退出。

---

## 安全提示

- **API Key 就是你的密码**，不要截图发给别人，不要发到聊天群，不要贴到公开网站。
- 不要把 API Key 告诉任何人，包括本项目作者。
- 如果怀疑 Key 泄露，请立即到 DeepSeek 开放平台**删除/重置**这个 Key，再创建一个新的。
- 在公共电脑上使用后，建议退出程序并清理终端记录。
- 本项目有本地加密保存 Key 的功能（`--keychain`），但**最安全的习惯仍然是不分享 Key**。

---

## 小词典（术语解释）

| 词 | 简单解释 |
|---|---|
| TUI | 文字界面程序，在终端里显示，不用鼠标也能操作 |
| 终端 / PowerShell | 电脑里用来输入命令的黑色窗口 |
| DSH | DeepSeek Harness 的缩写，可以理解为一个 AI 服务管理工具 |
| API Key | AI 服务给你的密钥/密码，用来验证你是谁 |
| Provider | 模型厂商/服务商，例如 DeepSeek、OpenAI |
| Mock / 体验模式 | 程序内置的模拟模式，不需要真实服务也能试玩 |
| 个人项目 | 作者个人维护，主要供学习与日常使用 |
| Release | GitHub 上的正式发布页，可以下载安装包 |

---

如果你在阅读本文时遇到困难，可以先从“体验模式”开始：  
`agentforge --mock` 或 `.\agentforge.exe --mock`。

祝使用愉快！
