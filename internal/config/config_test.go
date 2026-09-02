package config_test

import (
	"testing"

	"github.com/chronosvx/agentforge/internal/config"
)

func TestParseAPIURL(t *testing.T) {
	cfg, err := config.Parse([]string{"--api-url", "http://127.0.0.1:54695", "--mock=false"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.APIURL != "http://127.0.0.1:54695" {
		t.Fatalf("unexpected api url: %q", cfg.APIURL)
	}
	if cfg.DataDir == "" {
		t.Fatal("expected data dir to expand")
	}
}

func TestParseCostLimit(t *testing.T) {
	cfg, err := config.Parse([]string{"--cost-limit", "10.5"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.CostLimit != 10.5 {
		t.Fatalf("unexpected cost limit: %v", cfg.CostLimit)
	}
}

func TestParseTheme(t *testing.T) {
	cfg, err := config.Parse([]string{"--theme", "catppuccin"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Theme != "catppuccin" {
		t.Fatalf("unexpected theme: %q", cfg.Theme)
	}
}

func TestParseMockFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--mock"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Mock {
		t.Fatal("expected mock true")
	}
}

func TestParseAPIToken(t *testing.T) {
	cfg, err := config.Parse([]string{"--api-token", "secret-token"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.APIToken != "secret-token" {
		t.Fatalf("unexpected api token: %q", cfg.APIToken)
	}
}

func TestParseRemote(t *testing.T) {
	cfg, err := config.Parse([]string{"--remote", "--api-url", "https://dsh.example.com"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Remote {
		t.Fatal("expected remote true")
	}
	if cfg.APIURL != "https://dsh.example.com" {
		t.Fatalf("unexpected api url: %q", cfg.APIURL)
	}
}

func TestParseProviderFlags(t *testing.T) {
	cfg, err := config.Parse([]string{
		"--provider", "deepseek",
		"--provider-key", "key-a,key-b",
		"--provider-base-url", "https://example.com/v1",
		"--model", "deepseek-chat",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Provider != "deepseek" {
		t.Fatalf("Provider = %q, want deepseek", cfg.Provider)
	}
	if cfg.ProviderKey != "key-a,key-b" {
		t.Fatalf("ProviderKey = %q, want key-a,key-b", cfg.ProviderKey)
	}
	if cfg.ProviderBaseURL != "https://example.com/v1" {
		t.Fatalf("ProviderBaseURL = %q, want https://example.com/v1", cfg.ProviderBaseURL)
	}
	if cfg.Model != "deepseek-chat" {
		t.Fatalf("Model = %q, want deepseek-chat", cfg.Model)
	}
}

func TestParseKeychainAndSecretKeyFlags(t *testing.T) {
	cfg, err := config.Parse([]string{
		"--keychain",
		"--secret-key", "0123456789abcdef0123456789abcdef",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Keychain {
		t.Fatal("expected Keychain true")
	}
	if cfg.SecretKey != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("SecretKey = %q, want test key", cfg.SecretKey)
	}
}

func TestParseKeychainDefaultsFalse(t *testing.T) {
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Keychain {
		t.Fatal("expected Keychain false by default")
	}
	if cfg.SecretKey != "" {
		t.Fatalf("SecretKey = %q, want empty by default", cfg.SecretKey)
	}
}

func TestParseProviderDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Parse([]string{"--provider", "ollama"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Provider != "ollama" {
		t.Fatalf("Provider = %q, want ollama", cfg.Provider)
	}
	if cfg.ProviderBaseURL != "" || cfg.Model != "" || cfg.ProviderKey != "" {
		t.Fatalf("expected empty optional provider fields, got %+v", cfg)
	}
}

func TestParseLang(t *testing.T) {
	cfg, err := config.Parse([]string{"--lang", "en"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Lang != "en" {
		t.Fatalf("Lang = %q, want en", cfg.Lang)
	}
}

func TestParseLangDefault(t *testing.T) {
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Lang != "zh" {
		t.Fatalf("Lang = %q, want zh", cfg.Lang)
	}
}

func TestParseConfirmDataExport(t *testing.T) {
	cfg, err := config.Parse([]string{"--confirm-data-export"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.ConfirmDataExport {
		t.Fatal("expected ConfirmDataExport true")
	}
}

func TestParseLangInvalid(t *testing.T) {
	if _, err := config.Parse([]string{"--lang", "fr"}); err == nil {
		t.Fatal("expected error for unsupported lang")
	}
}

func TestParseTelemetryFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--telemetry"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Telemetry {
		t.Fatal("expected telemetry true")
	}
}

func TestParseTelemetryDefaultsFalse(t *testing.T) {
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Telemetry {
		t.Fatal("expected telemetry false by default")
	}
}

func TestParseProviderKeyStdinFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--provider-key-stdin"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.ProviderKeyStdin {
		t.Fatal("expected ProviderKeyStdin true")
	}
}

func TestParseEncryptSessionsFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--encrypt-sessions"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.EncryptSessions {
		t.Fatal("expected EncryptSessions true")
	}
}

func TestParseNoColorFlag(t *testing.T) {
	cfg, err := config.Parse([]string{"--no-color"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.NoColor {
		t.Fatal("expected NoColor true")
	}
}

func TestParseNoColorDefaultsFalse(t *testing.T) {
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.NoColor {
		t.Fatal("expected NoColor false by default")
	}
}

func TestParseNoColorEnv(t *testing.T) {
	t.Setenv("DSH_NO_COLOR", "true")
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.NoColor {
		t.Fatal("expected NoColor true from DSH_NO_COLOR")
	}
}
