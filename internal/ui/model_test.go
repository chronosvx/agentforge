package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/i18n"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"github.com/chronosvx/agentforge/internal/plugin"
	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/skill"
)

type fakeBackend struct {
	invokes []string
	subCtx  context.Context
}

func (f *fakeBackend) Open(ctx context.Context, workspace, sessionID string, options map[string]any) (*client.Snapshot, error) {
	return &client.Snapshot{ChannelID: sessionID}, nil
}

func (f *fakeBackend) Subscribe(ctx context.Context, channelID string, after int64) (<-chan client.Event, error) {
	f.subCtx = ctx
	return make(chan client.Event), nil
}

func (f *fakeBackend) Invoke(ctx context.Context, channelID, method string, args []any) (any, error) {
	f.invokes = append(f.invokes, channelID)
	return nil, nil
}

func (f *fakeBackend) Close(ctx context.Context, channelID string) error {
	return nil
}

func (f *fakeBackend) CloseConn() error {
	return nil
}

type auditEntry struct {
	event  string
	fields map[string]any
}

type fakeAudit struct {
	entries []auditEntry
}

func (f *fakeAudit) record(event string, fields map[string]any) {
	f.entries = append(f.entries, auditEntry{event: event, fields: fields})
}

func (f *fakeAudit) eventNames() []string {
	names := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		names = append(names, e.event)
	}
	return names
}

func TestSendCmdTo(t *testing.T) {
	fb := &fakeBackend{}
	m := Model{cl: fb, channelID: "s1"}
	cmd := m.sendCmdTo("hello", "s2")
	msg := cmd()
	if _, ok := msg.(sentMsg); !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if len(fb.invokes) != 1 || fb.invokes[0] != "s2" {
		t.Fatalf("unexpected invokes: %#v", fb.invokes)
	}
}

func TestHandleCommandTogglesContextPanel(t *testing.T) {
	m := Model{}
	if m.panel != panelNone {
		t.Fatalf("expected panelNone, got %v", m.panel)
	}
	m.handleCommand("/context")
	if m.panel != panelContext {
		t.Fatalf("expected panelContext, got %v", m.panel)
	}
	m.handleCommand("/context")
	if m.panel != panelNone {
		t.Fatalf("expected panelNone after toggle, got %v", m.panel)
	}
}

func TestHandleCommandTogglesDiffPanel(t *testing.T) {
	m := Model{}
	m.handleCommand("/diff")
	if m.panel != panelDiff {
		t.Fatalf("expected panelDiff, got %v", m.panel)
	}
	m.handleCommand("/diff")
	if m.panel != panelNone {
		t.Fatalf("expected panelNone after toggle, got %v", m.panel)
	}
}

func TestHandleCommandTogglesTodosPanel(t *testing.T) {
	m := Model{}
	m.handleCommand("/todos")
	if m.panel != panelTodos {
		t.Fatalf("expected panelTodos, got %v", m.panel)
	}
	m.handleCommand("/todos")
	if m.panel != panelNone {
		t.Fatalf("expected panelNone after toggle, got %v", m.panel)
	}
}

func TestLimitTabs(t *testing.T) {
	entries := make([]session.Entry, 10)
	for i := range entries {
		entries[i].ID = fmt.Sprintf("s-%d", i)
	}
	got := limitTabs(entries, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 tabs, got %d", len(got))
	}
}

func TestHandleDiffKey(t *testing.T) {
	m := Model{diffs: []any{map[string]any{}, map[string]any{}}}
	if !m.handleDiffKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.diffCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.diffCursor)
	}
	if !m.handleDiffKey("a") {
		t.Fatal("expected a to be consumed")
	}
	if m.diffDecisions[1] != "accepted" {
		t.Fatalf("expected accepted decision, got %#v", m.diffDecisions)
	}
	if !m.handleDiffKey("k") {
		t.Fatal("expected k to be consumed")
	}
	if m.diffCursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.diffCursor)
	}
}

