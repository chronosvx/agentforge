package dshapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/dshapi"
	"github.com/coder/websocket"
)

func TestUnaryAndSSE(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/api/session.list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		writeServerResponse(t, w, r, map[string]any{
			"items": []map[string]any{
				{"sessionId": "s1", "updatedAt": 1, "running": false, "blank": false},
			},
		})
	})
	mux.HandleFunc("/api/session.create", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{
			"sessionId": "new-session",
		})
	})
	mux.HandleFunc("/api/session.prompt", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{"accepted": true})
	})
	mux.HandleFunc("/api/events.mux", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "test done")
		data, _ := json.Marshal(map[string]any{
			"type": "server-request", "rpcId": "rpc-1", "method": "events.mux",
			"payload": map[string]any{
				"type": "session/event", "sessionId": "s1",
				"event": map[string]any{"type": "turn/start", "seq": 1, "time": 1, "data": map[string]any{}},
			},
		})
		_ = conn.Write(context.Background(), websocket.MessageText, data)
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	cl := dshapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	list, err := cl.ListSessions(ctx)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].SessionID != "s1" {
		t.Fatalf("unexpected list: %#v", list)
	}

	created, err := cl.CreateSession(ctx, dshapi.CreateSessionRequest{SessionID: "new-session"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.SessionID != "new-session" {
		t.Fatalf("unexpected create: %#v", created)
	}

	if _, err := cl.Prompt(ctx, "new-session", "hello", "queue"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	frames, err := cl.OpenMux(ctx)
	if err != nil {
		t.Fatalf("open mux: %v", err)
	}
	select {
	case frame := <-frames:
		if frame.Type != "server-request" || !strings.Contains(string(frame.Payload), "session/event") {
			t.Fatalf("unexpected mux frame: %#v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for mux frame")
	}
}

func TestCallError(t *testing.T) {
	var mux http.ServeMux
	mux.HandleFunc("/api/session.list", func(w http.ResponseWriter, r *http.Request) {
		writeServerResponse(t, w, r, map[string]any{})
		// The handler above writes a success result; we need a failure path, so
		// use a separate endpoint below.
	})
	mux.HandleFunc("/api/session.missing", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			RPCID string `json:"rpcId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"type": "server-response", "rpcId": req.RPCID,
			"result": map[string]any{
				"ok":    false,
				"error": map[string]any{"code": "session-not-found", "message": "missing", "details": map[string]any{}},
			},
		})
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	cl := dshapi.New(srv.URL)
	err := cl.Call(context.Background(), "session.missing", map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "session-not-found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPAuthorizationHeader(t *testing.T) {
	var got string
	var mux http.ServeMux
	mux.HandleFunc("/api/session.list", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		writeServerResponse(t, w, r, map[string]any{"items": []any{}})
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	cl := dshapi.NewWithToken(srv.URL, "secret-token")
	if _, err := cl.ListSessions(context.Background()); err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if got != "Bearer secret-token" {
		t.Fatalf("unexpected authorization header: %q", got)
	}
}

func TestWebSocketAuthorizationHeader(t *testing.T) {
	var got string
	var mux http.ServeMux
	mux.HandleFunc("/api/events.mux", func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "test done")
	})

	srv := httptest.NewServer(&mux)
	defer srv.Close()

	cl := dshapi.NewWithToken(srv.URL, "secret-token")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	frames, err := cl.OpenMux(ctx)
	if err != nil {
		t.Fatalf("open mux: %v", err)
	}
	if got != "Bearer secret-token" {
		t.Fatalf("unexpected websocket authorization header: %q", got)
	}
	_ = frames
}

func writeServerResponse(t *testing.T, w http.ResponseWriter, r *http.Request, value any) {
	t.Helper()
	var req struct {
		RPCID string `json:"rpcId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode request: %v", err)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":  "server-response",
		"rpcId": req.RPCID,
		"result": map[string]any{
			"ok":    true,
			"value": value,
		},
	})
}
