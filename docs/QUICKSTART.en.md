# agentforge Quickstart (No Experience Needed)

[中文](QUICKSTART.md) | [English](QUICKSTART.en.md)

> This guide is written for people who are not programmers.
> You do not need to know how to code. You only need to: **open a folder, double-click a file, and copy-paste commands**.
> Follow the steps in order. If you see an unfamiliar term, check the glossary at the end.

---

## What Is This? What Can It Do?

`agentforge` is an **AI chat workspace that runs in your computer's terminal**.

- It can connect to **DeepSeek Harness (DSH)** and let you chat with AI using a text interface.
- It can also **use a model provider's API key directly** (for example, a DeepSeek key) without installing DSH.
- It includes a **Mock mode** (experience mode), so you can open the interface even without a key.

The "terminal" is the black window where you can type commands:

- On Windows it is called **PowerShell** or **Command Prompt**
- On macOS it is called **Terminal**
- On Linux it is usually also called **Terminal**

This is a **personal project** primarily for the author's learning and daily use.

---

## What Do You Need?

| Requirement | Explanation |
|---|---|
| A computer | Windows 10/11, macOS, or Linux |
| Internet access | Needed to download packages and connect to AI services |
| A terminal window | See above: PowerShell on Windows, Terminal on macOS/Linux |
| Optional: DSH | If you already have DeepSeek Harness installed, you can connect to it |
| Optional: provider API key | For example a DeepSeek `sk-...` key; without one you can still use Mock mode |

> **What is an API key?**
> Think of it as the "password/key" for an AI service.
> The program needs it to call AI services such as DeepSeek on your behalf.
> It is private. **Do not share it with anyone.**

---

## Step 1: Download / Install

The program is published in the **Releases** page on GitHub.
Open the project's GitHub page, find **Releases**, and download the package for your computer.

> If you are not sure which one to pick:
> - A Windows 10/11 computer usually uses **`amd64`**.
> - A Mac bought after 2020 (Apple silicon) usually uses **`arm64`**; older Intel Macs use **`amd64`**.
> - A Linux computer usually uses **`amd64`**; ARM devices such as a Raspberry Pi use **`arm64`**.

### Windows

1. Download **`agentforge-windows-amd64.zip`** (or `...-arm64.zip` for ARM devices).
2. Right-click the archive and choose **Extract All**.
3. After extraction, you will see a folder containing **`agentforge.exe`**.
4. Remember where this folder is; you will need it in the next step.

### macOS

1. Download **`agentforge-darwin-amd64.dmg`** or **`agentforge-darwin-arm64.dmg`**.
   - If the Release page only has a `.zip`, download the matching `.zip` and double-click to unzip.
2. Double-click the `.dmg` file.
3. Drag **`agentforge`** into the **Applications** folder (or any folder you remember).
4. If macOS says it cannot open the app because it is from an unidentified developer, right-click the file → **Open**, then confirm.

### Linux

1. Download **`agentforge-linux-amd64.tar.gz`** (ARM devices: `...-arm64.tar.gz`).
2. Open a terminal and go to the download directory, for example:

   ```bash
   cd ~/Downloads
   ```

3. Extract and install it to a system directory (you will be asked for your password):

   ```bash
   tar -xzf agentforge-linux-amd64.tar.gz
   sudo install -m 0755 agentforge-linux-amd64/agentforge /usr/local/bin/
   ```

   > If you do not want to use `sudo`, you can put it in your own directory:
   > `mkdir -p ~/.local/bin && cp agentforge-linux-amd64/agentforge ~/.local/bin/`

---

## Step 2: Start the Program

After starting it, you will see a text-based chat interface. Here is how to start it on each system.

### Windows

- **Easiest: double-click `agentforge.exe`**.
  If the window closes immediately, use the more reliable method:
  1. Open the extracted folder.
  2. Type `powershell` in the folder address bar and press Enter (PowerShell opens in that folder).
  3. Type the following command and press Enter:

     ```powershell
     .\agentforge.exe
     ```

### macOS

1. Press `Command + Space`, type "Terminal", and press Enter.
2. If you dragged the program into Applications, type:

   ```bash
   /Applications/agentforge
   ```

   If you left it in Downloads, type:

   ```bash
   ~/Downloads/agentforge
   ```

3. Press Enter and the program will start.

### Linux

1. Open a terminal.
2. Type:

   ```bash
   agentforge
   ```

   If you put it in `~/.local/bin` and get "command not found", run:

   ```bash
   export PATH="$HOME/.local/bin:$PATH"
   agentforge
   ```

3. Press Enter and the program will start.

After it starts, you should see something like this:

```text
┌──────────────────────────┬──────────────────────────────────────────────┐
│ Sessions                 │ [session-xxx] [session-yyy]                   │
│ ▶ session-xxx            │──────────────────────────────────────────────│
│                          │ ◆ user                                       │
│                          │   Hello                                      │
│                          │ ◆ assistant                                  │
│                          │   This is an AI reply...                      │
│                          │──────────────────────────────────────────────│
│                          │ > Type a message, Enter to send, Ctrl+C to exit│
└──────────────────────────┴──────────────────────────────────────────────┘
```

