package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// runDoctor 实现 `agentforge doctor`：检查本机环境是否满足运行要求。
func runDoctor(args []string) error {
	fs := newFlagSet("doctor")
	dataDir := fs.String("data-dir", "", "自定义数据目录（默认 ~/.agentforge）")
	apiURL := fs.String("api-url", "", "DSH Web API 基础 URL")
	apiToken := fs.String("api-token", "", "DSH Web API token")
	fs.StringVar(apiToken, "dsh-api-token", "", "DSH Web API token（--api-token 的别名）")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := *dataDir
	if dir == "" {
		dir = "~/.agentforge"
	}
	expanded, err := expandHome(dir)
	if err != nil {
		return err
	}

	fmt.Println("agentforge doctor")
	fmt.Println("=================")
	ok := true
	var hints []doctorHint

	ok = reportWithHint("Go toolchain", checkGo(), ok, &hints, "Install Go from https://go.dev/dl")
	ok = reportWithHint("DSH CLI (dsh)", checkExecutable("dsh"), ok, &hints, "npm i -g @deepseek-ai/dsh 或参考官方文档")
	ok = reportWithHint("Git", checkExecutable("git"), ok, &hints, "安装 git")
	ok = reportWithHint("GitHub CLI (gh)", checkExecutable("gh"), ok, &hints, "安装 gh")
	ok = reportWithHint("GitHub auth (gh auth status)", checkGitHubAuth(), ok, &hints, "运行 gh auth login")
	ok = reportWithHint("Keychain backend", checkKeychainBackend(), ok, &hints, keychainBackendHint())
	ok = reportWithHint("Data directory", checkDataDir(expanded), ok, &hints, "若权限过宽：chmod 700 <dir>")
	reportInfo("Provider env (AGENTFORGE_PROVIDER)", os.Getenv("AGENTFORGE_PROVIDER"))
	reportInfo("MCP URL (MCP_URL)", os.Getenv("MCP_URL"))

	ok = reportWithHint("DSH version", checkDSHVersion(), ok, &hints, "安装或更新 dsh CLI")
	if *apiURL != "" {
		ok = reportWithHint("DSH Web API", checkDSHWebAPI(*apiURL, *apiToken), ok, &hints, "先启动 dsh web（dsh web --no-open --port 0），或检查 --api-url/--api-token")
	} else {
		reportInfo("DSH Web API url", "(not set)")
	}

	reportTerminal(&hints)

	if ok {
		fmt.Println("\nAll checks passed.")
	} else {
		fmt.Println("\nSome checks failed. See messages above.")
		printSuggestedFixes(hints)
	}
	return nil
}

// doctorHint 保存一条可执行修复建议。
type doctorHint struct {
	name string
	hint string
}

// addHint 将一条修复建议追加到 hints 中；hint 为空时不记录。
func addHint(hints *[]doctorHint, name, hint string) {
	if hints == nil || hint == "" {
		return
	}
	*hints = append(*hints, doctorHint{name: name, hint: hint})
}

// reportWithHint 与 report 行为一致，但检查失败时会同时记录修复建议。
func reportWithHint(name string, err error, ok bool, hints *[]doctorHint, hint string) bool {
	if err == nil {
		fmt.Printf("[PASS] %s\n", name)
		return ok
	}
	fmt.Printf("[WARN] %s: %v\n", name, err)
	addHint(hints, name, hint)
	return false
}

// printSuggestedFixes 在最终诊断信息后打印可执行修复建议。
func printSuggestedFixes(hints []doctorHint) {
	if len(hints) == 0 {
		return
	}
	fmt.Println("\nSuggested fixes:")
	for _, h := range hints {
		fmt.Printf("  - %s: %s\n", h.name, h.hint)
	}
}

func keychainBackendHint() string {
	switch runtime.GOOS {
	case "darwin":
		return "安装 security（macOS 通常自带）"
	case "windows":
		return "安装 powershell"
	case "linux":
		return "安装 secret-tool（例如 apt install libsecret-tools）"
	default:
		return "按平台安装 security/powershell/secret-tool"
	}
}

func report(name string, err error, ok bool) bool {
	return reportWithHint(name, err, ok, nil, "")
}