func TestSavePatch(t *testing.T) {
	dir := t.TempDir()
	m := Model{
		cfg: &config.Config{DataDir: dir},
		diffs: []any{
			map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"},
			map[string]any{"file": "b.txt", "diff": "diff --git a/b.txt b/b.txt\n-old2\n+new2"},
		},
		diffDecisions: map[int]string{0: "accepted", 1: "rejected"},
		sessionID:     "sess/1",
	}
	path, err := m.savePatch()
	if err != nil {
		t.Fatalf("savePatch: %v", err)
	}
	if !strings.Contains(path, "patches") {
		t.Fatalf("unexpected patch path: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if !strings.Contains(string(data), "a.txt") || strings.Contains(string(data), "b.txt") {
		t.Fatalf("unexpected patch content: %s", data)
	}
	if !strings.HasPrefix(filepath.Base(path), "sess_1-") {
		t.Fatalf("unexpected filename sanitization: %s", filepath.Base(path))
	}
}

func TestCheckCostAlert(t *testing.T) {
	m := Model{costLimit: 1, cost: map[string]any{"estimatedYuan": 2.0}}
	m.checkCostAlert()
	if !m.costAlerted {
		t.Fatal("expected costAlerted")
	}
	if !strings.Contains(m.status, "cost limit exceeded") {
		t.Fatalf("unexpected status: %q", m.status)
	}
}

func TestRenderSidebar(t *testing.T) {
	m := Model{tabs: []session.Entry{{ID: "s1"}, {ID: "s2"}}, tabIndex: 0, sessionID: "s1"}
	if m.renderSidebar() == "" {
		t.Fatal("expected non-empty sidebar")
	}
	if !strings.Contains(m.renderSidebar(), "s1") {
		t.Fatal("expected s1 in sidebar")
	}
}

func TestHandleDiffApplyConfirmation(t *testing.T) {
	m := Model{diffs: []any{map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"}}, diffDecisions: map[int]string{0: "accepted"}}
	if !m.handleDiffKey("g") {
		t.Fatal("expected first g to be consumed")
	}
	if !m.pendingApply {
		t.Fatal("expected pendingApply true after first g")
	}
	if !m.handleDiffKey("esc") {
		t.Fatal("expected esc to be consumed")
	}
	if m.pendingApply {
		t.Fatal("expected pendingApply false after esc")
	}
}

func TestToggleRightPanel(t *testing.T) {
	m := Model{}
	m.handleCommand("/right")
	if !m.rightPanel {
		t.Fatal("expected rightPanel true")
	}
	m.handleCommand("/right")
	if m.rightPanel {
		t.Fatal("expected rightPanel false")
	}
}

func TestRenderRightPanel(t *testing.T) {
	m := Model{context: map[string]any{"totalTokens": 10}, cost: map[string]any{"estimatedYuan": 1.5}, diffs: []any{map[string]any{}}, todos: []any{map[string]any{}}}
	if m.renderRightPanel() == "" {
		t.Fatal("expected non-empty right panel")
	}
}

func TestToggleSplitView(t *testing.T) {
	m := Model{tabs: []session.Entry{{ID: "s1"}, {ID: "s2"}}, sessionID: "s1"}
	m.handleCommand("/split")
	if !m.splitEnabled || m.splitSessionID != "s2" {
		t.Fatalf("unexpected split state: enabled=%v id=%q", m.splitEnabled, m.splitSessionID)
	}
	m.handleCommand("/split")
	if m.splitEnabled {
		t.Fatal("expected split disabled after toggle")
	}
}

func TestBackupRestore(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Model{cfg: &config.Config{DataDir: dir}, sessionID: "s1"}
	backup, err := m.backupFiles([]string{"a.txt"}, repo)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.restoreBackup(backup, repo); err != nil {
		t.Fatalf("restore: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("unexpected restored content: %q", data)
	}
}

func TestAcceptedDiffFiles(t *testing.T) {
	files := acceptedDiffFiles([]any{
		map[string]any{"file": "a.txt"},
		map[string]any{"file": "b.txt"},
		map[string]any{"file": "a.txt"},
	}, map[int]string{0: "accepted", 1: "rejected", 2: "accepted"})
	if len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("unexpected files: %#v", files)
	}
}

func TestHandleSplitKey(t *testing.T) {
	m := Model{tabs: []session.Entry{{ID: "s1"}, {ID: "s2"}, {ID: "s3"}}, splitSessionID: "s1"}
	if !m.handleSplitKey("]") || m.splitSessionID != "s2" {
		t.Fatalf("unexpected after ]: %q", m.splitSessionID)
	}
	if !m.handleSplitKey("[") || m.splitSessionID != "s1" {
		t.Fatalf("unexpected after [: %q", m.splitSessionID)
	}
}

func TestHandleRightPanelKey(t *testing.T) {
	m := Model{}
	if !m.handleRightPanelKey("1") {
		t.Fatal("expected key 1 consumed")
	}
	if m.panel != panelContext {
		t.Fatalf("expected panelContext, got %v", m.panel)
	}
	if !m.handleRightPanelKey("5") {
		t.Fatal("expected key 5 consumed")
	}
	if m.panel != panelSkills {
		t.Fatalf("expected panelSkills, got %v", m.panel)
	}
	if m.handleRightPanelKey("9") {
		t.Fatal("expected unknown key not consumed")
	}
}

func TestAccentColor(t *testing.T) {
	m := Model{cfg: &config.Config{Theme: "catppuccin"}}
	if m.accentColor() != "5" {
		t.Fatalf("unexpected catppuccin accent: %q", m.accentColor())
	}
	m2 := Model{cfg: &config.Config{Theme: "default"}}
	if m2.accentColor() != "6" {
		t.Fatalf("unexpected default accent: %q", m2.accentColor())
	}
}

func TestApplyFileConfirmation(t *testing.T) {
	m := Model{diffs: []any{
		map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"},
		map[string]any{"file": "b.txt", "diff": "diff --git a/b.txt b/b.txt\n-old2\n+new2"},
	}, diffDecisions: map[int]string{0: "accepted", 1: "accepted"}}

	m.handleDiffKey("g")
	if !m.pendingApply || len(m.applyFiles) != 2 {
		t.Fatalf("unexpected apply state: %#v", m)
	}
	if !m.applySelected["a.txt"] || !m.applySelected["b.txt"] {
		t.Fatalf("expected all selected by default: %#v", m.applySelected)
	}
	m.handleApplyKey("j")
	if m.applyCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.applyCursor)
	}
	m.handleApplyKey(" ")
	if m.applySelected["b.txt"] {
		t.Fatal("expected b.txt deselected")
	}
	m.handleApplyKey("esc")
	if m.pendingApply {
		t.Fatal("expected pendingApply false after esc")
	}
}

func TestExportSession(t *testing.T) {
	dir := t.TempDir()
	m := Model{
		cfg:       &config.Config{DataDir: dir},
		sessionID: "s1",
		messages:  []client.Message{{Role: "user", Content: "hello"}},
	}
	paths, err := m.exportSession()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(paths, ".md") || !strings.Contains(paths, ".json") {
		t.Fatalf("unexpected export paths: %s", paths)
	}
	if _, err := os.Stat(filepath.Join(dir, "exports", "s1.md")); err != nil {
		t.Fatalf("md export missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "exports", "s1.json")); err != nil {
		t.Fatalf("json export missing: %v", err)
	}
}

func TestToggleToolsPanel(t *testing.T) {
	m := Model{}
	m.handleCommand("/tools")
	if m.panel != panelTools {
		t.Fatalf("expected panelTools, got %v", m.panel)
	}
	m.handleCommand("/tools")
	if m.panel != panelNone {
		t.Fatalf("expected panelNone after toggle, got %v", m.panel)
	}
}

func TestToggleSkillsPanel(t *testing.T) {
	m := Model{}
	m.handleCommand("/skills")
	if m.panel != panelSkills {
		t.Fatalf("expected panelSkills, got %v", m.panel)
	}
	m.handleCommand("/skills")
	if m.panel != panelNone {
		t.Fatalf("expected panelNone after toggle, got %v", m.panel)
	}
}

func TestHandleSkillKeyNavigationToggleDetail(t *testing.T) {
	m := Model{
		skills: []skill.Entry{
			{Name: "alpha", Description: "Alpha", Source: "local", Enabled: true},
			{Name: "beta", Description: "Beta", Source: "remote", Enabled: false},
		},
	}

	if !m.handleSkillKey("j") {
		t.Fatal("expected j to be consumed")
	}
	if m.skillCursor != 1 {
		t.Fatalf("expected cursor 1, got %d", m.skillCursor)
	}
	if !m.handleSkillKey("k") || m.skillCursor != 0 {
		t.Fatalf("expected cursor back to 0, got %d", m.skillCursor)
	}

	if !m.handleSkillKey(" ") {
		t.Fatal("expected space to be consumed")
	}
	if m.skills[0].Enabled {
		t.Fatal("expected alpha disabled after space")
	}
	if !m.handleSkillKey(" ") || !m.skills[0].Enabled {
		t.Fatal("expected alpha re-enabled after second space")
	}

	if !m.handleSkillKey("enter") {
		t.Fatal("expected enter to be consumed")
	}
	if !m.skillDetail {
		t.Fatal("expected skill detail open")
	}
	if !m.handleSkillKey("esc") {
		t.Fatal("expected esc to be consumed in detail")
	}
	if m.skillDetail {
		t.Fatal("expected skill detail closed after esc")
	}
}

func TestSyncSkillsFromMessagesMergesRemote(t *testing.T) {
	m := Model{
		localSkills: []skill.Entry{{Name: "local", Description: "local", Source: "local", Enabled: true}},
		skills:      []skill.Entry{{Name: "local", Description: "local", Source: "local", Enabled: true}},
		messages: []client.Message{{
			Role:    "user",
			Content: "<available_skills>\n- remote: remote desc\n</available_skills>",
		}},
	}
	m.syncSkillsFromMessages()
	if len(m.skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %#v", len(m.skills), m.skills)
	}
	if m.skills[0].Name != "local" || m.skills[0].Source != "local" {
		t.Fatalf("unexpected first skill: %#v", m.skills[0])
	}
	if m.skills[1].Name != "remote" || m.skills[1].Source != "remote" || m.skills[1].Description != "remote desc" {
		t.Fatalf("unexpected second skill: %#v", m.skills[1])
	}
}

func TestMergeSkillsPreservesEnabledState(t *testing.T) {
	m := Model{
		localSkills: []skill.Entry{{Name: "a", Source: "local", Enabled: true}},
		remoteSkills: []skill.Entry{
			{Name: "a", Source: "remote", Enabled: true},
			{Name: "b", Source: "remote", Enabled: true},
		},
		skills: []skill.Entry{{Name: "a", Source: "local", Enabled: false}},
	}
	m.mergeSkills()
	if len(m.skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %#v", len(m.skills), m.skills)
	}
	if m.skills[0].Enabled {
		t.Fatal("expected preserved disabled state for a")
	}
}

func TestNewLoadsLocalSkills(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "demo.md"), []byte("# Demo\n\nDemo skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := New(&config.Config{DataDir: dir}, &fakeBackend{}, store)
	if len(m.localSkills) != 1 || m.localSkills[0].Name != "demo" {
		t.Fatalf("unexpected local skills: %#v", m.localSkills)
	}
	if len(m.skills) != 1 || m.skills[0].Name != "demo" {
		t.Fatalf("unexpected merged skills: %#v", m.skills)
	}
}

func TestNewCommandSetsPending(t *testing.T) {
	m := Model{}
	m.handleCommand("/new")
	if !m.pendingNew {
		t.Fatal("expected pendingNew true after /new")
	}
}

func TestSyncToolsFromState(t *testing.T) {
	m := Model{}
	m.syncToolsFromState(map[string]any{"tools": []any{map[string]any{"name": "bash"}}})
	if len(m.tools) != 1 {
		t.Fatalf("unexpected tools: %#v", m.tools)
	}
}

func TestRenameCommand(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	m := Model{store: store, sessionID: "s1"}
	m.handleCommand("/rename 新标题")
	entry, err := store.Load("s1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "新标题" {
		t.Fatalf("unexpected title: %q", entry.Title)
	}
}

func TestSearchCommand(t *testing.T) {
	m := Model{
		allSessions: []session.Entry{{ID: "alpha"}, {ID: "beta"}},
	}
	m.handleCommand("/search alp")
	if len(m.sessions) != 1 || m.sessions[0].ID != "alpha" {
		t.Fatalf("unexpected filtered sessions: %#v", m.sessions)
	}
	m.handleCommand("/search")
	if len(m.sessions) != 2 {
		t.Fatalf("expected filter cleared, got %d", len(m.sessions))
	}
}

func TestConnectCmdKeepsSubscribeContextAlive(t *testing.T) {
	fake := &fakeBackend{}
	m := Model{
		cfg:       &config.Config{Workspace: "ws"},
		cl:        fake,
		sessionID: "s1",
	}

	cmd := m.connectCmd()
	msg, ok := cmd().(connectedMsg)
	if !ok {
		t.Fatalf("connectCmd returned %T, want connectedMsg", msg)
	}
	if fake.subCtx == nil {
		t.Fatal("Subscribe was not called")
	}
	if err := fake.subCtx.Err(); err != nil {
		t.Fatalf("Subscribe context should still be alive after connectCmd returns, got %v", err)
	}
	if msg.cancel == nil {
		t.Fatal("connectedMsg.cancel is nil")
	}
	msg.cancel()
	if err := fake.subCtx.Err(); err == nil {
		t.Fatal("expected Subscribe context to be canceled after connectedMsg.cancel")
	}
}

func TestHandleCommandUnknownSetsStatus(t *testing.T) {
	m := Model{}
	m.handleCommand("/does-not-exist")
	if m.status == "" {
		t.Fatal("expected non-empty status for unknown command")
	}
}

func TestSyncUsageFromState(t *testing.T) {
	m := Model{}
	m.syncUsageFromState(map[string]any{
		"context": map[string]any{"totalTokens": 42},
		"cost":    map[string]any{"estimatedYuan": 0.1},
	})
	if m.context == nil || m.context["totalTokens"] != 42 {
		t.Fatalf("unexpected context: %#v", m.context)
	}
	if m.cost == nil || m.cost["estimatedYuan"] != 0.1 {
		t.Fatalf("unexpected cost: %#v", m.cost)
	}
}

func TestDigitIndex(t *testing.T) {
	if idx, ok := digitIndex("3"); !ok || idx != 3 {
		t.Fatalf("expected 3, got %d ok=%v", idx, ok)
	}
	if _, ok := digitIndex("a"); ok {
		t.Fatal("expected not ok for non-digit")
	}
}

func TestSanitizeTerminalText(t *testing.T) {
	in := "hello\x1b[31mred\x00world\nnext"
	got := sanitizeTerminalText(in)
	if strings.ContainsAny(got, "\x1b\x00") {
		t.Fatalf("control chars not removed: %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Fatal("newline should be preserved")
	}
}

func TestCleanDiffPath(t *testing.T) {
	if _, err := cleanDiffPath("../../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejected")
	}
	if _, err := cleanDiffPath("/abs/path"); err == nil {
		t.Fatal("expected absolute path rejected")
	}
	if _, err := cleanDiffPath("a\\b"); err == nil {
		t.Fatal("expected backslash rejected")
	}
	if got, err := cleanDiffPath("a/b.txt"); err != nil || got != filepath.FromSlash("a/b.txt") {
		t.Fatalf("unexpected clean: %q err=%v", got, err)
	}
}

func TestNewLoadsResumeHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: "hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	m := New(&config.Config{DataDir: dir, SessionID: "s1"}, &fakeBackend{}, store)
	if len(m.messages) != 1 || m.messages[0].Content != "hi" {
		t.Fatalf("expected resume history loaded, got %#v", m.messages)
	}
}

func TestStaleConnectedMsgIgnored(t *testing.T) {
	m := Model{sessionEpoch: 2}
	res, _ := m.Update(connectedMsg{epoch: 1, snapshot: &client.Snapshot{ChannelID: "old"}})
	updated := res.(Model)
	if updated.channelID != "" {
		t.Fatalf("stale connectedMsg should be ignored, got channel %q", updated.channelID)
	}
}

func TestCommandPaletteOpenClose(t *testing.T) {
	m := Model{}
	m.openCommandPalette()
	if !m.commandPaletteOpen {
		t.Fatal("expected command palette open")
	}
	if len(m.commandPaletteFiltered) != len(commandPaletteCommands) {
		t.Fatalf("expected all commands listed, got %d", len(m.commandPaletteFiltered))
	}
	if m.commandPaletteQuery != "" {
		t.Fatalf("expected empty query, got %q", m.commandPaletteQuery)
	}

	m.closeCommandPalette()
	if m.commandPaletteOpen {
		t.Fatal("expected command palette closed")
	}
	if m.commandPaletteFiltered != nil {
		t.Fatalf("expected filtered list cleared, got %#v", m.commandPaletteFiltered)
	}
}

func TestCommandPaletteFilter(t *testing.T) {
	all := filterCommandPalette("")
	if len(all) != len(commandPaletteCommands) {
		t.Fatalf("expected all commands, got %d", len(all))
	}

	diff := filterCommandPalette("diff")
	if len(diff) == 0 || diff[0] != "/diff" {
		t.Fatalf("expected /diff first for query diff, got %#v", diff)
	}

	fuzzy := filterCommandPalette("df")
	foundDiff := false
	for _, cmd := range fuzzy {
		if cmd == "/diff" {
			foundDiff = true
			break
		}
	}
	if !foundDiff {
		t.Fatalf("expected /diff in fuzzy results for query df, got %#v", fuzzy)
	}
}

func TestCommandPaletteExecute(t *testing.T) {
	m := Model{}
	m.openCommandPalette()
	m.commandPaletteQuery = "context"
	m.applyCommandPaletteFilter()
	if len(m.commandPaletteFiltered) == 0 || m.commandPaletteFiltered[0] != "/context" {
		t.Fatalf("expected /context filtered first, got %#v", m.commandPaletteFiltered)
	}

	if handled, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter}); !handled {
		t.Fatal("expected enter to be consumed")
	}
	if m.commandPaletteOpen {
		t.Fatal("expected palette closed after execute")
	}
	if m.panel != panelContext {
		t.Fatalf("expected panelContext after execute, got %v", m.panel)
	}
}

