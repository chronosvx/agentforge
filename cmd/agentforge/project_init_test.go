package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunProjectInitGeneratesAGENTMd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	if err := runProjectInit([]string{"--dir", dir}); err != nil {
		t.Fatalf("runProjectInit: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENT.md"))
	if err != nil {
		t.Fatalf("read AGENT.md: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "go") {
		t.Fatalf("AGENT.md should mention Go/go.mod, got:\n%s", content)
	}
	if !strings.Contains(content, filepath.Base(dir)) {
		t.Fatalf("AGENT.md should mention project name %q, got:\n%s", filepath.Base(dir), content)
	}
}

func TestRunProjectInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(path, []byte("old content\n"), 0o644); err != nil {
		t.Fatalf("write existing AGENT.md: %v", err)
	}

	if err := runProjectInit([]string{"--dir", dir}); err == nil {
		t.Fatal("expected error when AGENT.md exists without --force")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing AGENT.md: %v", err)
	}
	if string(data) != "old content\n" {
		t.Fatalf("AGENT.md was modified without --force: %q", string(data))
	}

	if err := runProjectInit([]string{"--dir", dir, "--force"}); err != nil {
		t.Fatalf("runProjectInit --force: %v", err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read overwritten AGENT.md: %v", err)
	}
	if string(data) == "old content\n" {
		t.Fatalf("AGENT.md was not overwritten by --force")
	}
}
