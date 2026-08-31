package security_test

import (
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/security"
)

func TestRedactBearerToken(t *testing.T) {
	cases := []string{
		"Authorization: Bearer sk-abcdefghijklmnopqrstuvwxyz123456",
		"Authorization: Bearer abc123",
	}
	for _, in := range cases {
		got := security.Redact(in)
		if strings.Contains(got, "sk-abcdefghijklmnopqrstuvwxyz123456") ||
			strings.Contains(got, "abc123") {
			t.Fatalf("bearer token leaked: %s", got)
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("expected redaction marker: %s", got)
		}
	}
}

func TestRedactAPIKeyKeyValue(t *testing.T) {
	cases := []string{
		"apiKey=abcdefghijklmnopqrstuvwxyz123456",
		`apiKey: "abcdefghijklmnopqrstuvwxyz123456"`,
		"token=AbCdEf1234567890AbCdEf1234567890",
		"client_secret: 1234567890abcdef1234567890abcdef",
		"password = hunter2secretvalue123456",
	}
	for _, in := range cases {
		got := security.Redact(in)
		if strings.Contains(got, "abcdefghijklmnopqrstuvwxyz123456") ||
			strings.Contains(got, "AbCdEf1234567890AbCdEf1234567890") ||
			strings.Contains(got, "1234567890abcdef1234567890abcdef") ||
			strings.Contains(got, "hunter2secretvalue123456") {
			t.Fatalf("secret leaked for %q -> %q", in, got)
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("expected redaction marker for %q -> %q", in, got)
		}
	}
}

func TestRedactKnownKeyPrefixes(t *testing.T) {
	cases := []string{
		"sk-1234567890abcdef1234567890abcdef",
		"ghp_1234567890abcdef1234567890abcdef123456",
		"AKIA1234567890ABCDEF12",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
	}
	for _, in := range cases {
		got := security.Redact(in)
		if strings.Contains(got, "1234567890abcdef1234567890abcdef") ||
			strings.Contains(got, "1234567890abcdef1234567890abcdef123456") ||
			strings.Contains(got, "AKIA1234567890ABCDEF12") ||
			strings.Contains(got, "eyJhbGciOiJIUzI1NiJ9") {
			t.Fatalf("known key leaked for %q -> %q", in, got)
		}
		if !strings.Contains(got, "***") {
			t.Fatalf("expected redaction marker for %q -> %q", in, got)
		}
	}
}

func TestRedactLongHighEntropyToken(t *testing.T) {
	in := "连接失败，token 是 AbCdEf1234567890AbCdEf1234567890，请检查"
	got := security.Redact(in)
	if strings.Contains(got, "AbCdEf1234567890AbCdEf1234567890") {
		t.Fatalf("long token leaked: %s", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("expected redaction marker: %s", got)
	}
}

func TestRedactLongLowercaseKey(t *testing.T) {
	in := "key abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"
	got := security.Redact(in)
	if strings.Contains(got, "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("long lowercase key leaked: %s", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("expected redaction marker: %s", got)
	}
}

func TestRedactLeavesNormalText(t *testing.T) {
	in := "hello world, this is a normal sentence with 1234567890 and no secrets."
	got := security.Redact(in)
	if got != in {
		t.Fatalf("normal text changed: %q -> %q", in, got)
	}
}

func TestRedactMapRecursively(t *testing.T) {
	m := map[string]any{
		"apiKey": "sk-1234567890abcdef1234567890abcdef",
		"nested": map[string]any{
			"token": "AbCdEf1234567890AbCdEf1234567890",
			"title": "普通标题",
			"credentials": map[string]any{
				"password": "hunter2secretvalue123456",
				"user":     "alice",
			},
		},
		"items": []any{
			map[string]any{"authorization": "Bearer sk-abcdefghijklmnopqrstuvwxyz123456"},
			"AbCdEf1234567890AbCdEf1234567890",
			"普通文本",
		},
		"safe": "keep-me",
	}

	security.RedactMap(m)

	if m["apiKey"] != "***" {
		t.Fatalf("apiKey not redacted: %#v", m["apiKey"])
	}
	nested, ok := m["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested not map: %#v", m["nested"])
	}
	if nested["token"] != "***" {
		t.Fatalf("nested token not redacted: %#v", nested["token"])
	}
	if nested["title"] != "普通标题" {
		t.Fatalf("nested title changed: %#v", nested["title"])
	}
	creds, ok := nested["credentials"].(map[string]any)
	if !ok {
		t.Fatalf("credentials not map: %#v", nested["credentials"])
	}
	if creds["password"] != "***" {
		t.Fatalf("password not redacted: %#v", creds["password"])
	}
	if creds["user"] != "alice" {
		t.Fatalf("user changed: %#v", creds["user"])
	}
	items, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("items not slice: %#v", m["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] not map: %#v", items[0])
	}
	if first["authorization"] != "***" {
		t.Fatalf("slice authorization not redacted: %#v", first["authorization"])
	}
	if items[1] != "***" {
		t.Fatalf("slice long token not redacted: %#v", items[1])
	}
	if items[2] != "普通文本" {
		t.Fatalf("slice normal text changed: %#v", items[2])
	}
	if m["safe"] != "keep-me" {
		t.Fatalf("safe value changed: %#v", m["safe"])
	}
}

func TestRedactMapRedactsNonSensitiveKeys(t *testing.T) {
	m := map[string]any{
		"message": "api key sk-1234567890abcdef1234567890abcdef",
		"content": "Authorization: Bearer AbCdEf1234567890AbCdEf1234567890",
		"safe":    "hello",
	}
	security.RedactMap(m)
	if strings.Contains(m["message"].(string), "sk-1234567890abcdef1234567890abcdef") {
		t.Fatalf("message secret leaked: %#v", m["message"])
	}
	if strings.Contains(m["content"].(string), "AbCdEf1234567890AbCdEf1234567890") {
		t.Fatalf("content secret leaked: %#v", m["content"])
	}
	if m["safe"] != "hello" {
		t.Fatalf("safe value changed: %#v", m["safe"])
	}
}

func TestRedactMapString(t *testing.T) {
	orig := map[string]string{
		"message": "sk-1234567890abcdef1234567890abcdef",
		"safe":    "hello",
	}
	got := security.RedactMapString(orig)
	if orig["message"] != "sk-1234567890abcdef1234567890abcdef" {
		t.Fatalf("original mutated: %#v", orig)
	}
	if strings.Contains(got["message"], "sk-1234567890abcdef1234567890abcdef") {
		t.Fatalf("redacted value leaked: %#v", got["message"])
	}
	if got["safe"] != "hello" {
		t.Fatalf("safe value changed: %#v", got["safe"])
	}
}
