package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunSetupStdinUnavailableReturnsError 确保非交互环境下 setup 会返回可读错误，
// 而不是卡住等待输入或写入空配置。
func TestRunSetupStdinUnavailableReturnsError(t *testing.T) {
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdin = r
	_ = w.Close()
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()

	dir := t.TempDir()
	err = runSetup([]string{"--data-dir", dir})
	if err == nil {
		t.Fatal("expected error when stdin is unavailable")
	}
	if !strings.Contains(err.Error(), "stdin") && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("expected a readable stdin error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(statErr) {
		t.Fatalf("config should not be written when setup cannot read stdin: %v", statErr)
	}
}

// TestAskSetupSecretMasksExistingValue 验证 setup 的敏感字段默认值不会明文回显，
// 且用户留空时保留原有值而不是写入占位符。
func TestAskSetupSecretMasksExistingValue(t *testing.T) {
	oldStdin := os.Stdin
	oldStdout := os.Stdout
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	os.Stdin = inR
	if _, err := inW.WriteString("\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = inW.Close()
	defer func() {
		os.Stdin = oldStdin
		_ = inR.Close()
	}()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = outW
	defer func() {
		os.Stdout = oldStdout
		_ = outR.Close()
	}()

	reader := bufio.NewReader(inR)
	got, err := askSetupSecret(reader, "Provider API Key", "sk-super-secret", false)
	_ = outW.Close()
	out, _ := io.ReadAll(outR)

	if err != nil {
		t.Fatalf("askSetupSecret: %v", err)
	}
	if got != "sk-super-secret" {
		t.Fatalf("askSetupSecret with blank input = %q, want existing value preserved", got)
	}
	text := string(out)
	if strings.Contains(text, "sk-super-secret") {
		t.Fatalf("setup prompt leaked the current secret value: %q", text)
	}
	if !strings.Contains(text, "[已设置]") {
		t.Fatalf("setup prompt should display masked default marker, got: %q", text)
	}
}

// TestWriteSetupConfig 验证配置以 JSON 写入且权限为 0600。
func TestWriteSetupConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	cfg := map[string]any{
		"mock":         true,
		"provider":     "deepseek",
		"provider-key": "sk-test",
		"api-url":      "http://127.0.0.1:54695",
	}

	if err := writeSetupConfig(path, cfg); err != nil {
		t.Fatalf("writeSetupConfig: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("written config is not valid JSON: %v\n%s", err, data)
	}
	if got["mock"] != true {
		t.Fatalf("mock = %#v, want true", got["mock"])
	}
	if got["provider"] != "deepseek" || got["provider-key"] != "sk-test" || got["api-url"] != "http://127.0.0.1:54695" {
		t.Fatalf("unexpected config: %#v", got)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("config permissions = %o, want 600", fi.Mode().Perm())
	}
}
