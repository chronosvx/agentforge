package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chronosvx/agentforge/internal/config"
)

func TestLoadFileJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"api-url":"http://127.0.0.1:1234","lang":"en"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	kv, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if kv["api-url"] != "http://127.0.0.1:1234" || kv["lang"] != "en" {
		t.Fatalf("unexpected kv: %#v", kv)
	}
}

func TestLoadFileLineFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.txt")
	if err := os.WriteFile(path, []byte("api-url = http://127.0.0.1:5678\n# comment\nlang=en\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	kv, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if kv["api-url"] != "http://127.0.0.1:5678" || kv["lang"] != "en" {
		t.Fatalf("unexpected kv: %#v", kv)
	}
}

func TestParseConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"api-url":"http://127.0.0.1:9999","theme":"catppuccin"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse([]string{"--config", path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://127.0.0.1:9999" || cfg.Theme != "catppuccin" {
		t.Fatalf("unexpected cfg: %#v", cfg)
	}
}

func TestParseCLIOverridesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"api-url":"http://127.0.0.1:9999"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse([]string{"--config", path, "--api-url", "http://127.0.0.1:8888"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://127.0.0.1:8888" {
		t.Fatalf("CLI should override file: %q", cfg.APIURL)
	}
}

func TestParseMissingConfigIgnored(t *testing.T) {
	cfg, err := config.Parse([]string{"--config", filepath.Join(t.TempDir(), "nope.json")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "" {
		t.Fatalf("unexpected api url: %q", cfg.APIURL)
	}
}
