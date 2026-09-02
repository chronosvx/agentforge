package ui

import (
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/contextx"
	"github.com/chronosvx/agentforge/internal/i18n"
)

// handleCompactCommand 压缩当前会话中除最后一条外的旧消息为一条 system 摘要。
func (m *Model) handleCompactCommand() {
	if len(m.messages) <= 1 {
		m.status = i18n.T("status.compact_empty")
		return
	}

	old := m.messages[:len(m.messages)-1]
	newest := m.messages[len(m.messages)-1]

	summary := contextx.SummarizeMessages(old, 2000)
	m.messages = []client.Message{
		summary.AsMessage("system"),
		newest,
	}
	m.dirty = true
	m.followBottom = true
	m.syncWindowFromMessages()
	m.persist()
	m.status = i18n.T("status.compacted", len(old))
}
