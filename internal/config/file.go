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

// DefaultPaths 返回用户配置与项目配置路径。
func DefaultPaths() []string {
	home, _ := os.UserHomeDir()
	user := filepath.Join(home, ".agentforge", "config.json")
	project := ".agentforge.json"
	return []string{user, project}
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
	if !set("user") && kv["user"] != "" {
		cfg.User = kv["user"]
	}
	if !set("roles") && kv["roles"] != "" {
		cfg.Roles = kv["roles"]
	}
}
