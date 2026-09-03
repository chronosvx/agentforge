package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chronosvx/agentforge/internal/config"
)

// runInit 实现 `agentforge init`：生成或更新用户配置文件。
//
// 如果已有配置文件，init 会读取并保留已有配置，只覆盖本次显式提供的字段。
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	provider := fs.String("provider", "", "直连厂商名：deepseek/openai/moonshot/qwen/ollama")
	model := fs.String("model", "", "模型名（DeepSeek 默认 deepseek-v4-flash，可选 deepseek-v4-pro / deepseek-v4-vision / deepseek-v4-flash-vision-exp）")
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

	path := filepath.Join(dir, "config.json")
	cfg := map[string]any{}

	// 复用已有配置：先读取旧值，再覆盖本次显式提供的字段。
	if existing, err := config.LoadFile(path); err == nil && len(existing) > 0 {
		for k, v := range existing {
			cfg[k] = v
		}
	}

	provided := false
	if *provider != "" {
		cfg["provider"] = *provider
		provided = true
	}
	if *model != "" {
		cfg["model"] = *model
		provided = true
	}
	if *baseURL != "" {
		cfg["provider-base-url"] = *baseURL
		provided = true
	}
	if *apiURL != "" {
		cfg["api-url"] = *apiURL
		provided = true
	}
	if *apiToken != "" {
		cfg["api-token"] = *apiToken
		provided = true
	}
	if *keychain {
		cfg["keychain"] = true
		provided = true
	}
	if *encryptSessions {
		cfg["encrypt-sessions"] = true
		provided = true
	}

	// 没有提供新配置且已有配置时，不覆盖，直接提示。
	if !provided && len(cfg) > 0 {
		fmt.Printf("using existing config at %s\n", path)
		return nil
	}

	// Provider Key：仅当本次显式指定 provider 且未使用 keychain 时提示输入。
	if *provider != "" && !*keychain {
		reader := bufio.NewReader(os.Stdin)
		key, err := readSecretInput(reader, "Enter provider key (input hidden; leave empty to keep existing): ")
		if err != nil {
			return err
		}
		if key != "" {
			cfg["provider-key"] = key
		}
	}

	if len(cfg) == 0 {
		return errors.New("init: nothing to configure; use --provider/--api-url etc")
	}

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
