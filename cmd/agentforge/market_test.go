package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMarketIndex(t *testing.T, entries ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "market-index.json")
	data := "[" + strings.Join(entries, ",") + "]"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunMarketSearch(t *testing.T) {
	indexPath := writeMarketIndex(t,
		`{"name":"hello-web","version":"1.0.0","description":"Web search plugin","url":"https://example.invalid/hello-web.zip"}`,
		`{"name":"git-tool","version":"2.0.0","description":"Git helper","url":"https://example.invalid/git.zip"}`,
	)

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := runMarket([]string{"--data-dir", t.TempDir(), "search", indexPath, "search"})
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("runMarket search: %v", runErr)
	}
	if !strings.Contains(string(out), "hello-web") {
		t.Fatalf("search output missing hello-web:\n%s", out)
	}
	if strings.Contains(string(out), "git-tool") {
		t.Fatalf("search output should not include git-tool:\n%s", out)
	}
}

func TestRunMarketInstallAndList(t *testing.T) {
	payload := []byte("#!/bin/sh\necho demo\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	indexPath := writeMarketIndex(t,
		fmt.Sprintf(`{"name":"demo","version":"1.0.0","description":"Demo plugin","url":%q}`, srv.URL+"/demo"),
	)

	dataDir := t.TempDir()
	if err := runMarket([]string{"--data-dir", dataDir, "install", indexPath, "demo"}); err != nil {
		t.Fatalf("runMarket install: %v", err)
	}

	manifest := filepath.Join(dataDir, "plugins", "demo", "demo.dsh-plugin.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("installed manifest not found: %v", err)
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := runMarket([]string{"--data-dir", dataDir, "list"})
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("runMarket list: %v", runErr)
	}
	if !strings.Contains(string(out), "demo") {
		t.Fatalf("list output missing demo plugin:\n%s", out)
	}
}

func TestRunMarketListEmpty(t *testing.T) {
	dataDir := t.TempDir()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := runMarket([]string{"--data-dir", dataDir, "list"})
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("runMarket list empty: %v", runErr)
	}
	if !strings.Contains(string(out), "No plugins installed") {
		t.Fatalf("list empty output unexpected:\n%s", out)
	}
}
