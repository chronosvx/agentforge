package security_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/security"
)

func TestAuditLogAppendWritesRedactedJSONLines(t *testing.T) {
	dir := t.TempDir()
	log, err := security.NewAuditLog(dir)
	if err != nil {
		t.Fatalf("new audit log: %v", err)
	}

	fields := map[string]any{
		"sessionId": "s1",
		"token":     "AbCdEf1234567890AbCdEf1234567890",
		"nested": map[string]any{
			"apiKey": "sk-1234567890abcdef1234567890abcdef",
			"title":  "普通标题",
		},
	}
	if err := log.Append("session.saved", fields); err != nil {
		t.Fatalf("append: %v", err)
	}

	// 原始 map 不应被修改。
	if fields["token"] != "AbCdEf1234567890AbCdEf1234567890" {
		t.Fatalf("append mutated caller map: %#v", fields["token"])
	}

	path := filepath.Join(dir, "audit.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if strings.Contains(string(data), "AbCdEf1234567890AbCdEf1234567890") {
		t.Fatalf("audit log leaked token: %s", data)
	}
	if strings.Contains(string(data), "sk-1234567890abcdef1234567890abcdef") {
		t.Fatalf("audit log leaked api key: %s", data)
	}
	if !strings.Contains(string(data), "***") {
		t.Fatalf("audit log missing redaction marker: %s", data)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		t.Fatalf("expected at least one audit line")
	}
	var rec map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
		t.Fatalf("decode audit line: %v", err)
	}
	if rec["event"] != "session.saved" {
		t.Fatalf("event = %#v, want session.saved", rec["event"])
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

func TestAuditLogAppendsMultipleLines(t *testing.T) {
	dir := t.TempDir()
	log, err := security.NewAuditLog(dir)
	if err != nil {
		t.Fatalf("new audit log: %v", err)
	}

	if err := log.Append("one", nil); err != nil {
		t.Fatalf("append one: %v", err)
	}
	if err := log.Append("two", map[string]any{"secret": "AbCdEf1234567890AbCdEf1234567890"}); err != nil {
		t.Fatalf("append two: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %s", len(lines), data)
	}
	if !strings.Contains(lines[1], "***") {
		t.Fatalf("second line not redacted: %s", lines[1])
	}
}

func TestAuditLogRejectsEmptyDataDir(t *testing.T) {
	if _, err := security.NewAuditLog(""); err == nil {
		t.Fatalf("expected error for empty data dir")
	}
}

func TestAuditLogCanUseDataDirFieldDirectly(t *testing.T) {
	dir := t.TempDir()
	log := &security.AuditLog{DataDir: dir}
	if err := log.Append("direct", nil); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), `"event":"direct"`) {
		t.Fatalf("unexpected audit line: %s", data)
	}
}
