package client

import (
	"testing"
	"time"
)

func TestBroadcastEventDoesNotBlockWhenSubscriberFull(t *testing.T) {
	ch := make(chan Event, 1)
	ch <- Event{}

	c := &Client{
		subs: map[string]map[chan Event]struct{}{
			"channel-1": {ch: {}},
		},
		done: make(chan struct{}),
	}

	ver := int64(1)
	msg := serverMessage{
		Operation: "event",
		ChannelID: "channel-1",
		Version:   &ver,
	}

	done := make(chan struct{})
	go func() {
		c.broadcastEvent(msg)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcastEvent blocked on a full subscriber channel")
	}
}
