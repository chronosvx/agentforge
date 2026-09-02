package ui

import (
	"strings"
	"testing"
	"time"
)

func TestUpdateTPSCalculatesPositiveTPS(t *testing.T) {
	m := Model{
		tokenUsage:     map[string]any{"outputTokens": 100},
		lastTokenTotal: 10,
		lastTokenAt:    time.Now().Add(-2 * time.Second),
	}

	m.updateTPS()

	if m.tps < 40 || m.tps > 50 {
		t.Fatalf("tps = %v, want ~45", m.tps)
	}
	if m.lastTokenTotal != 100 {
		t.Fatalf("lastTokenTotal = %d, want 100", m.lastTokenTotal)
	}
	if m.lastTokenAt.Before(time.Now().Add(-time.Second)) {
		t.Fatalf("lastTokenAt was not updated: %v", m.lastTokenAt)
	}
}

func TestUpdateTPSUsesContextAndInt(t *testing.T) {
	m := Model{
		context:        map[string]any{"outputTokens": 50},
		lastTokenTotal: 0,
		lastTokenAt:    time.Now().Add(-time.Second),
	}

	m.updateTPS()

	if m.tps < 40 || m.tps > 60 {
		t.Fatalf("tps = %v, want ~50", m.tps)
	}
	if m.lastTokenTotal != 50 {
		t.Fatalf("lastTokenTotal = %d, want 50", m.lastTokenTotal)
	}
}

func TestUpdateTPSOnlyRefreshesTimestampWhenNotIncreased(t *testing.T) {
	m := Model{
		tokenUsage:     map[string]any{"outputTokens": 80},
		lastTokenTotal: 100,
		lastTokenAt:    time.Now().Add(-time.Second),
		tps:            12.5,
	}

	m.updateTPS()

	if m.tps != 12.5 {
		t.Fatalf("tps changed to %v, want 12.5", m.tps)
	}
	if m.lastTokenTotal != 100 {
		t.Fatalf("lastTokenTotal = %d, want 100", m.lastTokenTotal)
	}
	if m.lastTokenAt.Before(time.Now().Add(-time.Second)) {
		t.Fatalf("lastTokenAt was not updated: %v", m.lastTokenAt)
	}
}

func TestRenderHUDIncludesTokensCostTPS(t *testing.T) {
	m := Model{
		tokenUsage: map[string]any{"outputTokens": 123},
		cost:       map[string]any{"estimatedYuan": 0.012},
		tps:        45.6,
	}

	got := m.renderHUD()
	for _, want := range []string{"tokens", "123", "cost", "0.012", "tps", "45.6"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderHUD = %q, missing %q", got, want)
		}
	}
}

func TestRenderHUDShowsPlaceholderWhenNoData(t *testing.T) {
	m := Model{}

	got := m.renderHUD()
	if !strings.Contains(got, "—") {
		t.Fatalf("renderHUD = %q, want placeholder —", got)
	}
}
