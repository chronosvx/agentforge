package ui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestProgramSendMessage(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	cfg := &config.Config{DataDir: dir, Workspace: "."}
	model := New(cfg, fb, store)
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))

	go func() {
		time.Sleep(50 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello")})
		time.Sleep(30 * time.Millisecond)
		p.Send(tea.KeyMsg{Type: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)
		sendQuit(p)
	}()

	if _, err := p.Run(); err != nil {
		t.Fatalf("program run: %v", err)
	}
	if len(fb.invokes) == 0 {
		t.Fatal("expected backend invoke after sending message")
	}
}

func TestProgramToggleRightPanel(t *testing.T) {
	final := runProgramWithCommands(t, "/right")
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if !m.rightPanel {
		t.Fatal("expected rightPanel true")
	}
}

func TestProgramToggleSplit(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	now := time.Now()
	if err := store.Save(session.Entry{ID: "s1", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(session.Entry{ID: "s2", UpdatedAt: now.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{DataDir: dir, Workspace: "."}
	model := New(cfg, fb, store)
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		typeCommand(p, "/split")
		time.Sleep(50 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if !m.splitEnabled {
		t.Fatal("expected splitEnabled true")
	}
}

func TestProgramOpenDiffPanel(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save(session.Entry{
		ID:    "s1",
		Diffs: []any{map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"}},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{DataDir: dir, Workspace: ".", SessionID: "s1"}
	model := New(cfg, fb, store)
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		typeCommand(p, "/diff")
		time.Sleep(50 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if m.panel != panelDiff {
		t.Fatalf("expected panelDiff, got %v", m.panel)
	}
}

func TestProgramCostAlert(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg := &config.Config{DataDir: dir, Workspace: ".", CostLimit: 1}
	model := New(cfg, fb, store)
	model.cost = map[string]any{"estimatedYuan": 0.5}

	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		p.Send(eventMsg{
			channelID: "",
			event: client.Event{
				State: map[string]any{
					"cost": map[string]any{"estimatedYuan": 2.0},
				},
			},
		})
		time.Sleep(50 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if !m.costAlerted {
		t.Fatal("expected costAlerted true")
	}
}

func TestProgramDiffApplyConfirm(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if err := store.Save(session.Entry{
		ID: "s1",
		Diffs: []any{
			map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{DataDir: dir, Workspace: ".", SessionID: "s1"}
	model := New(cfg, fb, store)
	model.diffs = []any{map[string]any{"file": "a.txt", "diff": "diff --git a/a.txt b/a.txt\n-old\n+new"}}
	model.diffDecisions = map[int]string{0: "accepted"}
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		typeCommand(p, "/diff")
		time.Sleep(50 * time.Millisecond)
		sendSingleKey(p, "g")
		time.Sleep(50 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if !m.pendingApply {
		t.Fatal("expected pendingApply true after first g")
	}
}

func TestProgramThemeSwitch(t *testing.T) {
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg := &config.Config{DataDir: dir, Workspace: ".", Theme: "catppuccin"}
	model := New(cfg, fb, store)
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	m, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type: %T", final)
	}
	if m.accentColor() != "5" {
		t.Fatalf("expected catppuccin accent, got %q", m.accentColor())
	}
}

func sendSingleKey(p *tea.Program, key string) {
	p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
}

func sendQuit(p *tea.Program) {
	// Ctrl+C 第一次停止生成，第二次退出。
	p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	time.Sleep(20 * time.Millisecond)
	p.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
}

func runProgramWithCommands(t *testing.T, commands ...string) tea.Model {
	t.Helper()
	fb := &fakeBackend{}
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	cfg := &config.Config{DataDir: dir, Workspace: "."}
	model := New(cfg, fb, store)
	p := tea.NewProgram(model, tea.WithInput(strings.NewReader("")), tea.WithOutput(io.Discard))
	go func() {
		time.Sleep(80 * time.Millisecond)
		for _, cmd := range commands {
			typeCommand(p, cmd)
			time.Sleep(30 * time.Millisecond)
		}
		time.Sleep(30 * time.Millisecond)
		sendQuit(p)
	}()
	final, err := p.Run()
	if err != nil {
		t.Fatalf("program run: %v", err)
	}
	return final
}

func typeCommand(p *tea.Program, cmd string) {
	for _, r := range cmd {
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	p.Send(tea.KeyMsg{Type: tea.KeyEnter})
}