func TestCommandPaletteCloseEsc(t *testing.T) {
	m := Model{}
	m.openCommandPalette()
	if handled, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEsc}); !handled {
		t.Fatal("expected esc to be consumed")
	}
	if m.commandPaletteOpen {
		t.Fatal("expected palette closed after esc")
	}
}

func TestCommandPaletteTypingNavigationAndBackspace(t *testing.T) {
	m := Model{}
	m.openCommandPalette()

	m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.commandPaletteQuery != "d" {
		t.Fatalf("expected query d, got %q", m.commandPaletteQuery)
	}
	if len(m.commandPaletteFiltered) == 0 {
		t.Fatal("expected filtered commands after typing")
	}

	m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.commandPaletteCursor != 1 {
		t.Fatalf("expected cursor 1 after j, got %d", m.commandPaletteCursor)
	}
	m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.commandPaletteCursor != 0 {
		t.Fatalf("expected cursor 0 after k, got %d", m.commandPaletteCursor)
	}

	m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.commandPaletteQuery != "" {
		t.Fatalf("expected query cleared after backspace, got %q", m.commandPaletteQuery)
	}
	if len(m.commandPaletteFiltered) != len(commandPaletteCommands) {
		t.Fatalf("expected all commands after clearing, got %d", len(m.commandPaletteFiltered))
	}
}

