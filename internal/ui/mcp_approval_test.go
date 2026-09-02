package ui

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/mcp"
)

func TestMCPCallCmdOpensApprovalPanel(t *testing.T) {
	m := Model{
		mcpPolicy: mcp.NewPolicy(mcp.ModeConfirm),
		mcpAudit:  mcp.NewAuditLog(),
	}

	cmd := m.mcpCallCmd("files.read", map[string]any{"path": "/tmp"})
	if cmd != nil {
		t.Fatalf("expected nil tea.Cmd when approval is required, got %T", cmd)
	}
	if !m.mcpPendingOpen {
		t.Fatal("expected mcpPendingOpen to be true when approval is required")
	}
	if m.pendingMCPName != "files.read" {
		t.Fatalf("pendingMCPName = %q, want files.read", m.pendingMCPName)
	}
	if !strings.Contains(m.status, i18n.T("mcp.approval_pending")) {
		t.Fatalf("status = %q, want it to contain %q", m.status, i18n.T("mcp.approval_pending"))
	}
}

func TestHandleMCPApprovalKeyAllowOnce(t *testing.T) {
	m := pendingMCPModel()
	handled, cmd := m.handleMCPApprovalKey("a")
	if !handled {
		t.Fatal("expected a to be handled")
	}
	if cmd == nil {
		t.Fatal("expected allow-once to return an execute cmd")
	}
	assertMCPClosed(t, &m)
	if m.status != i18n.T("mcp.approved_once") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("mcp.approved_once"))
	}
}

func TestHandleMCPApprovalKeyAlwaysAllow(t *testing.T) {
	m := pendingMCPModel()
	handled, cmd := m.handleMCPApprovalKey("s")
	if !handled {
		t.Fatal("expected s to be handled")
	}
	if cmd == nil {
		t.Fatal("expected always-allow to return an execute cmd")
	}
	if got := m.mcpPolicy.Mode("files.read"); got != mcp.ModeAuto {
		t.Fatalf("policy mode after s = %q, want %q", got, mcp.ModeAuto)
	}
	assertMCPClosed(t, &m)
	if m.status != i18n.T("mcp.always_allowed") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("mcp.always_allowed"))
	}
}

func TestHandleMCPApprovalKeyDenyOnce(t *testing.T) {
	m := pendingMCPModel()
	handled, cmd := m.handleMCPApprovalKey("d")
	if !handled {
		t.Fatal("expected d to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected deny-once to return nil cmd, got %T", cmd)
	}
	assertMCPClosed(t, &m)
	if m.status != i18n.T("mcp.denied_once") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("mcp.denied_once"))
	}
}

func TestHandleMCPApprovalKeyDenyAndRemember(t *testing.T) {
	m := pendingMCPModel()
	handled, cmd := m.handleMCPApprovalKey("n")
	if !handled {
		t.Fatal("expected n to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected deny-and-remember to return nil cmd, got %T", cmd)
	}
	if got := m.mcpPolicy.Mode("files.read"); got != mcp.ModeDeny {
		t.Fatalf("policy mode after n = %q, want %q", got, mcp.ModeDeny)
	}
	assertMCPClosed(t, &m)
	if m.status != i18n.T("mcp.denied_and_remembered") {
		t.Fatalf("status = %q, want %q", m.status, i18n.T("mcp.denied_and_remembered"))
	}
}

func TestHandleMCPApprovalKeyCancel(t *testing.T) {
	for _, key := range []string{"esc", "q"} {
		m := pendingMCPModel()
		handled, cmd := m.handleMCPApprovalKey(key)
		if !handled {
			t.Fatalf("expected %s to be handled", key)
		}
		if cmd != nil {
			t.Fatalf("expected %s to return nil cmd, got %T", key, cmd)
		}
		assertMCPClosed(t, &m)
	}
}

func TestRenderMCPApprovalShowsToolAndHint(t *testing.T) {
	m := pendingMCPModel()
	got := m.renderMCPApproval()
	if !strings.Contains(got, "files.read") {
		t.Fatalf("renderMCPApproval missing tool name:\n%s", got)
	}
	if !strings.Contains(got, i18n.T("mcp.approval_hint")) {
		t.Fatalf("renderMCPApproval missing approval hint:\n%s", got)
	}
	if !strings.Contains(got, i18n.T("mcp.approval_title")) {
		t.Fatalf("renderMCPApproval missing approval title:\n%s", got)
	}
	if !strings.Contains(got, string(mcp.ModeConfirm)) {
		t.Fatalf("renderMCPApproval missing current policy:\n%s", got)
	}
}

func pendingMCPModel() Model {
	return Model{
		mcpPolicy:      mcp.NewPolicy(mcp.ModeConfirm),
		mcpAudit:       mcp.NewAuditLog(),
		mcpPendingOpen: true,
		pendingMCPName: "files.read",
		pendingMCPArgs: map[string]any{"path": "/tmp"},
	}
}

func assertMCPClosed(t *testing.T, m *Model) {
	t.Helper()
	if m.mcpPendingOpen {
		t.Error("expected mcpPendingOpen to be false")
	}
	if m.pendingMCPName != "" {
		t.Errorf("expected pendingMCPName to be cleared, got %q", m.pendingMCPName)
	}
	if m.pendingMCPArgs != nil {
		t.Errorf("expected pendingMCPArgs to be nil, got %#v", m.pendingMCPArgs)
	}
}
