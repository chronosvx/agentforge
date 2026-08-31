package enterprise

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SIEMEvent 表示一条安全信息与事件管理（SIEM）事件。
type SIEMEvent struct {
	Time     time.Time `json:"time"`
	User     string    `json:"user"`
	Action   string    `json:"action"`
	Resource string    `json:"resource"`
	Result   string    `json:"result"`
}

// SIEMSink 是 SIEM 事件输出接口。
//
// 当前 LogSink 写入本地 JSON Lines；后续可接入远程 SIEM/日志平台。
type SIEMSink interface {
	// Emit 发送一条 SIEM 事件。
	Emit(event SIEMEvent) error
}

// LogSink 将 SIEM 事件以 JSON Lines 格式写入 <data-dir>/siem.log。
type LogSink struct {
	dataDir string

	mu sync.Mutex
}

// NewLogSink 创建 SIEM 日志写入器，并确保 dataDir 目录存在。
func NewLogSink(dataDir string) (*LogSink, error) {
	if dataDir == "" {
		return nil, errors.New("enterprise: siem log data dir is empty")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("enterprise: create siem data dir: %w", err)
	}
	return &LogSink{dataDir: dataDir}, nil
}

// Path 返回 SIEM 日志文件路径。
func (s *LogSink) Path() string {
	if s == nil || s.dataDir == "" {
		return ""
	}
	return filepath.Join(s.dataDir, "siem.log")
}

// Emit 将事件追加写入 siem.log。
//
// 每行是一个 JSON 对象；写入使用追加模式，文件权限为 0600。
// 如果事件 Time 为零值，则自动使用当前 UTC 时间，避免产生无意义时间戳。
func (s *LogSink) Emit(event SIEMEvent) error {
	if s == nil || s.dataDir == "" {
		return errors.New("enterprise: siem log data dir is empty")
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("enterprise: siem encode: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.Path()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("enterprise: siem open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("enterprise: siem write: %w", err)
	}
	return nil
}
