package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// defaultKeybindings 返回内置默认键位。
func defaultKeybindings() map[string]string {
	return map[string]string{
		"help":    "ctrl+/",
		"palette": "ctrl+p",
		"history": "ctrl+r",
	}
}

// loadKeybindings 从 <dataDir>/keybindings.json 读取键位覆盖。
// 文件不存在或解析失败时返回默认键位；文件中的空值会被忽略。
func loadKeybindings(dataDir string) map[string]string {
	bindings := defaultKeybindings()
	if dataDir == "" {
		return bindings
	}

	path := filepath.Join(dataDir, "keybindings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return bindings
	}

	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return bindings
	}
	for action, key := range overrides {
		if key != "" {
			bindings[action] = key
		}
	}
	return bindings
}

// loadKeybindings 方法版本会把读取结果写回 Model.keybindings。
func (m *Model) loadKeybindings(dataDir string) map[string]string {
	m.keybindings = loadKeybindings(dataDir)
	return m.keybindings
}

// key 返回指定动作当前使用的键位。
func (m Model) key(action string) string {
	if m.keybindings != nil {
		if key, ok := m.keybindings[action]; ok && key != "" {
			return key
		}
	}
	return defaultKeybindings()[action]
}

// isKey 判断 key 是否匹配动作的当前键位，同时兼容默认 Ctrl+/ 的终端别名。
func (m Model) isKey(action, key string) bool {
	if key == m.key(action) {
		return true
	}
	// 部分终端会把 Ctrl+/ 上报为 Ctrl+_ 或 Ctrl+?。
	if action == "help" && m.key(action) == "ctrl+/" && (key == "ctrl+_" || key == "ctrl+?") {
		return true
	}
	return false
}
