package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
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
		allowUnsigned, yes, installArgs, err := parseMarketInstallArgs(rest)
		if err != nil {
			return err
		}
		if len(installArgs) != 3 {
			return errors.New("market: usage: market install [--allow-unsigned] [--yes] <index-url> <name>")
		}
		dir, err := resolveMarketDataDir(dataDir)
		if err != nil {
			return err
		}
		return marketInstallWithOptions(installArgs[1], installArgs[2], dir, allowUnsigned, yes)
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

// parseMarketInstallArgs 从 market install 的参数中提取 --allow-unsigned/--yes，
// 并返回保留子命令和位置参数的新列表。
func parseMarketInstallArgs(args []string) (allowUnsigned, yes bool, install []string, err error) {
	if len(args) == 0 {
		return false, false, nil, errors.New("market: install requires an index URL and plugin name")
	}
	install = []string{args[0]}
	for _, arg := range args[1:] {
		switch arg {
		case "--allow-unsigned":
			allowUnsigned = true
		case "--yes", "-y":
			yes = true
		case "--allow-unsigned=true":
			allowUnsigned = true
		case "--allow-unsigned=false":
			allowUnsigned = false
		case "--yes=true":
			yes = true
		case "--yes=false":
			yes = false
		default:
			install = append(install, arg)
		}
	}
	return allowUnsigned, yes, install, nil
}

func printMarketUsage() {
	fmt.Print(`Usage:
  agentforge market [--data-dir DIR] search <index-url> <query>
  agentforge market [--data-dir DIR] install [--allow-unsigned] [--yes] <index-url> <name>
  agentforge market [--data-dir DIR] list

Commands:
  search   Fetch the market index and fuzzy match plugin name/description
  install  Install a plugin from the market into <data-dir>/plugins
  list     List locally installed plugins

Install options:
  --allow-unsigned  Allow installing entries with neither SHA256 nor signature
  --yes             Skip the permission confirmation prompt
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
// 保留旧签名，默认不允许无签名/无哈希安装且要求交互确认。
func marketInstall(indexURL, name, dataDir string) error {
	return marketInstallWithOptions(indexURL, name, dataDir, false, false)
}

// marketInstallWithOptions 是 market install 的实现，支持显式 --allow-unsigned 与 --yes。
func marketInstallWithOptions(indexURL, name, dataDir string, allowUnsigned, yes bool) error {
	index, err := plugin.FetchMarketIndex(context.Background(), indexURL)
	if err != nil {
		return fmt.Errorf("market: fetch index: %w", err)
	}

	trimmedName := strings.TrimSpace(name)
	var entry *plugin.MarketIndex
	for i := range index {
		if strings.TrimSpace(index[i].Name) == trimmedName {
			entry = &index[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("market: install %q: %w", name, plugin.ErrMarketEntryNotFound)
	}

	if !allowUnsigned && strings.TrimSpace(entry.SHA256) == "" {
		if entry.Signature.IsZero() {
			return fmt.Errorf("market: install %q: entry has neither SHA256 nor signature; pass --allow-unsigned to install without integrity verification", name)
		}
		return fmt.Errorf("market: install %q: entry has no SHA256 and no trust store is configured to verify its signature; pass --allow-unsigned to install without integrity verification", name)
	}

	destDir := filepath.Join(dataDir, "plugins")

	if !yes {
		inspected, err := inspectMarketPlugin(index, trimmedName, allowUnsigned)
		if err != nil {
			return fmt.Errorf("market: inspect %q: %w", name, err)
		}
		fmt.Printf("Plugin %s %s\n", inspected.Manifest.Name, inspected.Manifest.Version)
		if inspected.Manifest.Description != "" {
			fmt.Printf("  %s\n", inspected.Manifest.Description)
		}
		fmt.Printf("Permissions:\n%s\n", formatPermissions(inspected.Manifest.Permissions))
		if !confirmMarketInstall() {
			fmt.Println("Install cancelled.")
			return nil
		}
	}

	installed, err := installMarketPlugin(index, trimmedName, destDir, allowUnsigned)
	if err != nil {
		return fmt.Errorf("market: install %q: %w", name, err)
	}

	fmt.Printf("Installed plugin %s %s\n", installed.Manifest.Name, installed.Manifest.Version)
	if installed.Manifest.Description != "" {
		fmt.Printf("  %s\n", installed.Manifest.Description)
	}
	fmt.Printf("  location: %s\n", filepath.Join(destDir, installed.Manifest.Name))
	return nil
}

func inspectMarketPlugin(index []plugin.MarketIndex, name string, allowUnsigned bool) (plugin.Plugin, error) {
	tmp, err := os.MkdirTemp("", "agentforge-market-inspect-*")
	if err != nil {
		return plugin.Plugin{}, err
	}
	defer os.RemoveAll(tmp)

	if allowUnsigned {
		return plugin.InstallFromMarketAllowUnsigned(context.Background(), index, name, tmp)
	}
	return plugin.InstallFromMarket(context.Background(), index, name, tmp)
}

func installMarketPlugin(index []plugin.MarketIndex, name, destDir string, allowUnsigned bool) (plugin.Plugin, error) {
	if allowUnsigned {
		return plugin.InstallFromMarketAllowUnsigned(context.Background(), index, name, destDir)
	}
	return plugin.InstallFromMarket(context.Background(), index, name, destDir)
}

func confirmMarketInstall() bool {
	fmt.Print("Proceed with installation? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func formatPermissions(p plugin.Permissions) string {
	return fmt.Sprintf("  AllowNetwork:     %t\n  AllowExec:        %t\n  AllowFileWrite:   %t\n  AllowReadAnyPath: %t", p.AllowNetwork, p.AllowExec, p.AllowFileWrite, p.AllowReadAnyPath)
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