func TestCommandPaletteUpdateCtrlP(t *testing.T) {
	m := Model{}
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	updated := res.(Model)
	if !updated.commandPaletteOpen {
		t.Fatal("expected ctrl+p to open command palette")
	}

	res, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated = res.(Model)
	if updated.commandPaletteOpen {
		t.Fatal("expected esc to close command palette via Update")
	}
}

func TestCommandPaletteExecuteNewSkipsInput(t *testing.T) {
	input := textarea.New()
	input.SetValue("hello")
	m := Model{
		input: input,
		cl:    &fakeBackend{},
		cfg:   &config.Config{Workspace: "."},
	}
	m.openCommandPalette()
	m.commandPaletteFiltered = []string{"/new"}

	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := res.(Model)
	if len(updated.messages) != 0 {
		t.Fatalf("expected no message sent from palette, got %#v", updated.messages)
	}
	if updated.input.Value() != "hello" {
		t.Fatalf("expected pending input untouched, got %q", updated.input.Value())
	}
	if updated.sessionID != "" {
		t.Fatalf("expected new session to be started, got session %q", updated.sessionID)
	}
}

func TestSkillInstallCommandFromDir(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "demo.md"), []byte("# Demo\n\nDemo skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := Model{cfg: &config.Config{DataDir: dest}}
	cmd := m.handleCommand("/skill-install " + src)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for /skill-install")
	}
	msg := cmd()
	im, ok := msg.(skillInstallMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if im.err != nil {
		t.Fatalf("skill install command: %v", im.err)
	}
	if len(im.entries) != 1 || im.entries[0].Name != "demo" {
		t.Fatalf("unexpected installed entries: %#v", im.entries)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "demo.md")); err != nil {
		t.Fatalf("installed file missing: %v", err)
	}

	res, _ := m.Update(im)
	updated := res.(Model)
	if len(updated.localSkills) != 1 || updated.localSkills[0].Name != "demo" {
		t.Fatalf("expected local skills refreshed, got %#v", updated.localSkills)
	}
}

