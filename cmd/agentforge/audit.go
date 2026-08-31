package main

import (
	"github.com/chronosvx/agentforge/internal/enterprise"
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

// newSIEMSink 创建本地 SIEM JSON Lines 日志写入器；创建失败时返回 nil（不阻断主流程）。
func newSIEMSink(dataDir string) *enterprise.LogSink {
	if dataDir == "" {
		return nil
	}
	sink, err := enterprise.NewLogSink(dataDir)
	if err != nil {
		return nil
	}
	return sink
}

// emitSIEM 写入一条 SIEM 事件；sink 为 nil 时忽略。
func emitSIEM(sink *enterprise.LogSink, user, action, resource, result string) {
	if sink == nil {
		return
	}
	_ = sink.Emit(enterprise.SIEMEvent{
		User:     user,
		Action:   action,
		Resource: resource,
		Result:   result,
	})
}