func reportInfo(name, value string) {
	if value == "" {
		fmt.Printf("[INFO] %s: (not set)\n", name)
		return
	}
	fmt.Printf("[INFO] %s: %s\n", name, value)
}

// reportTerminal 输出终端兼容性报告区：TERM/COLORTERM/LANG 与真彩色/UTF-8 建议。
func reportTerminal(hints *[]doctorHint) {
	fmt.Println("\nTerminal capability")
	reportInfo("TERM", os.Getenv("TERM"))
	reportInfo("COLORTERM", os.Getenv("COLORTERM"))
	reportInfo("LANG", os.Getenv("LANG"))

	if terminalTrueColor() {
		fmt.Println("[PASS] Truecolor: COLORTERM indicates truecolor support")
	} else {
		fmt.Println("[INFO] Truecolor: not detected (COLORTERM is not truecolor/24bit)")
	}

	if terminalNeedsANSISuggestion() {
		fmt.Println("[HINT] TERM is empty or dumb; 建议使用支持 ANSI/真彩色的终端")
		addHint(hints, "Terminal", "建议使用支持 ANSI/真彩色的终端")
	}
	if localeNeedsUTF8Suggestion() {
		fmt.Println("[HINT] LANG is empty or C; 建议设置 UTF-8 locale 以获得更好的中文/emoji 显示")
		addHint(hints, "Locale", "建议设置 UTF-8 locale 以获得更好的中文/emoji 显示")
	}
}

// terminalTrueColor 报告 COLORTERM 是否疑似启用真彩色。
func terminalTrueColor() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("COLORTERM")))
	return v == "truecolor" || v == "24bit"
}

// terminalNeedsANSISuggestion 报告当前 TERM 是否为空或 dumb，需要 ANSI/真彩色终端建议。
func terminalNeedsANSISuggestion() bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	return term == "" || strings.EqualFold(term, "dumb")
}

// localeNeedsUTF8Suggestion 报告 LANG 是否为空或 C/POSIX，需要 UTF-8 locale 建议。
func localeNeedsUTF8Suggestion() bool {
	lang := strings.TrimSpace(os.Getenv("LANG"))
	return lang == "" || strings.EqualFold(lang, "C") || strings.EqualFold(lang, "POSIX")
}

func checkGo() error {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return fmt.Errorf("go version returned empty output")
	}
	fmt.Printf("       %s\n", version)
	return nil
}

func checkDSHVersion() error {
	out, err := exec.Command("dsh", "--version").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%v: %s", err, text)
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return fmt.Errorf("dsh --version returned empty output")
	}
	fmt.Printf("       %s\n", version)
	return nil
}

func checkDSHWebAPI(apiURL, apiToken string) error {
	body := `{"type":"client-request","rpcId":"doctor","method":"host.describe","payload":{}}`
	endpoint := strings.TrimRight(apiURL, "/") + "/api/host.describe"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response from %s: %w", endpoint, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text := strings.TrimSpace(string(respBody))
		if text == "" {
			return fmt.Errorf("unexpected HTTP status %s", resp.Status)
		}
		return fmt.Errorf("unexpected HTTP status %s: %s", resp.Status, text)
	}

	text := strings.ToLower(string(respBody))
	if !strings.Contains(text, "server-response") && !strings.Contains(text, "ok") {
		return fmt.Errorf("unexpected response from %s: HTTP %s (missing server-response/ok): %s", endpoint, resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func checkExecutable(name string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", name)
	}
	fmt.Printf("       found at %s\n", path)
	return nil
}

func checkGitHubAuth() error {
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" {
			return err
		}
		return fmt.Errorf("%v: %s", err, text)
	}
	return nil
}

func checkKeychainBackend() error {
	switch runtime.GOOS {
	case "darwin":
		return checkExecutable("security")
	case "windows":
		return checkExecutable("powershell")
	case "linux":
		return checkExecutable("secret-tool")
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}

func checkDataDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		fmt.Printf("       %s does not exist yet; it will be created on first run\n", dir)
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf("%s permissions are %04o (expected 0700 or stricter)", filepath.Clean(dir), mode)
	}
	fmt.Printf("       %s\n", filepath.Clean(dir))
	return nil
}

// newFlagSet 创建一个不会把 -h 当错误的 flag.FlagSet，便于子命令统一处理。
func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}
