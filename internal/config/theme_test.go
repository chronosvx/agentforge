package config_test

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/config"
)

func TestParseThemeInvalid(t *testing.T) {
	_, err := config.Parse([]string{"--theme", "gruvbox"})
	if err == nil {
		t.Fatal("expected error for unsupported theme")
	}
	if !strings.Contains(err.Error(), "invalid --theme") {
		t.Fatalf("expected invalid --theme error, got: %v", err)
	}
}

func TestParseThemeSupported(t *testing.T) {
	themes := []string{
		"default",
		"catppuccin",
		"dracula",
		"tokyo-night",
		"nord",
		"solarized",
	}
	for _, theme := range themes {
		cfg, err := config.Parse([]string{"--theme", theme})
		if err != nil {
			t.Fatalf("parse theme %q: %v", theme, err)
		}
		if cfg.Theme != theme {
			t.Fatalf("Theme = %q, want %q", cfg.Theme, theme)
		}
	}
}
