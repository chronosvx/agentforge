package ui

import (
	"context"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/session"
)

func (m *Model) persist() {
	if m.store == nil || m.sessionID == "" {
		return
	}
	m.persistDirty = true
	if time.Since(m.lastPersistAt) >= persistInterval {
		m.persistNow()
	}
}

func (m *Model) flushPersist() {
	m.persistNow()
	if m.asyncStore != nil {
		_ = m.asyncStore.Flush(context.Background())
	}
}

func (m Model) FlushPersist() {
	m.flushPersist()
}

func (m Model) ClosePersist() error {
	if m.asyncStore == nil {
		return nil
	}
	return m.asyncStore.Close()
}

func (m *Model) persistNow() {
	if !m.persistDirty || m.store == nil || m.sessionID == "" {
		return
	}
	now := time.Now()
	workspace := ""
	if m.cfg != nil {
		workspace = m.cfg.Workspace
	}
	title := workspace
	if m.sessionTitle != "" {
		title = m.sessionTitle
	} else {
		for _, msg := range m.messages {
			if msg.Role != "user" {
				continue
			}
			candidate := strings.TrimSpace(msg.Content)
			if candidate == "" {
				break
			}
			if runes := []rune(candidate); len(runes) > 40 {
				candidate = string(runes[:40])
			}
			title = candidate
			m.sessionTitle = candidate
			break
		}
	}
	entry := session.Entry{
		ID:        m.sessionID,
		Workspace: workspace,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  m.messages,
		Context:   m.context,
		Cost:      m.cost,
		Diffs:     m.diffs,
		Todos:     m.todos,
		Tools:     m.tools,
	}
	if m.asyncStore != nil {
		if err := m.asyncStore.Save(entry); err != nil {
			m.persistDirty = true
			return
		}
		m.persistDirty = false
		m.lastPersistAt = now
		return
	}
	if err := m.store.Save(entry); err != nil {
		// 保留脏标记，等待下一次节流重试。
		m.persistDirty = true
		return
	}
	m.persistDirty = false
	m.lastPersistAt = now
}

// 消息类型定义
