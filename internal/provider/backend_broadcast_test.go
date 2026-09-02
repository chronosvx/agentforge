package provider

import (
	"context"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
)

func TestBackendCloseNotBlockedBySlowTerminalSubscriber(t *testing.T) {
	b := NewBackend(nil, nil)
	if _, err := b.Open(context.Background(), "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if _, err := b.Subscribe(context.Background(), "sess", 0); err != nil {
		t.Fatalf("Subscribe() error: %v", err)
	}

	// Fill the subscriber channel so a terminal send cannot complete until a
	// reader drains it.
	b.mu.Lock()
	st := b.sessions["sess"]
	if st == nil {
		b.mu.Unlock()
		t.Fatal("session not found after Open")
	}
	for ch := range st.subs {
		for i := 0; i < cap(ch); i++ {
			ch <- client.Event{Operation: "event", ChannelID: "sess", Version: int64(i + 1)}
		}
	}
	b.mu.Unlock()

	started := make(chan struct{})
	go func() {
		b.mu.Lock()
		close(started)
		b.broadcastLocked(client.Event{Operation: "done", ChannelID: "sess", Version: 999}, st)
		b.mu.Unlock()
	}()

	<-started
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- b.Close(context.Background(), "sess")
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close() blocked because a slow subscriber stalled the terminal broadcast")
	}
}
