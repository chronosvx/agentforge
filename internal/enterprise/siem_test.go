package enterprise_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/enterprise"
)

func TestLogSinkEmitWritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	sink, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	event := enterprise.SIEMEvent{
		Time:     now,
		User:     "alice",
		Action:   "file.read",
		Resource: "/etc/hosts",
		Result:   "denied",
	}
	if err := sink.Emit(event); err != nil {
		t.Fatalf("emit: %v", err)
	}

	data, err := os.ReadFile(sink.Path())
	if err != nil {
		t.Fatalf("read siem log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), data)
	}

	var got enterprise.SIEMEvent
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("decode line: %v", err)
	}
	if !got.Time.Equal(now) {
		t.Fatalf("time = %v, want %v", got.Time, now)
	}
	if got.User != event.User || got.Action != event.Action || got.Resource != event.Resource || got.Result != event.Result {
		t.Fatalf("event mismatch: got %+v, want %+v", got, event)
	}
}

func TestLogSinkAppendsMultipleEvents(t *testing.T) {
	dir := t.TempDir()
	sink, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}

	for i := 0; i < 3; i++ {
		event := enterprise.SIEMEvent{
			Time:     time.Unix(int64(i), 0).UTC(),
			User:     "u",
			Action:   "action",
			Resource: "resource",
			Result:   "ok",
		}
		if err := sink.Emit(event); err != nil {
			t.Fatalf("emit %d: %v", i, err)
		}
	}

	data, err := os.ReadFile(sink.Path())
	if err != nil {
		t.Fatalf("read siem log: %v", err)
	}
	if got := strings.Count(strings.TrimSpace(string(data)), "\n"); got != 2 {
		t.Fatalf("line breaks = %d, want 2 for 3 events", got)
	}
}

func TestLogSinkDefaultsZeroTimeToNow(t *testing.T) {
	dir := t.TempDir()
	sink, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}

	before := time.Now().UTC()
	if err := sink.Emit(enterprise.SIEMEvent{User: "u", Action: "a"}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	after := time.Now().UTC()

	data, err := os.ReadFile(sink.Path())
	if err != nil {
		t.Fatalf("read siem log: %v", err)
	}
	var got enterprise.SIEMEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Time.Before(before) || got.Time.After(after) {
		t.Fatalf("default time = %v, want between %v and %v", got.Time, before, after)
	}
}

func TestLogSinkPersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	sink, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	event := enterprise.SIEMEvent{
		Time:     time.Unix(100, 0).UTC(),
		User:     "bob",
		Action:   "kms.decrypt",
		Resource: "secret",
		Result:   "ok",
	}
	if err := sink.Emit(event); err != nil {
		t.Fatalf("emit: %v", err)
	}

	again, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink again: %v", err)
	}
	if err := again.Emit(event); err != nil {
		t.Fatalf("emit again: %v", err)
	}

	data, err := os.ReadFile(again.Path())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n"); lines != 1 {
		t.Fatalf("got %d line breaks, want 1 (2 events)", lines)
	}
}

func TestLogSinkPath(t *testing.T) {
	dir := t.TempDir()
	sink, err := enterprise.NewLogSink(dir)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if got := sink.Path(); got != filepath.Join(dir, "siem.log") {
		t.Fatalf("path = %q, want %q", got, filepath.Join(dir, "siem.log"))
	}
}
