package dshapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/coder/websocket"
)

func TestBackendMuxSurvivesOpenContextCancel(t *testing.T) {
	accepted := make(chan struct{})
	closed := make(chan struct{})

	var mux http.ServeMux
	mux.HandleFunc("/api/session.history", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{"events": []any{}, "hasMore": false})
	})
	mux.HandleFunc("/api/events.mux", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		readDone := conn.CloseRead(context.Background())
		accepted <- struct{}{}
		<-readDone.Done()
		closed <- struct{}{}
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	backend := dshapi.NewBackend(dshapi.New(srv.URL))
	openCtx, openCancel := context.WithCancel(context.Background())
	snap, err := backend.Open(openCtx, "/tmp", "s1", nil)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for events.mux websocket to be accepted")
	}

	// Cancelling the caller's Open context must not tear down the mux stream.
	openCancel()
	select {
	case <-closed:
		t.Fatal("events.mux closed after Open context was cancelled")
	case <-time.After(200 * time.Millisecond):
	}

	// Backend.Close is the owner of the mux and should close it.
	if err := backend.Close(context.Background(), snap.ChannelID); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("events.mux did not close after Backend.Close")
	}
}