---

## Step 3: Connect to DeepSeek Harness or Use a Provider Key

There are two common ways to use the program. Choose the one you need.

### Option A: Connect to DeepSeek Harness (DSH)

Use this if you already have DSH installed and running.

1. Open a terminal and start the DSH web service:

   ```bash
   dsh --profile web --no-open --port 0
   ```

2. The program will print an address similar to this:

   ```text
   dsh web: http://127.0.0.1:54695
   ```

3. Open another terminal and start `agentforge` with that address:

   - Windows (in PowerShell inside the extracted folder):

     ```powershell
     .\agentforge.exe --api-url http://127.0.0.1:54695
     ```

   - macOS / Linux:

     ```bash
     agentforge --api-url http://127.0.0.1:54695
     ```

   > Note: `54695` is only an example. Use the actual port shown on your computer.

### Option B: Use a DeepSeek API Key Directly (No DSH Needed)

Use this if you only want to use the DeepSeek official API without installing DSH.

1. Open the DeepSeek open platform, create an account, and create an **API key** (usually looks like `sk-xxxxxxxx...`).
2. Copy the key and make sure there are no extra spaces.
3. Start the program and replace `sk-YOUR_KEY` with your own key:

   - Windows (in PowerShell inside the extracted folder):

     ```powershell
     .\agentforge.exe --provider deepseek --provider-key sk-YOUR_KEY
     ```

   - macOS / Linux:

     ```bash
     agentforge --provider deepseek --provider-key sk-YOUR_KEY
     ```

4. When the chat interface appears, type a message and press Enter to chat with DeepSeek.

> Tip: The program uses DeepSeek's V4 model `deepseek-v4-flash` by default, so you usually do not need to configure anything else.
> To choose a model later, add `--model deepseek-v4-flash`, `--model deepseek-v4-pro`, or `--model deepseek-v4-vision` (`deepseek-v4-flash-vision-exp` is also supported). Legacy `deepseek-chat` / `deepseek-reasoner` remain available.

### No Key / No DSH?

You can use **Mock mode** to open the program without any key or cost:

- Windows:

  ```powershell
  .\agentforge.exe --mock
  ```

- macOS / Linux:

  ```bash
  agentforge --mock
  ```

Mock mode simulates an AI reply so you can get familiar with the interface.

---

## Frequently Asked Questions

### 1. The window flashes and closes / it won't open

- Use the **command-line method** from Step 2 so you can see error messages.
- If Windows SmartScreen warns you, click **More info** → **Run anyway**.
- If macOS says the developer cannot be verified, right-click → **Open**.
- If Linux says `Permission denied`, the file may not be executable. Run:

  ```bash
  chmod +x agentforge
  ```

  Then start it again.

### 2. "'agentforge' is not recognized" / "command not found"

The system cannot find the program.

- Windows: go into the extracted folder first, then use `.\agentforge.exe`.
- macOS / Linux: check the program path; if you put it in `~/.local/bin`, run the `export PATH=...` command above first.

### 3. Connection failed / cannot connect

- **When using DSH**: make sure the DSH web service is running and that the port in `--api-url` matches the port printed by DSH.
- **When using a DeepSeek key**:
  - Check your network connection.
  - Check that the key was copied completely, with no spaces at the beginning or end.
  - Check that the key is valid and the account has balance/permissions.
- If you use proxy software, you may need to disable or configure the proxy first.

### 4. What if I don't have a key?

- Use Mock mode first with `--mock` to see the interface.
- To chat for real, register on the DeepSeek open platform and create an API key.
- For local testing only, you can also try local model tools such as Ollama (this requires more computer knowledge and is not covered here).

### 5. How do I exit the program?

- Press **`Ctrl + C`** (Windows / macOS / Linux).
- If AI is replying, the first press stops generation; press again to exit.
- If nothing is being generated, one press usually exits.

---

## Security Tips

- **Your API key is like a password.** Do not screenshot it and send it to others, post it in chat groups, or paste it on public websites.
- Do not tell your API key to anyone, including the project author.
- If you think your key has leaked, immediately **delete/reset it** on the DeepSeek open platform and create a new one.
- After using a public computer, it is recommended to exit the program and clear terminal history.
- The project supports saving keys locally with encryption (`--keychain`), but the safest habit is still to avoid sharing keys.

---

## Glossary

| Term | Simple explanation |
|---|---|
| TUI | A text-based interface program that runs in the terminal and can be used without a mouse |
| Terminal / PowerShell | The black window on your computer where you type commands |
| DSH | Short for DeepSeek Harness; think of it as an AI service management tool |
| API key | The secret/key an AI service gives you to verify who you are |
| Provider | A model vendor/service, e.g. DeepSeek, OpenAI |
| Mock / experience mode | A built-in simulation mode that does not need a real service |
| Open source | The source code is public; anyone can view, use, and help improve it |
| Release | The official publishing page on GitHub where you can download install packages |

---

If you get stuck while reading this guide, start with Mock mode:
`agentforge --mock` or `.\agentforge.exe --mock`.

Enjoy!
