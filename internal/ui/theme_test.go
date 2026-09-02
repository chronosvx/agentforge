package ui

import (
	"testing"

	"github.com/chronosvx/agentforge/internal/config"
)

func TestAccentColorForSupportedThemes(t *testing.T) {
	tests := []struct {
		theme string
		want  string
	}{
		{theme: "default", want: "6"},
		{theme: "catppuccin", want: "5"},
		{theme: "dracula", want: "5"},
		{theme: "tokyo-night", want: "4"},
		{theme: "nord", want: "6"},
		{theme: "solarized", want: "3"},
	}

	for _, tt := range tests {
		m := Model{cfg: &config.Config{Theme: tt.theme}}
		if got := m.accentColor(); got != tt.want {
			t.Errorf("accentColor(%q) = %q, want %q", tt.theme, got, tt.want)
		}
	}
}

func TestAccentColorWithoutConfigDefaultsToSix(t *testing.T) {
	m := Model{}
	if got := m.accentColor(); got != "6" {
		t.Fatalf("accentColor() without cfg = %q, want 6", got)
	}
}
