package ui

import (
	"context"
	"strings"

	"github.com/chronosvx/agentforge/internal/memory"
)

// memoryProject 返回当前项目记忆使用的 project 标识。
func (m *Model) memoryProject() string {
	if m.cfg != nil && strings.TrimSpace(m.cfg.Workspace) != "" {
		return strings.TrimSpace(m.cfg.Workspace)
	}
	return "default"
}

// handleRememberCommand 处理 /remember 命令。
func (m *Model) handleRememberCommand(cmd string) {
	rest := strings.TrimSpace(strings.TrimPrefix(cmd, "/remember"))
	key, value, ok := strings.Cut(rest, " ")
	if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		m.status = "usage: /remember <key> <value>"
		return
	}
	if m.memoryStore == nil {
		m.status = "memory store unavailable"
		return
	}
	entry := memory.Entry{
		Project: m.memoryProject(),
		Key:     strings.TrimSpace(key),
		Content: strings.TrimSpace(value),
	}
	if err := m.memoryStore.Save(context.Background(), entry); err != nil {
		m.status = "remember error: " + err.Error()
	} else {
		m.status = "remembered: " + entry.Key
	}
}

// handleForgetCommand 处理 /forget 命令。
func (m *Model) handleForgetCommand(cmd string) {
	key := strings.TrimSpace(strings.TrimPrefix(cmd, "/forget"))
	if key == "" {
		m.status = "usage: /forget <key>"
		return
	}
	if m.memoryStore == nil {
		m.status = "memory store unavailable"
		return
	}
	if err := m.memoryStore.Delete(context.Background(), m.memoryProject(), key); err != nil {
		m.status = "forget error: " + err.Error()
	} else {
		m.status = "forgot: " + key
	}
}

// handleMemoryCommand 处理 /memory 命令。
func (m *Model) handleMemoryCommand(cmd string) {
	query := strings.TrimSpace(strings.TrimPrefix(cmd, "/memory"))
	if m.memoryStore == nil {
		m.status = "memory store unavailable"
		return
	}
	project := m.memoryProject()
	if query == "" {
		entries, err := m.memoryStore.List(context.Background(), project)
		if err != nil {
			m.status = "memory error: " + err.Error()
			return
		}
		lines := make([]string, 0, len(entries))
		for _, e := range entries {
			lines = append(lines, e.Key+": "+truncateText(e.Content, 40))
		}
		if len(lines) == 0 {
			m.status = "memory: empty"
		} else {
			m.status = "memory: " + strings.Join(lines, "; ")
		}
		return
	}
	results := memory.NewSearcher(m.memoryStore).Search(project, query, 20)
	lines := make([]string, 0, len(results))
	for _, r := range results {
		lines = append(lines, r.Key+": "+truncateText(r.Content, 40))
	}
	if len(lines) == 0 {
		m.status = "memory: no results"
	} else {
		m.status = "memory: " + strings.Join(lines, "; ")
	}
}
