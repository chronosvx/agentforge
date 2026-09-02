package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/x/term"
)

// runSetup 实现 `agentforge setup`：以向导方式选择后端并生成/更新配置。
//
// 与 init 类似，setup 会先读取已有 <data-dir>/config.json；本次问答未覆盖的
// 已有字段会保留在配置中。
func runSetup(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "~/.agentforge", "数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("setup: unexpected arguments: %v", fs.Args())
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("setup: create data dir: %w", err)
	}

	path := filepath.Join(dir, "config.json")
	cfg := map[string]any{}
	if _, err := os.Stat(path); err == nil {
		existing, err := loadSetupConfig(path)
		if err != nil {
			return err
		}
		cfg = existing
		fmt.Printf("检测到已有配置 %s，本次未覆盖的字段会保留。\n", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("setup: stat config: %w", err)
	}

	fmt.Println("agentforge setup 首次运行向导")
	fmt.Println("请选择要使用的后端：")
	fmt.Println("  1) Mock 体验")
	fmt.Println("  2) DSH Web API")
	fmt.Println("  3) Provider 直连")

	reader := bufio.NewReader(os.Stdin)
	choice, err := askSetupBackend(reader)
	if err != nil {
		return err
	}

	switch choice {
	case "1":
		cfg["mock"] = true
	case "2":
		apiURL, err := askSetupValue(reader, "DSH Web API 地址", configString(cfg["api-url"]), true)
		if err != nil {
			return err
		}
		if apiURL != "" {
			cfg["api-url"] = apiURL
		}

		apiToken, err := askSetupSecret(reader, "DSH Web API Token（可留空）", configString(cfg["api-token"]), false)
		if err != nil {
			return err
		}
		if apiToken != "" {
			cfg["api-token"] = apiToken
		}
	case "3":
		provider, err := askSetupValue(reader, "Provider 名称 (deepseek/openai/moonshot/qwen/ollama)", configString(cfg["provider"]), true)
		if err != nil {
			return err
		}
		if provider != "" {
			cfg["provider"] = provider
		}

		model, err := askSetupValue(reader, "Model（可留空使用默认）", configString(cfg["model"]), false)
		if err != nil {
			return err
		}
		if model != "" {
			cfg["model"] = model
		}

		providerKey, err := askSetupSecret(reader, "Provider API Key（输入隐藏）", configString(cfg["provider-key"]), false)
		if err != nil {
			return err
		}
		if providerKey != "" {
			cfg["provider-key"] = providerKey
		}
	default:
		return fmt.Errorf("setup: unsupported backend choice %q", choice)
	}

	if err := writeSetupConfig(path, cfg); err != nil {
		return fmt.Errorf("setup: write config: %w", err)
	}

	fmt.Printf("配置已写入 %s\n", path)
	switch choice {
	case "1":
		fmt.Println("现在运行 agentforge --mock 体验")
	case "2":
		apiURL := configString(cfg["api-url"])
		if apiURL != "" {
			fmt.Printf("现在运行 agentforge --api-url %s\n", apiURL)
		} else {
			fmt.Println("现在运行 agentforge --api-url <你的 DSH Web API 地址>")
		}
	case "3":
		provider := configString(cfg["provider"])
		if provider != "" {
			fmt.Printf("现在运行 agentforge --provider %s\n", provider)
		} else {
			fmt.Println("现在运行 agentforge --provider <你的 Provider 名称>")
		}
	}
	return nil
}

// askSetupBackend 读取并校验用户选择的后端编号。
func askSetupBackend(reader *bufio.Reader) (string, error) {
	for {
		fmt.Print("请输入 1-3: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if line == "" {
				return "", fmt.Errorf("setup: 读取后端选择失败: %w", err)
			}
		}
		switch strings.TrimSpace(line) {
		case "1", "2", "3":
			return strings.TrimSpace(line), nil
		default:
			fmt.Println("无效选择，请输入 1、2 或 3。")
		}
	}
}

// askSetupValue 读取一行输入。current 是已有配置值，用户留空时返回 current；
// required 为 true 且没有 current 时会循环要求输入。
func askSetupValue(reader *bufio.Reader, label, current string, required bool) (string, error) {
	for {
		if current != "" {
			fmt.Printf("%s [%s]: ", label, current)
		} else {
			fmt.Printf("%s: ", label)
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if line == "" {
				return "", fmt.Errorf("setup: 读取%s失败: %w", label, err)
			}
		}
		value := strings.TrimSpace(line)
		if value != "" {
			return value, nil
		}
		if current != "" {
			return current, nil
		}
		if !required {
			return "", nil
		}
		fmt.Println("该项不能为空，请重新输入。")
	}
}

// askSetupSecret 读取敏感字段。current 是已有配置值，但提示中只显示 [已设置]，
// 不会把明文密钥打印到终端；在支持无回显输入的终端上会关闭本地回显。
func askSetupSecret(reader *bufio.Reader, label, current string, required bool) (string, error) {
	for {
		prompt := label
		if current != "" {
			prompt += " [已设置]"
		}
		prompt += ": "

		value, err := readSecretInput(reader, prompt)
		if err != nil {
			return "", fmt.Errorf("setup: 读取%s失败: %w", label, err)
		}
		if value != "" {
			return value, nil
		}
		if current != "" {
			return current, nil
		}
		if !required {
			return "", nil
		}
		fmt.Println("该项不能为空，请重新输入。")
	}
}

// readSecretInput 输出 prompt 后读取一行敏感输入。
//
// 在终端上使用 x/term 关闭本地回显；在非终端（管道、测试）中回退到普通
// bufio 读取，便于脚本和非交互调用继续工作。
func readSecretInput(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	if term.IsTerminal(os.Stdin.Fd()) {
		data, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Println()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		if line == "" {
			return "", err
		}
	}
	return strings.TrimSpace(line), nil
}

// configString 将配置值转为用于提示的字符串。
func configString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// loadSetupConfig 读取已有 config.json，兼容 JSON 对象与 key=value 行格式。
func loadSetupConfig(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("setup: read existing config %s: %w", path, err)
	}

	cfg := map[string]any{}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return cfg, nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("setup: parse existing config %s: %w", path, err)
		}
		for k, v := range raw {
			cfg[strings.ToLower(k)] = v
		}
		return cfg, nil
	}

	sc := bufio.NewScanner(strings.NewReader(trimmed))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexAny(line, "=:"); idx > 0 {
			k := strings.ToLower(strings.TrimSpace(line[:idx]))
			v := strings.TrimSpace(line[idx+1:])
			v = strings.Trim(v, `"'`)
			cfg[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("setup: parse existing config %s: %w", path, err)
	}
	return cfg, nil
}

// writeSetupConfig 将配置 map 以 JSON 写入 path，权限为 0600。
func writeSetupConfig(path string, cfg map[string]any) error {
	if cfg == nil {
		cfg = map[string]any{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
