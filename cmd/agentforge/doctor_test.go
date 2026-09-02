package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckDSHWebAPISuccess(t *testing.T) {
	var gotPath, gotAuth, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"server-response","ok":true}`))
	}))
	defer srv.Close()

	err := checkDSHWebAPI(srv.URL, "test-token")
	if err != nil {
		t.Fatalf("checkDSHWebAPI returned error for valid server: %v", err)
	}
	if gotPath != "/api/host.describe" {
		t.Fatalf("request path = %q, want /api/host.describe", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
}

func TestCheckDSHWebAPISuccessWithoutToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("Authorization header = %q, want empty", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":"ok"}`))
	}))
	defer srv.Close()

	if err := checkDSHWebAPI(srv.URL, ""); err != nil {
		t.Fatalf("checkDSHWebAPI returned error for valid server: %v", err)
	}
}

func TestCheckDSHWebAPIHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer srv.Close()

	err := checkDSHWebAPI(srv.URL, "")
	if err == nil {
		t.Fatal("checkDSHWebAPI returned nil for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want mention of HTTP status", err)
	}
}

func TestCheckDSHWebAPIBadBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"type":"unexpected"}`))
	}))
	defer srv.Close()

	err := checkDSHWebAPI(srv.URL, "")
	if err == nil {
		t.Fatal("checkDSHWebAPI returned nil for body missing server-response/ok")
	}
	if !strings.Contains(err.Error(), "missing server-response/ok") {
		t.Fatalf("error = %v, want readable missing marker message", err)
	}
}

func TestCheckDSHVersionMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := checkDSHVersion(); err == nil {
		t.Fatal("checkDSHVersion returned nil when dsh is not on PATH")
	}
}