func TestSkillSyncCommandFromLocalManifest(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "sync.md"), []byte("# Sync\n\nSync skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(t.TempDir(), "manifest.json")
	data, _ := json.Marshal([]map[string]string{
		{"name": "sync", "url": src, "description": "Sync skill"},
	})
	if err := os.WriteFile(manifest, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m := Model{cfg: &config.Config{DataDir: dest}}
	cmd := m.handleCommand("/skill-sync " + manifest)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd for /skill-sync")
	}
	msg := cmd()
	sm, ok := msg.(skillSyncMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if sm.err != nil {
		t.Fatalf("skill sync command: %v", sm.err)
	}
	if len(sm.entries) != 1 || sm.entries[0].Name != "sync" {
		t.Fatalf("unexpected synced entries: %#v", sm.entries)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "sync.md")); err != nil {
		t.Fatalf("synced file missing: %v", err)
	}
}

func TestFindCommandSearch(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{
		ID:       "s1",
		Messages: []client.Message{{Role: "user", Content: "hello world"}},
	}); err != nil {
		t.Fatal(err)
	}
	m := Model{store: store}
	m.handleCommand("/find hello")
	if m.panel != panelSearch || len(m.searchResults) == 0 {
		t.Fatalf("expected search results, panel=%v results=%d", m.panel, len(m.searchResults))
	}
}

func TestPluginPanelToggle(t *testing.T) {
	m := Model{pluginManager: plugin.NewManager()}
	m.handleCommand("/plugin")
	if m.panel != panelPlugins {
		t.Fatalf("expected panelPlugins, got %v", m.panel)
	}
}

