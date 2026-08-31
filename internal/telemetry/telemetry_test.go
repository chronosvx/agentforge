package telemetry_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/telemetry"
)

func TestEnabledDefaultsToFalse(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "")
	telemetry.SetEnabled(false)

	if telemetry.Enabled() {
		t.Fatal("telemetry should be disabled by default")
	}
}

func TestEnabledByEnvironment(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "1")
	telemetry.SetEnabled(false)

	if !telemetry.Enabled() {
		t.Fatal("telemetry should be enabled by DSH_TELEMETRY=1")
	}
}

func TestEnabledBySetEnabled(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "")
	telemetry.SetEnabled(true)

	if !telemetry.Enabled() {
		t.Fatal("telemetry should be enabled by SetEnabled(true)")
	}
}

func TestRecordDisabledIsNoop(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "")
	telemetry.SetEnabled(false)
	dir := t.TempDir()
	if err := telemetry.SetDataDir(dir); err != nil {
		t.Fatalf("set data dir: %v", err)
	}

	if err := telemetry.Record("app.started", map[string]any{"n": 1}); err != nil {
		t.Fatalf("record when disabled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "telemetry.log")); !os.IsNotExist(err) {
		t.Fatalf("telemetry.log should not be created when disabled: %v", err)
	}
}

func TestRecordWritesRedactedJSONLines(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "")
	telemetry.SetEnabled(true)
	dir := t.TempDir()
	if err := telemetry.SetDataDir(dir); err != nil {
		t.Fatalf("set data dir: %v", err)
	}

	fields := map[string]any{
		"sessionId": "s1",
		"token":     "AbCdEf1234567890AbCdEf1234567890",
		"nested": map[string]any{
			"apiKey": "sk-1234567890abcdef1234567890abcdef",
			"title":  "普通标题",
		},
	}
	if err := telemetry.Record("app.started", fields); err != nil {
		t.Fatalf("record: %v", err)
	}

	// 原始 map 不应被修改。
	if fields["token"] != "AbCdEf1234567890AbCdEf1234567890" {
		t.Fatalf("record mutated caller map: %#v", fields["token"])
	}

	path := filepath.Join(dir, "telemetry.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry log: %v", err)
	}
	if strings.Contains(string(data), "AbCdEf1234567890AbCdEf1234567890") {
		t.Fatalf("telemetry log leaked token: %s", data)
	}
	if strings.Contains(string(data), "sk-1234567890abcdef1234567890abcdef") {
		t.Fatalf("telemetry log leaked api key: %s", data)
	}
	if !strings.Contains(string(data), "***") {
		t.Fatalf("telemetry log missing redaction marker: %s", data)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		t.Fatal("expected at least one telemetry line")
	}
	var rec map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
		t.Fatalf("decode telemetry line: %v", err)
	}
	if rec["event"] != "app.started" {
		t.Fatalf("event = %#v, want app.started", rec["event"])
	}
	if _, ok := rec["time"]; !ok {
		t.Fatalf("missing time field: %#v", rec)
	}
	fieldsOut, ok := rec["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields not object: %#v", rec["fields"])
	}
	if fieldsOut["token"] != "***" {
		t.Fatalf("fields token = %#v, want ***", fieldsOut["token"])
	}
}

func TestRecordAppendsMultipleLines(t *testing.T) {
	t.Setenv(telemetry.EnvEnable, "")
	telemetry.SetEnabled(true)
	dir := t.TempDir()
	if err := telemetry.SetDataDir(dir); err != nil {
		t.Fatalf("set data dir: %v", err)
	}

	if err := telemetry.Record("one", nil); err != nil {
		t.Fatalf("record one: %v", err)
	}
	if err := telemetry.Record("two", map[string]any{"secret": "AbCdEf1234567890AbCdEf1234567890"}); err != nil {
		t.Fatalf("record two: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "telemetry.log"))
	if err != nil {
		t.Fatalf("read telemetry log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %s", len(lines), data)
	}
	if !strings.Contains(lines[1], "***") {
		t.Fatalf("second line not redacted: %s", lines[1])
	}
}

func TestSetDataDirRejectsEmpty(t *testing.T) {
	if err := telemetry.SetDataDir(""); err == nil {
		t.Fatal("expected error for empty data dir")
	}
}
