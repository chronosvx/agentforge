package main

import (
	"github.com/chronosvx/agentforge/internal/security"
)

// newAuditLogger 创建审计日志；enabled=false 时返回 nil（不记录）。
func newAuditLogger(dataDir string, enabled bool) *security.AuditLog {
	if !enabled {
		return nil
	}
	l, err := security.NewAuditLog(dataDir)
	if err != nil {
		return nil
	}
	return l
}

// audit 记录一条审计事件；日志器为 nil 时忽略。
func audit(l *security.AuditLog, event string, fields map[string]any) {
	if l == nil {
		return
	}
	_ = l.Append(event, fields)
}