func writeDemoPlugin(t *testing.T) *plugin.Manager {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("external plugin shell test skipped on windows")
	}
	dir := t.TempDir()
	manifest := `{"name":"demo","version":"1.0.0","description":"Demo plugin","commands":["hello","status"],"executable":"./demo.sh"}`
	if err := os.WriteFile(filepath.Join(dir, "demo.dsh-plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo.sh"), []byte("#!/bin/sh\necho \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr := plugin.NewManager(dir)
	if err := mgr.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func TestCommandPaletteIncludesPluginCommands(t *testing.T) {
	m := Model{pluginManager: writeDemoPlugin(t)}
	m.openCommandPalette()
	for _, want := range []string{"/plugin:demo:hello", "/hello", "/plugin:demo:status", "/status"} {
		found := false
		for _, cmd := range m.commandPaletteFiltered {
			if cmd == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q in command palette, got %#v", want, m.commandPaletteFiltered)
		}
	}
}

func TestHandlePluginCommandRuns(t *testing.T) {
	m := Model{pluginManager: writeDemoPlugin(t)}

	cmd := m.handleCommand("/hello arg1 arg2")
	if cmd == nil {
		t.Fatal("expected plugin command tea.Cmd for /hello")
	}
	msg := cmd()
	res, ok := msg.(pluginResultMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if res.err != nil {
		t.Fatalf("plugin command error: %v", res.err)
	}
	if !strings.Contains(res.output, "arg1") || !strings.Contains(res.output, "arg2") {
		t.Fatalf("unexpected plugin output: %q", res.output)
	}

	cmd = m.handleCommand("/plugin:demo:status")
	if cmd == nil {
		t.Fatal("expected plugin command tea.Cmd for /plugin:demo:status")
	}
	msg = cmd()
	res, ok = msg.(pluginResultMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if res.err != nil {
		t.Fatalf("explicit plugin command error: %v", res.err)
	}
}

func TestPersistThrottleAndFlush(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{
		store:     store,
		sessionID: "s1",
		cfg:       &config.Config{Workspace: "ws"},
		messages:  []client.Message{{Role: "user", Content: "hello"}},
	}

	m.persist()
	if m.persistDirty {
		t.Fatal("expected clean after first immediate persist")
	}
	if _, err := store.Load("s1"); err != nil {
		t.Fatalf("expected session saved on first persist: %v", err)
	}

	m.persistDirty = true
	m.lastPersistAt = time.Now()
	m.persist()
	if !m.persistDirty {
		t.Fatal("expected dirty to remain while throttled")
	}

	m.flushPersist()
	if m.persistDirty {
		t.Fatal("expected clean after flushPersist")
	}
	entry, err := store.Load("s1")
	if err != nil {
		t.Fatalf("expected session after flush: %v", err)
	}
	if len(entry.Messages) != 1 || entry.Messages[0].Content != "hello" {
		t.Fatalf("unexpected persisted messages: %#v", entry.Messages)
	}
}

func TestApplyLayoutNarrowHidesPanels(t *testing.T) {
	m := Model{
		width:          60,
		height:         20,
		rightPanel:     true,
		splitEnabled:   true,
		splitSessionID: "s2",
		input:          textarea.New(),
	}
	m.applyLayout()
	if m.rightPanel || m.splitEnabled || m.splitSessionID != "" {
		t.Fatalf("expected right panel and split hidden on narrow screen, got right=%v split=%v splitID=%q", m.rightPanel, m.splitEnabled, m.splitSessionID)
	}
}

func TestApplyLayoutWideKeepsRightPanel(t *testing.T) {
	m := Model{width: 120, height: 20, rightPanel: true, input: textarea.New()}
	m.applyLayout()
	if !m.rightPanel {
		t.Fatal("expected right panel kept on wide screen")
	}
}

func TestMouseWheelMovesCommandPaletteCursor(t *testing.T) {
	m := Model{}
	m.openCommandPalette()
	m.commandPaletteCursor = 0

	res, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated := res.(Model)
	if updated.commandPaletteCursor != 1 {
		t.Fatalf("expected cursor 1 after wheel down, got %d", updated.commandPaletteCursor)
	}

	res, _ = updated.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	updated = res.(Model)
	if updated.commandPaletteCursor != 0 {
		t.Fatalf("expected cursor 0 after wheel up, got %d", updated.commandPaletteCursor)
	}
}

func TestMouseWheelMovesPluginPanelCursor(t *testing.T) {
	m := Model{
		panel:   panelPlugins,
		plugins: []plugin.Plugin{{Manifest: plugin.Manifest{Name: "a"}}, {Manifest: plugin.Manifest{Name: "b"}}},
	}
	res, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated := res.(Model)
	if updated.pluginCursor != 1 {
		t.Fatalf("expected plugin cursor 1 after wheel down, got %d", updated.pluginCursor)
	}

	res, _ = updated.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	updated = res.(Model)
	if updated.pluginCursor != 0 {
		t.Fatalf("expected plugin cursor 0 after wheel up, got %d", updated.pluginCursor)
	}
}

func TestIsViewportScrollMsg(t *testing.T) {
	if !isViewportScrollMsg(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}) {
		t.Fatal("expected wheel up to be a viewport scroll message")
	}
	if !isViewportScrollMsg(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}) {
		t.Fatal("expected wheel down to be a viewport scroll message")
	}
	if isViewportScrollMsg(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("expected enter not to be a viewport scroll message")
	}
}

// marker

func TestMarkerAppend(t *testing.T) {}

func TestNewSetsLanguageFromConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, Lang: "en"}
	_ = New(cfg, &fakeBackend{}, nil)
	if i18n.CurrentLang() != i18n.LangEN {
		t.Fatalf("CurrentLang = %q, want en after New with --lang en", i18n.CurrentLang())
	}
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })
}

func TestNewPlaceholderUsesLanguage(t *testing.T) {
	dir := t.TempDir()
	i18n.SetLang(i18n.LangZH)
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })

	m := New(&config.Config{DataDir: dir, Lang: "zh"}, &fakeBackend{}, nil)
	if !strings.Contains(m.input.Placeholder, "输入消息") {
		t.Fatalf("zh placeholder = %q, want Chinese text", m.input.Placeholder)
	}

	m = New(&config.Config{DataDir: dir, Lang: "en"}, &fakeBackend{}, nil)
	if !strings.Contains(m.input.Placeholder, "Type a message") {
		t.Fatalf("en placeholder = %q, want English text", m.input.Placeholder)
	}
}

func TestRenderPanelUsesI18n(t *testing.T) {
	i18n.SetLang(i18n.LangZH)
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })

	m := Model{panel: panelContext}
	zh := m.renderPanel()
	if !strings.Contains(zh, "context 数据暂不可用") {
		t.Fatalf("zh panel = %q, want Chinese context unavailable", zh)
	}

	i18n.SetLang(i18n.LangEN)
	en := m.renderPanel()
	if !strings.Contains(en, "Context data unavailable") {
		t.Fatalf("en panel = %q, want English context unavailable", en)
	}
}

func TestCommandHelpUsesI18n(t *testing.T) {
	i18n.SetLang(i18n.LangZH)
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })

	m := Model{}
	m.handleCommand("/help")
	if !strings.Contains(m.status, "可用命令") {
		t.Fatalf("zh help status = %q, want Chinese command help", m.status)
	}

	i18n.SetLang(i18n.LangEN)
	m = Model{}
	m.handleCommand("/help")
	if !strings.Contains(m.status, "Commands:") {
		t.Fatalf("en help status = %q, want English command help", m.status)
	}
}

