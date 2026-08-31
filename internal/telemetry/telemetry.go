// Package telemetry 提供可选的本地遥测能力。
//
// 遥测默认关闭。可通过环境变量 DSH_TELEMETRY=1 或调用 SetEnabled(true)（对应
// --telemetry）开启。开启后 Record 仅将事件以 JSON Lines 写入
// <data-dir>/telemetry.log，不会联网，也不会收集敏感信息；所有字段写入前会
// 经过 internal/security.RedactMap 脱敏。
package telemetry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chronosvx/agentforge/internal/security"
)

const (
	// EnvEnable 是开启本地遥测的环境变量名。
	EnvEnable = "DSH_TELEMETRY"
	// DefaultDataDir 是未显式设置数据目录时的默认目录。
	DefaultDataDir = "~/.agentforge"
	// LogFileName 是遥测日志文件名。
	LogFileName = "telemetry.log"
)

// defaultTelemetry 保存进程级遥测配置。默认关闭。
var defaultTelemetry = telemetryState{}

type telemetryState struct {
	enabled bool
	dataDir string
}

// Enabled 报告遥测是否开启。
//
// 开启条件为环境变量 DSH_TELEMETRY=1，或通过 --telemetry / SetEnabled(true)
// 显式开启；两者任一成立即开启。
func Enabled() bool {
	return os.Getenv(EnvEnable) == "1" || defaultTelemetry.enabled
}

// SetEnabled 显式设置遥测开关。命令行 --telemetry 解析后应调用本函数。
func SetEnabled(enabled bool) {
	defaultTelemetry.enabled = enabled
}

// SetDataDir 设置遥测日志的数据目录，并确保目录存在。
func SetDataDir(dir string) error {
	if dir == "" {
		return errors.New("telemetry: data dir is empty")
	}
	expanded, err := expandHome(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(expanded, 0o700); err != nil {
		return fmt.Errorf("telemetry: create data dir: %w", err)
	}
	defaultTelemetry.dataDir = expanded
	return nil
}

// Path 返回当前遥测日志文件路径；未设置数据目录时返回空字符串。
func Path() string {
	dir := defaultTelemetry.dataDir
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, LogFileName)
}

// Record 记录一条本地遥测事件。
//
// 遥测未开启时本函数不做任何事并返回 nil。fields 会在写入前深拷贝并脱敏，
// 不会修改调用方传入的 map。
func Record(event string, fields map[string]any) error {
	if !Enabled() {
		return nil
	}

	dir := defaultTelemetry.dataDir
	if dir == "" {
		return errors.New("telemetry: data dir is empty; call SetDataDir first")
	}

	redacted := copyMap(fields)
	security.RedactMap(redacted)

	record := map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339Nano),
		"event":  event,
		"fields": redacted,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("telemetry: encode: %w", err)
	}
	line = append(line, '\n')

	path := filepath.Join(dir, LogFileName)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("telemetry: open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("telemetry: write: %w", err)
	}
	return nil
}

func expandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("telemetry: resolve home: %w", err)
		}
		return home, nil
	}
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("telemetry: resolve home: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = copyValue(v)
	}
	return out
}

func copyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return copyMap(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = copyValue(item)
		}
		return out
	default:
		return val
	}
}
