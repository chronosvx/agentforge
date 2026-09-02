package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/plugin"
)

// runMarket 实现 `agentforge market`：搜索、安装和列出插件市场中的插件。
//
// 用法：
//
// agentforge market [--data-dir DIR] search <index-url> <query>
// agentforge market [--data-dir DIR] install <index-url> <name>
// agentforge market [--data-dir DIR] list
func runMarket(args []string) error {
	dataDir, rest, help, err := parseMarketArgs(args)
	if err != nil {
		return err
	}
	if help {
		return nil
	}
	if len(rest) == 0 {
		printMarketUsage()
		return errors.New("market: expected search, install or list")
	}

	switch rest[0] {
	case "search":
		if len(rest) != 3 {
			return errors.New("market: usage: market search <index-url> <query>")
		}
		return marketSearch(rest[1], rest[2])
	case "install":
		if len(rest) != 3 {
			return errors.New("market: usage: market install <index-url> <name>")
		}
		dir, err := resolveMarketDataDir(dataDir)
		if err != nil {
			return err
		}
		return marketInstall(rest[1], rest[2], dir)
	case "list":
		if len(rest) != 1 {
			return errors.New("market: usage: market list")
		}
		dir, err := resolveMarketDataDir(dataDir)
		if err != nil {
			return err
		}
		return marketList(dir)
	default:
		return fmt.Errorf("market: unknown subcommand %q (expected search, install or list)", rest[0])
	}
}

// resolveMarketDataDir 返回 market 使用的数据目录：优先使用 --data-dir，
// 否则读取与主程序一致的配置文件。
func resolveMarketDataDir(dataDir string) (string, error) {
	if dataDir == "" {
		cfg, err := config.Parse(nil)
		if err != nil {
			return "", fmt.Errorf("market: load config: %w", err)
		}
		dataDir = cfg.DataDir
	} else {
		expanded, err := expandHome(dataDir)
		if err != nil {
			return "", fmt.Errorf("market: expand data dir: %w", err)
		}
		dataDir = expanded
	}
	return dataDir, nil
}

// parseMarketArgs 从 market 参数中提取可选的 --data-dir，并保留位置参数。
// 该解析器允许 --data-dir 出现在子命令前后，方便测试和脚本使用。
func parseMarketArgs(args []string) (dataDir string, rest []string, help bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--data-dir":
			if i+1 >= len(args) {
				return "", nil, false, errors.New("market: --data-dir requires a value")
			}
			dataDir = args[i+1]
			i++
		case strings.HasPrefix(arg, "--data-dir="):
			dataDir = strings.TrimPrefix(arg, "--data-dir=")
		case arg == "-h" || arg == "--help":
			printMarketUsage()
			return "", nil, true, nil
		default:
			rest = append(rest, arg)
		}
	}
	return dataDir, rest, false, nil
}

func printMarketUsage() {
	fmt.Print(`Usage:
  agentforge market [--data-dir DIR] search <index-url> <query>
  agentforge market [--data-dir DIR] install <index-url> <name>
  agentforge market [--data-dir DIR] list

Commands:
  search   Fetch the market index and fuzzy match plugin name/description
  install  Install a plugin from the market into <data-dir>/plugins
  list     List locally installed plugins
`)
}

// marketSearch 拉取市场索引并对名称/描述做大小写不敏感的模糊匹配。
func marketSearch(indexURL, query string) error {
	index, err := plugin.FetchMarketIndex(context.Background(), indexURL)
	if err != nil {
		return fmt.Errorf("market: fetch index: %w", err)
	}

	q := strings.ToLower(strings.TrimSpace(query))
	var matches []plugin.MarketIndex
	for _, entry := range index {
		if marketEntryMatches(entry, q) {
			matches = append(matches, entry)
		}
	}

	fmt.Printf("Found %d plugin(s) matching %q:\n", len(matches), query)
	for _, entry := range matches {
		fmt.Printf("  - %s %s: %s\n", entry.Name, entry.Version, entry.Description)
		fmt.Printf("    %s\n", entry.URL)
	}
	return nil
}

// marketInstall 从市场索引安装指定插件到 <dataDir>/plugins。
func marketInstall(indexURL, name, dataDir string) error {
	index, err := plugin.FetchMarketIndex(context.Background(), indexURL)
	if err != nil {
		return fmt.Errorf("market: fetch index: %w", err)
	}

	installed, err := plugin.InstallFromMarket(context.Background(), index, strings.TrimSpace(name), filepath.Join(dataDir, "plugins"))
	if err != nil {
		return fmt.Errorf("market: install %q: %w", name, err)
	}

	fmt.Printf("Installed plugin %s %s\n", installed.Manifest.Name, installed.Manifest.Version)
	if installed.Manifest.Description != "" {
		fmt.Printf("  %s\n", installed.Manifest.Description)
	}
	fmt.Printf("  location: %s\n", filepath.Join(dataDir, "plugins", installed.Manifest.Name))
	return nil
}

// marketList 列出 <dataDir>/plugins 下已安装的外部插件。
func marketList(dataDir string) error {
	pluginsDir := filepath.Join(dataDir, "plugins")
	mgr := plugin.NewManager()
	if err := mgr.LoadDir(pluginsDir); err != nil {
		return fmt.Errorf("market: list plugins: %w", err)
	}

	installed := mgr.List()
	if len(installed) == 0 {
		fmt.Printf("No plugins installed in %s\n", pluginsDir)
		return nil
	}

	fmt.Printf("Installed plugins (%d) in %s:\n", len(installed), pluginsDir)
	for _, p := range installed {
		fmt.Printf("  - %s %s: %s\n", p.Manifest.Name, p.Manifest.Version, p.Manifest.Description)
	}
	return nil
}

// marketEntryMatches 判断市场条目是否匹配查询串：名称/描述包含查询，或子序列模糊匹配。
func marketEntryMatches(entry plugin.MarketIndex, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	name := strings.ToLower(entry.Name)
	desc := strings.ToLower(entry.Description)
	if strings.Contains(name, q) || strings.Contains(desc, q) {
		return true
	}
	return marketFuzzySubsequence(name, q) || marketFuzzySubsequence(desc, q)
}

// marketFuzzySubsequence 判断 query 是否为 s 的子序列（字符按顺序出现，可间隔）。
func marketFuzzySubsequence(s, query string) bool {
	if query == "" {
		return true
	}
	sr := []rune(s)
	qr := []rune(query)
	si := 0
	for _, qr := range qr {
		for si < len(sr) && sr[si] != qr {
			si++
		}
		if si >= len(sr) {
			return false
		}
		si++
	}
	return true
}
