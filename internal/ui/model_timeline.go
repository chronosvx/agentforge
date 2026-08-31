package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/contextx"
	"github.com/chronosvx/agentforge/internal/timeline"
)

// handleCheckpointCommand 处理 /checkpoint 命令。
func (m *Model) handleCheckpointCommand(cmd string) {
	name := strings.TrimSpace(strings.TrimPrefix(cmd, "/checkpoint"))
	if name == "" || m.checkpoints == nil || m.window == nil {
		m.status = "usage: /checkpoint <name>"
		return
	}
	cp := m.window.Checkpoint(name)
	if err := m.checkpoints.Save(cp); err != nil {
		m.status = "checkpoint error: " + err.Error()
		return
	}
	if m.timeline == nil {
		m.timeline = timeline.New()
	}
	data, err := json.Marshal(m.messages)
	if err != nil {
		m.status = "checkpoint saved: " + name + " (timeline error: " + err.Error() + ")"
		return
	}
	m.timeline.Add(name, data)
	m.status = "checkpoint saved: " + name
}

// handleRewindCommand 处理 /rewind 命令。
func (m *Model) handleRewindCommand(cmd string) {
	name := strings.TrimSpace(strings.TrimPrefix(cmd, "/rewind"))
	if name == "" {
		m.status = "usage: /rewind <name>"
		return
	}
	if m.checkpoints != nil {
		if cp, err := m.checkpoints.Load(name); err == nil {
			m.messages = append([]client.Message(nil), cp.Messages...)
			m.dirty = true
			m.followBottom = true
			if m.window == nil {
				m.window = contextx.NewWindow()
			}
			m.window.Rewind(cp)
			m.persist()
			m.status = "rewound to checkpoint: " + name
			return
		}
	}
	if m.timeline == nil {
		m.timeline = timeline.New()
	}
	snap, err := m.timeline.Rewind(name)
	if err != nil {
		m.status = "rewind error: " + err.Error()
		return
	}
	var msgs []client.Message
	if err := json.Unmarshal(snap.Data, &msgs); err != nil {
		m.status = "rewind error: " + err.Error()
		return
	}
	m.messages = msgs
	m.dirty = true
	m.followBottom = true
	if m.window == nil {
		m.window = contextx.NewWindow()
	}
	m.window.Rewind(contextx.Checkpoint{ID: name, Messages: msgs})
	m.persist()
	m.status = "rewound to timeline: " + name
}

// handleTimelineCommand 处理 /timeline 命令。
func (m *Model) handleTimelineCommand() {
	if m.timeline == nil {
		m.timeline = timeline.New()
	}
	snapshots := m.timeline.List()
	if len(snapshots) == 0 {
		m.status = "timeline: empty"
		return
	}
	lines := make([]string, 0, len(snapshots))
	for _, s := range snapshots {
		lines = append(lines, fmt.Sprintf("%s(%s)", s.ID, s.Label))
	}
	m.status = "timeline: " + strings.Join(lines, "; ")
}

// handleForkCommand 处理 /fork 命令。
func (m *Model) handleForkCommand(cmd string) {
	label := strings.TrimSpace(strings.TrimPrefix(cmd, "/fork"))
	if label == "" {
		m.status = "usage: /fork <label>"
		return
	}
	data, err := json.Marshal(m.messages)
	if err != nil {
		m.status = "fork error: " + err.Error()
		return
	}
	if m.timeline == nil {
		m.timeline = timeline.New()
	}
	snap := m.timeline.Fork(label, data)
	m.status = "forked timeline: " + snap.ID
}
