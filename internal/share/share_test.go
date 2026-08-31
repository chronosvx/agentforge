package share_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/session"
	"github.com/chronosvx/agentforge/internal/share"
)

func TestShareSessionOpenShareRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	entry := session.Entry{
		ID:        "s1",
		Workspace: "work",
		Title:     "shared session",
		CreatedAt: time.Unix(1000, 0),
		UpdatedAt: time.Unix(1001, 0),
		Messages:  []client.Message{{Role: "user", Content: "hello"}},
		Context:   map[string]any{"cwd": "/tmp"},
	}

	link, err := share.ShareSession(entry, key)
	if err != nil {
		t.Fatalf("share session: %v", err)
	}
	if !strings.HasPrefix(link, share.Scheme) {
		t.Fatalf("link %q does not start with %q", link, share.Scheme)
	}

	got, err := share.OpenShare(link, key)
	if err != nil {
		t.Fatalf("open share: %v", err)
	}
	if got.ID != entry.ID || got.Title != entry.Title || got.Workspace != entry.Workspace {
		t.Fatalf("unexpected entry: %+v", got)
	}
	if !got.UpdatedAt.Equal(entry.UpdatedAt) {
		t.Fatalf("UpdatedAt mismatch: got %v, want %v", got.UpdatedAt, entry.UpdatedAt)
	}
	if len(got.Messages) != 1 || got.Messages[0] != entry.Messages[0] {
		t.Fatalf("messages mismatch: got %#v, want %#v", got.Messages, entry.Messages)
	}
}

func TestOpenShareWrongKeyFails(t *testing.T) {
	t.Setenv(share.EnvKey, "")
	key := bytes.Repeat([]byte{0x11}, 32)
	wrong := bytes.Repeat([]byte{0x22}, 32)
	entry := session.Entry{ID: "s1", Title: "secret"}

	link, err := share.ShareSession(entry, key)
	if err != nil {
		t.Fatalf("share session: %v", err)
	}

	if _, err := share.OpenShare(link, wrong); err == nil {
		t.Fatalf("expected error when opening with wrong key")
	}
	if _, err := share.OpenShare(link, nil); err == nil {
		t.Fatalf("expected error when opening encrypted payload without key")
	}
}

func TestShareSessionPlaintextMode(t *testing.T) {
	t.Setenv(share.EnvKey, "")
	entry := session.Entry{ID: "plain", Title: "plain session", UpdatedAt: time.Unix(2000, 0)}

	link, err := share.ShareSession(entry, nil)
	if err != nil {
		t.Fatalf("share session plaintext: %v", err)
	}
	if !strings.HasPrefix(link, share.Scheme) {
		t.Fatalf("link %q does not start with %q", link, share.Scheme)
	}

	got, err := share.OpenShare(link, nil)
	if err != nil {
		t.Fatalf("open plaintext share: %v", err)
	}
	if got.ID != entry.ID || got.Title != entry.Title {
		t.Fatalf("unexpected plaintext entry: %+v", got)
	}

	// 明文链接即使传了密钥也应能打开（密钥仅用于解密加密负载）。
	key := bytes.Repeat([]byte{0x33}, 32)
	gotWithKey, err := share.OpenShare(link, key)
	if err != nil {
		t.Fatalf("open plaintext share with key: %v", err)
	}
	if gotWithKey.ID != entry.ID {
		t.Fatalf("unexpected plaintext entry with key: %+v", gotWithKey)
	}
}

func TestOpenShareAcceptsPayloadWithoutScheme(t *testing.T) {
	key := bytes.Repeat([]byte{0x44}, 32)
	entry := session.Entry{ID: "no-scheme", Title: "bare"}

	link, err := share.ShareSession(entry, key)
	if err != nil {
		t.Fatalf("share session: %v", err)
	}
	bare := strings.TrimPrefix(link, share.Scheme)

	got, err := share.OpenShare(bare, key)
	if err != nil {
		t.Fatalf("open bare payload: %v", err)
	}
	if got.ID != entry.ID {
		t.Fatalf("unexpected entry: %+v", got)
	}
}

func TestShareUsesEnvKeyWhenSecretKeyIsNil(t *testing.T) {
	key := bytes.Repeat([]byte{0x55}, 32)
	t.Setenv(share.EnvKey, hex.EncodeToString(key))
	entry := session.Entry{ID: "env", Title: "env key session"}

	link, err := share.ShareSession(entry, nil)
	if err != nil {
		t.Fatalf("share session with env key: %v", err)
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(link, share.Scheme))
	if err != nil {
		t.Fatalf("decode share payload: %v", err)
	}
	if !strings.Contains(string(rawPayload), `"plain":false`) {
		t.Fatalf("expected encrypted envelope when DSH_SHARE_KEY is set, got %q", rawPayload)
	}

	got, err := share.OpenShare(link, nil)
	if err != nil {
		t.Fatalf("open share with env key: %v", err)
	}
	if got.ID != entry.ID || got.Title != entry.Title {
		t.Fatalf("unexpected entry: %+v", got)
	}
}
