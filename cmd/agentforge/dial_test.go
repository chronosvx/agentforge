package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/config"
)

func TestIsInsecureRemoteURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{raw: "", want: false},
		{raw: "not a url", want: false},
		{raw: "http://127.0.0.1:8080", want: false},
		{raw: "http://localhost:8080", want: false},
		{raw: "http://[::1]:8080", want: false},
		{raw: "ws://127.0.0.1:18891/agentforge/v1/connection", want: false},
		{raw: "http://example.com", want: true},
		{raw: "http://example.com:8080/v1", want: true},
		{raw: "ws://example.com/agentforge", want: true},
		{raw: "https://example.com", want: false},
		{raw: "wss://example.com/agentforge", want: false},
	}

	for _, tt := range tests {
		if got := isInsecureRemoteURL(tt.raw); got != tt.want {
			t.Errorf("isInsecureRemoteURL(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestWarnIfInsecureRemoteDoesNotDependOnRemote(t *testing.T) {
	cfg := &config.Config{
		APIURL:          "http://api.example.com",
		Endpoint:        "ws://channel.example.com/agentforge",
		ProviderBaseURL: "http://provider.example.com/v1",
		// Remote deliberately false: the check must still run.
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stderr = w
	warnIfInsecureRemote(cfg)
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)

	text := string(out)
	for _, want := range []string{"api-url", "endpoint", "provider-base-url"} {
		if !strings.Contains(text, want) {
			t.Fatalf("warnIfInsecureRemote output does not mention %q:\n%s", want, text)
		}
	}
}

func TestCheckInsecureRemoteRefusesCredentialOverPlaintextRemote(t *testing.T) {
	cfg := &config.Config{
		APIURL:          "http://api.example.com",
		APIToken:        "secret-token",
		ProviderBaseURL: "http://provider.example.com/v1",
		ProviderKey:     "secret-key",
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stderr = w
	err = checkInsecureRemote(cfg)
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)

	if err == nil {
		t.Fatal("checkInsecureRemote() error = nil, want refusal when credentials are sent over remote plaintext")
	}
	if !strings.Contains(err.Error(), "--allow-insecure-http") {
		t.Fatalf("checkInsecureRemote() error = %v, want --allow-insecure-http hint", err)
	}
	text := string(out)
	if !strings.Contains(text, "HIGH-RISK warning") {
		t.Fatalf("checkInsecureRemote() stderr lacks high-risk warning:\n%s", text)
	}
}

func TestCheckInsecureRemoteAllowsWithExplicitOverride(t *testing.T) {
	cfg := &config.Config{
		APIURL:            "http://api.example.com",
		APIToken:          "secret-token",
		ProviderBaseURL:   "http://provider.example.com/v1",
		ProviderKey:       "secret-key",
		AllowInsecureHTTP: true,
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stderr = w
	err = checkInsecureRemote(cfg)
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)

	if err != nil {
		t.Fatalf("checkInsecureRemote() with --allow-insecure-http = %v, want nil", err)
	}
	if !strings.Contains(string(out), "--allow-insecure-http") {
		t.Fatalf("checkInsecureRemote() stderr should explain override is active:\n%s", string(out))
	}
}
