package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/session"
)

func TestHealthz(t *testing.T) {
	srv := NewServer(nil, "", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestCreateAndPrompt(t *testing.T) {
	store, _ := session.NewStore(t.TempDir())
	prompt := func(ctx context.Context, id, text string) (string, error) {
		return "ok:" + text, nil
	}
	srv := NewServer(store, "", prompt)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"id":"s1"}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/sessions/s1/prompt", strings.NewReader(`{"text":"hello"}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prompt status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["result"] != "ok:hello" {
		t.Fatalf("unexpected result: %#v", out)
	}
}

func TestPromptFuncFieldUsesFakePrompt(t *testing.T) {
	srv := NewServer(nil, "", nil)
	srv.PromptFunc = func(ctx context.Context, id, text string) (string, error) {
		return "fake:" + text, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/s1/prompt", strings.NewReader(`{"text":"hello"}`))
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out["result"] != "fake:hello" {
		t.Fatalf("unexpected result: %#v", out)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	srv := NewServer(nil, "secret", nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