func TestStopPromptUsesI18n(t *testing.T) {
	i18n.SetLang(i18n.LangZH)
	t.Cleanup(func() { i18n.SetLang(i18n.LangZH) })

	m := Model{cl: &fakeBackend{}}
	res, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := res.(Model)
	if !strings.Contains(updated.status, "再次 Ctrl+C 退出") {
		t.Fatalf("zh stop prompt = %q, want Chinese stop prompt", updated.status)
	}

	i18n.SetLang(i18n.LangEN)
	m = Model{cl: &fakeBackend{}}
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated = res.(Model)
	if !strings.Contains(updated.status, "press Ctrl+C again") {
		t.Fatalf("en stop prompt = %q, want English stop prompt", updated.status)
	}
}

func TestAuditMessageSent(t *testing.T) {
	audit := &fakeAudit{}
	m := Model{cl: &fakeBackend{}, sessionID: "s1", auditFn: audit.record}
	cmd := m.sendCmdTo("hello", "ch1")
	msg := cmd()
	if _, ok := msg.(sentMsg); !ok {
		t.Fatalf("unexpected msg type: %T %v", msg, msg)
	}
	if len(audit.entries) != 1 || audit.entries[0].event != "message.sent" {
		t.Fatalf("expected message.sent audit, got %#v", audit.eventNames())
	}
	fields := audit.entries[0].fields
	if fields["sessionId"] != "s1" || fields["channelId"] != "ch1" {
		t.Fatalf("unexpected message.sent fields: %#v", fields)
	}
}

func TestAuditSessionSwitch(t *testing.T) {
	audit := &fakeAudit{}
	m := Model{cl: &fakeBackend{}, channelID: "old-ch", auditFn: audit.record}
	res, _ := m.Update(switchSessionMsg{id: "s2"})
	updated := res.(Model)
	if updated.sessionID != "s2" {
		t.Fatalf("expected session s2, got %q", updated.sessionID)
	}
	if len(audit.entries) != 1 || audit.entries[0].event != "session.switch" {
		t.Fatalf("expected session.switch audit, got %#v", audit.eventNames())
	}
	if audit.entries[0].fields["sessionId"] != "s2" {
		t.Fatalf("unexpected session.switch fields: %#v", audit.entries[0].fields)
	}
}

func TestAuditSessionNew(t *testing.T) {
	audit := &fakeAudit{}
	m := Model{cl: &fakeBackend{}, sessionID: "old", pendingNew: true, auditFn: audit.record}
	res, _ := m.Update(tea.KeyMsg{})
	updated := res.(Model)
	if updated.sessionID != "" {
		t.Fatalf("expected new empty session, got %q", updated.sessionID)
	}
	if len(audit.entries) != 1 || audit.entries[0].event != "session.new" {
		t.Fatalf("expected session.new audit, got %#v", audit.eventNames())
	}
	if audit.entries[0].fields["previousSessionId"] != "old" {
		t.Fatalf("unexpected session.new fields: %#v", audit.entries[0].fields)
	}
}

func TestAuditSessionExport(t *testing.T) {
	dir := t.TempDir()
	audit := &fakeAudit{}
	m := Model{
		cfg:       &config.Config{DataDir: dir},
		sessionID: "s1",
		messages:  []client.Message{{Role: "user", Content: "hello"}},
		auditFn:   audit.record,
	}
	m.handleCommand("/export")
	if len(audit.entries) != 1 || audit.entries[0].event != "session.export" {
		t.Fatalf("expected session.export audit, got %#v", audit.eventNames())
	}
	fields := audit.entries[0].fields
	if fields["sessionId"] != "s1" {
		t.Fatalf("unexpected session.export fields: %#v", fields)
	}
	if path, _ := fields["path"].(string); !strings.Contains(path, "s1.md") {
		t.Fatalf("unexpected export path in audit: %#v", fields["path"])
	}
}

func TestAuditSessionDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s1"}); err != nil {
		t.Fatal(err)
	}
	audit := &fakeAudit{}
	m := Model{store: store, sessionID: "s1", auditFn: audit.record}
	m.handleCommand("/delete")
	if len(audit.entries) != 1 || audit.entries[0].event != "session.delete" {
		t.Fatalf("expected session.delete audit, got %#v", audit.eventNames())
	}
	if audit.entries[0].fields["sessionId"] != "s1" {
		t.Fatalf("unexpected session.delete fields: %#v", audit.entries[0].fields)
	}
}

func TestAuditDiffApply(t *testing.T) {
	repo := t.TempDir()
	if err := exec.Command("git", "-C", repo, "init").Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	audit := &fakeAudit{}
	m := Model{
		cfg:       &config.Config{DataDir: t.TempDir(), Workspace: repo},
		sessionID: "s1",
		diffs: []any{
			map[string]any{
				"file": "a.txt",
				"diff": "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
			},
		},
		diffDecisions: map[int]string{0: "accepted"},
		auditFn:       audit.record,
	}
	if _, err := m.applyAcceptedDiffsWithFilter(nil); err != nil {
		t.Fatalf("apply diff: %v", err)
	}
	if len(audit.entries) != 1 || audit.entries[0].event != "diff.apply" {
		t.Fatalf("expected diff.apply audit, got %#v", audit.eventNames())
	}
	files, ok := audit.entries[0].fields["files"].([]string)
	if !ok || len(files) != 1 || files[0] != "a.txt" {
		t.Fatalf("unexpected diff.apply files: %#v", audit.entries[0].fields["files"])
	}
}

func TestNewSetsAuditFn(t *testing.T) {
	var got []string
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil, func(event string, _ map[string]any) {
		got = append(got, event)
	})
	m.auditEvent("test.event", nil)
	if len(got) != 1 || got[0] != "test.event" {
		t.Fatalf("expected audit callback from New, got %#v", got)
	}
}

