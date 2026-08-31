package security

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditLog 将审计事件以 JSON Lines 形式写入 <data-dir>/audit.log。
//
// 所有字段在写入前会经过 RedactMap 脱敏，避免 Key/Token 等敏感信息落盘。
type AuditLog struct {
	// DataDir 是审计日志所在的数据目录；audit.log 会写入该目录。
	DataDir string
}

// NewAuditLog 创建审计日志，并确保 dataDir 目录存在。
func NewAuditLog(dataDir string) (*AuditLog, error) {
	if dataDir == "" {
		return nil, errors.New("security: audit log data dir is empty")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("security: create audit data dir: %w", err)
	}
	return &AuditLog{DataDir: dataDir}, nil
}

// Path 返回审计日志文件路径。
func (a *AuditLog) Path() string {
	if a == nil || a.DataDir == "" {
		return ""
	}
	return filepath.Join(a.DataDir, "audit.log")
}

// Append 追加一条审计日志。
//
// 记录格式为 JSON Lines：
//
//	{"time":"...","event":"...","fields":{...}}
//
// fields 会在写入前深拷贝并脱敏，不会修改调用方传入的 map。
func (a *AuditLog) Append(event string, fields map[string]any) error {
	if a == nil || a.DataDir == "" {
		return errors.New("security: audit log data dir is empty")
	}
	path := filepath.Join(a.DataDir, "audit.log")
	redacted := copyMap(fields)
	RedactMap(redacted)

	record := map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339Nano),
		"event":  event,
		"fields": redacted,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("security: audit encode: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("security: audit open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("security: audit write: %w", err)
	}
	return nil
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
