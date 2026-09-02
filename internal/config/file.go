package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// LoadFile 解析配置文件，支持 JSON 对象和 key=value 行格式。
func LoadFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := map[string]string{}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "{") {
		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", path, err)
		}
		for k, v := range raw {
			out[strings.ToLower(k)] = fmt.Sprintf("%v", v)
		}
		return out, nil
	}

	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexAny(line, "=:"); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			v = strings.Trim(v, `"'`)
			out[strings.ToLower(k)] = v
		}
	}
	return out, sc.Err()
}

// projectConfigName 是仓库/项目目录中可能存在的项目级配置文件名。
//
// 项目级配置默认不再自动加载；即使用户显式通过 --config 指定它，也只允许
// 修改非敏感字段，防止仓库投毒修改连接目标、关闭审计或泄露凭据。
const projectConfigName = ".agentforge.json"

// projectRestrictedConfigKeys 列出项目配置中禁止应用的敏感/安全关键字段。
var projectRestrictedConfigKeys = map[string]bool{
	"api-url":             true,
	"api-token":           true,
	"remote":              true,
	"endpoint":            true,
	"data-dir":            true,
	"provider-key":        true,
	"provider-base-url":   true,
	"secret-key":          true,
	"confirm-data-export": true,
	"no-audit":            true,
	// 该开关用于显式放行非 loopback 明文连接；不能让项目配置文件替用户放行。
	"allow-insecure-http": true,
	// 插件可执行外部代码，不能让项目配置文件替用户自动启用。
	"plugins-enabled": true,
	"plugin-enabled":  true,
	"plugins":         true,
}

// isProjectConfigPath 报告 path 是否为项目级 .agentforge.json。
func isProjectConfigPath(path string) bool {
	return filepath.Base(path) == projectConfigName
}

// sanitizeProjectConfig 返回移除了所有项目级敏感/安全关键字段后的配置。
// 调用方不应修改原 map。
func sanitizeProjectConfig(kv map[string]string) map[string]string {
	out := make(map[string]string, len(kv))
	for k, v := range kv {
		if !projectRestrictedConfigKeys[k] {
			out[k] = v
		}
	}
	return out
}

// DefaultPaths 返回默认自动加载的配置路径。
//
// 只包含用户级配置；不再自动加载项目目录中的 .agentforge.json，以避免仓库
// 投毒、凭据外发和审计关闭等风险。项目级配置仍可通过显式 --config 使用，
// 但敏感字段会被过滤。
func DefaultPaths() []string {
	home, _ := os.UserHomeDir()
	user := filepath.Join(home, ".agentforge", "config.json")
	return []string{user}
}

// applyConfig 将配置 map 应用到未由 CLI 显式设置的字段。
func applyConfig(cfg *Config, kv map[string]string, setFlags map[string]bool) {
	set := func(flag string) bool { return setFlags[flag] }
	if !set("api-url") && kv["api-url"] != "" {
		cfg.APIURL = kv["api-url"]
	}
	if !set("api-token") && kv["api-token"] != "" {
		cfg.APIToken = kv["api-token"]
	}
	if !set("remote") && kv["remote"] != "" {
		cfg.Remote, _ = strconv.ParseBool(kv["remote"])
	}
	if !set("endpoint") && kv["endpoint"] != "" {
		cfg.Endpoint = kv["endpoint"]
	}
	if !set("workspace") && kv["workspace"] != "" {
		cfg.Workspace = kv["workspace"]
	}
	if !set("resume") && kv["resume"] != "" {
		cfg.SessionID = kv["resume"]
	}
	if !set("data-dir") && kv["data-dir"] != "" {
		cfg.DataDir = kv["data-dir"]
	}
	if !set("mock") && kv["mock"] != "" {
		cfg.Mock, _ = strconv.ParseBool(kv["mock"])
	}
	if !set("cost-limit") && kv["cost-limit"] != "" {
		cfg.CostLimit, _ = strconv.ParseFloat(kv["cost-limit"], 64)
	}
	if !set("theme") && kv["theme"] != "" {
		cfg.Theme = kv["theme"]
	}
	if !set("lang") && kv["lang"] != "" {
		cfg.Lang = kv["lang"]
	}
	if !set("telemetry") && kv["telemetry"] != "" {
		cfg.Telemetry, _ = strconv.ParseBool(kv["telemetry"])
	}
	if !set("provider") && kv["provider"] != "" {
		cfg.Provider = kv["provider"]
	}
	if !set("provider-key") && kv["provider-key"] != "" {
		cfg.ProviderKey = kv["provider-key"]
	}
	if !set("provider-base-url") && kv["provider-base-url"] != "" {
		cfg.ProviderBaseURL = kv["provider-base-url"]
	}
	if !set("model") && kv["model"] != "" {
		cfg.Model = kv["model"]
	}
	if !set("keychain") && kv["keychain"] != "" {
		cfg.Keychain, _ = strconv.ParseBool(kv["keychain"])
	}
	if !set("secret-key") && kv["secret-key"] != "" {
		cfg.SecretKey = kv["secret-key"]
	}
	if !set("confirm-data-export") && kv["confirm-data-export"] != "" {
		cfg.ConfirmDataExport, _ = strconv.ParseBool(kv["confirm-data-export"])
	}
	if !set("no-audit") && kv["no-audit"] != "" {
		cfg.NoAudit, _ = strconv.ParseBool(kv["no-audit"])
	}
	if !set("no-color") && kv["no-color"] != "" {
		cfg.NoColor, _ = strconv.ParseBool(kv["no-color"])
	}
	if !set("allow-insecure-http") && kv["allow-insecure-http"] != "" {
		cfg.AllowInsecureHTTP, _ = strconv.ParseBool(kv["allow-insecure-http"])
	}
	if !set("plugins-enabled") {
		switch {
		case kv["plugins-enabled"] != "":
			cfg.PluginsEnabled, _ = strconv.ParseBool(kv["plugins-enabled"])
		case kv["plugin-enabled"] != "":
			cfg.PluginsEnabled, _ = strconv.ParseBool(kv["plugin-enabled"])
		case kv["plugins"] != "":
			cfg.PluginsEnabled, _ = strconv.ParseBool(kv["plugins"])
		}
	}
	if !set("user") && kv["user"] != "" {
		cfg.User = kv["user"]
	}
	if !set("roles") && kv["roles"] != "" {
		cfg.Roles = kv["roles"]
	}
}
