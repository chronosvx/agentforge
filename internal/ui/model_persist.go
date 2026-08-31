package ui

import (
	"context"
	"github.com/chronosvx/agentforge/internal/session"
	"time"
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
	entry := session.Entry{
		ID:        m.sessionID,
		Workspace: m.cfg.Workspace,
		Title:     m.cfg.Workspace,
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
