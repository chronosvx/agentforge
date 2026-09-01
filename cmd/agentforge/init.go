package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runInit 实现 `agentforge init`：生成用户配置文件。
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	provider := fs.String("provider", "", "直连厂商名：deepseek/openai/moonshot/qwen/ollama")
	model := fs.String("model", "", "模型名")
	baseURL := fs.String("base-url", "", "OpenAI 兼容 Base URL（可选）")
	apiURL := fs.String("api-url", "", "DSH Web API 地址（可选）")
	apiToken := fs.String("api-token", "", "DSH Web API Token（可选）")
	keychain := fs.Bool("keychain", false, "使用系统 Keychain 保存 Provider Key")
	encryptSessions := fs.Bool("encrypt-sessions", false, "启用加密会话存储")
	dataDir := fs.String("data-dir", "~/.agentforge", "数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	cfg := map[string]any{}
	if *provider != "" {
		cfg["provider"] = *provider
	}
	if *model != "" {
		cfg["model"] = *model
	}
	if *baseURL != "" {
		cfg["provider-base-url"] = *baseURL
	}
	if *apiURL != "" {
		cfg["api-url"] = *apiURL
	}
	if *apiToken != "" {
		cfg["api-token"] = *apiToken
	}
	if *keychain {
		cfg["keychain"] = true
	}
	if *encryptSessions {
		cfg["encrypt-sessions"] = true
	}

	// Provider Key：优先从 --provider-key-stdin 语义读取，这里简单从 stdin 读取。
	if *provider != "" && !*keychain {
		fmt.Print("Enter provider key (input will be visible): ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		key := strings.TrimSpace(line)
		if key != "" {
			cfg["provider-key"] = key
		}
	}

	if len(cfg) == 0 {
		return errors.New("init: nothing to configure; use --provider/--api-url etc")
	}

	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Printf("config written to %s\n", path)
	return nil
}