func TestSetAuditLogWritesEvent(t *testing.T) {
	dir := t.TempDir()
	log, err := security.NewAuditLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := Model{}
	m.SetAuditLog(log)
	m.auditEvent("message.sent", map[string]any{"sessionId": "s1"})
	data, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"event":"message.sent"`) {
		t.Fatalf("expected audit log entry, got %q", data)
	}
}

func TestKeyUIStringsNotHardcodedChineseInModel(t *testing.T) {
	data, err := os.ReadFile("model.go")
	if err != nil {
		t.Fatalf("read model.go: %v", err)
	}
	source := string(data)
	hardcoded := []string{
		`"停止请求中... 再次 Ctrl+C 退出"`,
		`"可用命令: /context`,
		`"命令面板  (Ctrl+P`,
		`"暂无本地会话"`,
		`"工具调用\n"`,
	}
	for _, s := range hardcoded {
		if strings.Contains(source, s) {
			t.Errorf("model.go still contains hardcoded Chinese UI string literal: %s", s)
		}
	}
}

func TestNewInitializesMCPPolicyAndTimeline(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	if m.mcpPolicy == nil {
		t.Fatal("expected mcpPolicy to be initialized")
	}
	if m.mcpAudit == nil {
		t.Fatal("expected mcpAudit to be initialized")
	}
	if m.timeline == nil {
		t.Fatal("expected timeline to be initialized")
	}
	if got := m.mcpPolicy.Mode("tool"); got != "confirm" {
		t.Fatalf("default policy mode = %q, want confirm", got)
	}
}

func TestMCPSetCommandsUpdatePolicy(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)

	m.handleCommand("/mcp-auto files.read")
	if got := m.mcpPolicy.Mode("files.read"); got != "auto" {
		t.Fatalf("Mode after /mcp-auto = %q, want auto", got)
	}
	m.handleCommand("/mcp-confirm files.read")
	if got := m.mcpPolicy.Mode("files.read"); got != "confirm" {
		t.Fatalf("Mode after /mcp-confirm = %q, want confirm", got)
	}
	m.handleCommand("/mcp-deny files.read")
	if got := m.mcpPolicy.Mode("files.read"); got != "deny" {
		t.Fatalf("Mode after /mcp-deny = %q, want deny", got)
	}
}

func TestMCPCallDeniedReturnsErrorAndRecordsAudit(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	m.handleCommand("/mcp-deny secrets.read")

	cmd := m.mcpCallCmd("secrets.read", nil)
	msg, ok := cmd().(mcpCallMsg)
	if !ok {
		t.Fatalf("unexpected msg type: %T", msg)
	}
	if msg.err == nil || !strings.Contains(msg.err.Error(), "denied by policy") {
		t.Fatalf("expected denied error, got %v", msg.err)
	}
	entries := m.mcpAudit.List()
	if len(entries) != 1 || entries[0].Tool != "secrets.read" {
		t.Fatalf("expected one audit entry for secrets.read, got %#v", entries)
	}
	if entries[0].Decision.Allowed {
		t.Fatalf("expected denied decision to be not allowed, got %#v", entries[0].Decision)
	}
}

func TestCheckpointAddsToTimelineAndRewindRestores(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	m.messages = []client.Message{{Role: "user", Content: "hello"}}
	m.handleCommand("/checkpoint first")
	if len(m.timeline.List()) != 1 {
		t.Fatalf("timeline length after checkpoint = %d, want 1", len(m.timeline.List()))
	}
	snapID := m.timeline.List()[0].ID

	m.messages = []client.Message{{Role: "assistant", Content: "changed"}}
	m.handleCommand("/rewind " + snapID)
	if len(m.messages) != 1 || m.messages[0].Content != "hello" {
		t.Fatalf("messages after timeline rewind = %#v, want original hello", m.messages)
	}
	if !strings.Contains(m.status, "rewound to timeline") {
		t.Fatalf("unexpected rewind status: %q", m.status)
	}
}

func TestTimelineAndForkCommands(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	m.messages = []client.Message{{Role: "user", Content: "hello"}}
	m.handleCommand("/checkpoint first")
	m.handleCommand("/timeline")
	if !strings.Contains(m.status, "timeline:") || !strings.Contains(m.status, "first") {
		t.Fatalf("unexpected timeline status: %q", m.status)
	}

	m.handleCommand("/fork branch")
	if !strings.HasPrefix(m.status, "forked timeline: ") {
		t.Fatalf("unexpected fork status: %q", m.status)
	}
	if len(m.timeline.List()) != 2 {
		t.Fatalf("timeline length after fork = %d, want 2", len(m.timeline.List()))
	}
}

func TestMCPAuditCommandShowsEntries(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	m.handleCommand("/mcp-deny secrets.read")
	m.mcpCallCmd("secrets.read", nil)()
	m.handleCommand("/mcp-audit")
	if !strings.Contains(m.status, "secrets.read") || !strings.Contains(m.status, "deny false false") {
		t.Fatalf("unexpected audit status: %q", m.status)
	}
}

func TestParseMultiTasksJSON(t *testing.T) {
	tasks, err := parseMultiTasks(`[
		{"id":"a","prompt":"run echo a"},
		{"id":"b","prompt":"run echo b","deps":["a"]}
	]`, "/tmp")
	if err != nil {
		t.Fatalf("parseMultiTasks() error = %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("len(tasks) = %d, want 2", len(tasks))
	}
	if tasks[0].ID != "a" || tasks[0].WorkDir != "/tmp" {
		t.Fatalf("task[0] = %+v", tasks[0])
	}
	if len(tasks[1].Dependencies) != 1 || tasks[1].Dependencies[0] != "a" {
		t.Fatalf("task[1].Dependencies = %v, want [a]", tasks[1].Dependencies)
	}
}

func TestRenderMultiAgentPanelShowsDependencyTree(t *testing.T) {
	m := New(&config.Config{DataDir: t.TempDir()}, &fakeBackend{}, nil)
	m.multiTasks = []multiagent.Task{
		{ID: "a", Prompt: "A"},
		{ID: "b", Prompt: "B", Dependencies: []string{"a"}},
	}
	m.multiStates = map[string]string{"a": "done", "b": "running"}
	got := m.renderMultiAgentPanel()
	if !strings.Contains(got, "a [done]") || !strings.Contains(got, "b [running]") {
		t.Fatalf("unexpected multi-agent panel:\n%s", got)
	}
}
