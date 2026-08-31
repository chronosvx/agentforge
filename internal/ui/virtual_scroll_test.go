package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/client"
)

func TestVirtualScrollRendersOnlyVisibleRange(t *testing.T) {
	messages := make([]client.Message, 100)
	for i := range messages {
		messages[i] = client.Message{Role: "user", Content: fmt.Sprintf("msg-%02d", i)}
	}

	m := Model{
		messages: messages,
		viewport: viewport.New(80, 10),
	}
	m.dirty = true
	m.renderMessages()

	content := m.viewport.View()
	if !strings.Contains(content, "msg-00") {
		t.Fatalf("expected first visible message in content, got %q", content)
	}
	if strings.Contains(content, "msg-99") {
		t.Fatalf("virtual scroll rendered out-of-range message 99: %q", content)
	}
	if m.viewport.TotalLineCount() >= len(messages) {
		t.Fatalf("expected viewport content to contain only a visible slice, got %d lines", m.viewport.TotalLineCount())
	}

	// 向上滚动查看历史：从接近末尾的位置渲染时不应再包含最早的消息。
	m.scrollOffset = 95
	m.dirty = true
	m.renderMessages()

	content = m.viewport.View()
	if !strings.Contains(content, "msg-95") {
		t.Fatalf("expected scrolled view to include msg-95, got %q", content)
	}
	if strings.Contains(content, "msg-00") {
		t.Fatalf("scrolled view should not include first message, got %q", content)
	}
}

func TestIncrementalRenderSkipsRebuildWhenUnchanged(t *testing.T) {
	m := Model{
		messages: []client.Message{{Role: "user", Content: "hello"}},
		viewport: viewport.New(80, 10),
	}
	m.dirty = true
	m.renderMessages()
	if m.viewportContentBuilds != 1 {
		t.Fatalf("expected first render to build viewport once, got %d", m.viewportContentBuilds)
	}
	if m.dirty {
		t.Fatal("expected dirty to be cleared after render")
	}

	builds := m.viewportContentBuilds
	m.renderMessages()
	if m.viewportContentBuilds != builds {
		t.Fatalf("unchanged messages rebuilt viewport: builds=%d -> %d", builds, m.viewportContentBuilds)
	}
	if m.dirty {
		t.Fatal("expected dirty to stay false after unchanged render")
	}

	// 消息变化后再次渲染应重建。
	m.messages[0].Content = "changed"
	m.dirty = true
	m.renderMessages()
	if m.viewportContentBuilds != builds+1 {
		t.Fatalf("changed messages should rebuild viewport: builds=%d -> %d", builds, m.viewportContentBuilds)
	}
}

func TestSyncMessagesFromStateSkipsDirtyWhenUnchanged(t *testing.T) {
	m := Model{messages: []client.Message{{Role: "user", Content: "hi"}}}
	m.syncMessagesFromState(map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if m.dirty {
		t.Fatal("expected dirty to stay false when messages are unchanged")
	}

	m.syncMessagesFromState(map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "changed"}},
	})
	if !m.dirty {
		t.Fatal("expected dirty to be set when messages changed")
	}
}

func TestVirtualScrollHistoryNotForcedToBottom(t *testing.T) {
	messages := make([]client.Message, 50)
	for i := range messages {
		messages[i] = client.Message{Role: "user", Content: fmt.Sprintf("msg-%02d", i)}
	}
	m := Model{
		messages:     messages,
		viewport:     viewport.New(80, 10),
		scrollOffset: 40,
		followBottom: false,
	}
	m.dirty = true
	m.renderMessages()

	// 模拟一条新消息到达：Update 非滚动消息会把视图贴底，这里直接验证 scrollByViewportMsg
	// 不会强制跳底，只有非滚动更新才贴底。
	m.scrollByViewportMsg(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if m.scrollOffset >= 40 {
		t.Fatalf("wheel up should move toward history, got scrollOffset=%d", m.scrollOffset)
	}
	if m.followBottom {
		t.Fatal("scrolling up should stop following the bottom")
	}
}
