package ui

import (
	"context"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/mcp"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/skill"
)

type workspaceLoadedMsg struct {
	tabs []session.Entry
}

type connectedMsg struct {
	snapshot *client.Snapshot
	events   <-chan client.Event
	cancel   context.CancelFunc
	epoch    int
}

type eventMsg struct {
	event     client.Event
	channelID string
}

type disconnectedMsg struct {
	epoch int
}

type reconnectMsg struct{}

type editorDoneMsg struct {
	content string
	err     error
}

type skillInstallMsg struct {
	entries []skill.Entry
	err     error
}

type skillSyncMsg struct {
	entries []skill.Entry
	err     error
}

type pluginResultMsg struct {
	output string
	err    error
}

type mcpConnectMsg struct {
	url    string
	client *mcp.Client
	tools  []mcp.Tool
	err    error
}

type mcpCallMsg struct {
	name             string
	result           any
	err              error
	requiresApproval bool
}

type multiAgentEventMsg struct {
	Event multiagent.Event
}

type multiAgentDoneMsg struct {
	Results []multiagent.Result
	Err     error
}

type sentMsg struct {
	text string
}

type switchSessionMsg struct {
	id string
}

type errMsg struct {
	err error
}
