package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageCommandNoArgsShowsUsageAndDoesNotSend(t *testing.T) {
	fb := &fakeBackend{}
	m := Model{cl: fb, channelID: "s1"}

	cmd := m.handleCommand("/image")
	if cmd != nil {
		t.Fatalf("handleCommand(/image) cmd = %#v, want nil", cmd)
	}
	if !strings.Contains(m.status, "usage: /image") {
		t.Fatalf("status = %q, want usage hint", m.status)
	}
	if len(m.messages) != 0 {
		t.Fatalf("messages = %#v, want empty", m.messages)
	}
	if len(fb.invokes) != 0 {
		t.Fatalf("invokes = %#v, want empty", fb.invokes)
	}
}

func TestImageCommandFileNotFoundShowsErrorAndDoesNotSend(t *testing.T) {
	fb := &fakeBackend{}
	m := Model{cl: fb, channelID: "s1"}

	missing := filepath.Join(t.TempDir(), "missing.png")
	cmd := m.handleCommand("/image " + missing + " describe")
	if cmd != nil {
		t.Fatalf("handleCommand(/image missing) cmd = %#v, want nil", cmd)
	}
	if !strings.Contains(m.status, "image not found") {
		t.Fatalf("status = %q, want image not found", m.status)
	}
	if len(m.messages) != 0 {
		t.Fatalf("messages = %#v, want empty", m.messages)
	}
	if len(fb.invokes) != 0 {
		t.Fatalf("invokes = %#v, want empty", fb.invokes)
	}
}

func TestImageCommandSuccessAppendsUserMessageAndSends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.png")
	if err := os.WriteFile(path, []byte("fake-image"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	fb := &fakeBackend{}
	m := Model{cl: fb, channelID: "s1"}
	cmd := m.handleCommand("/image " + path + " describe this")
	if cmd == nil {
		t.Fatal("handleCommand(/image) cmd = nil, want tea.Cmd")
	}

	if len(m.messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1; messages=%#v", len(m.messages), m.messages)
	}
	msg := m.messages[0]
	if msg.Role != "user" || msg.Content != "describe this" {
		t.Fatalf("message = %#v, want user describe this", msg)
	}
	if !m.dirty {
		t.Fatal("expected dirty=true after appending image user message")
	}

	result := cmd()
	if _, ok := result.(sentMsg); !ok {
		t.Fatalf("result type = %T, want sentMsg", result)
	}
	if len(fb.invokes) != 1 {
		t.Fatalf("len(invokes) = %d, want 1; invokes=%#v", len(fb.invokes), fb.invokes)
	}
	if len(fb.invokeArgs) != 1 {
		t.Fatalf("len(invokeArgs) = %d, want 1", len(fb.invokeArgs))
	}
	args := fb.invokeArgs[0]
	if len(args) != 2 {
		t.Fatalf("args = %#v, want [text paths]", args)
	}
	if text, _ := args[0].(string); text != "describe this" {
		t.Fatalf("args[0] = %#v, want describe this", args[0])
	}
	paths, ok := args[1].([]string)
	if !ok || len(paths) != 1 || paths[0] != path {
		t.Fatalf("args[1] = %#v, want [%q]", args[1], path)
	}
}

func TestImageCommandWithoutTextUsesFallbackText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.png")
	if err := os.WriteFile(path, []byte("fake-image"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	fb := &fakeBackend{}
	m := Model{cl: fb, channelID: "s1"}
	cmd := m.handleCommand("/image " + path)
	if cmd == nil {
		t.Fatal("handleCommand(/image path) cmd = nil, want tea.Cmd")
	}

	if len(m.messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(m.messages))
	}
	if got := m.messages[0].Content; got != "查看图片："+path {
		t.Fatalf("message content = %q, want %q", got, "查看图片："+path)
	}

	_ = cmd()
	if len(fb.invokeArgs) != 1 {
		t.Fatalf("len(invokeArgs) = %d, want 1", len(fb.invokeArgs))
	}
	args := fb.invokeArgs[0]
	if text, _ := args[0].(string); text != "查看图片："+path {
		t.Fatalf("args[0] = %#v, want fallback text", args[0])
	}
}
