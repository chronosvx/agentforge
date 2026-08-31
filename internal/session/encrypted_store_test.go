package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/session"
)

func testEncryptionKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	return key
}

func TestLocalEncryptedStoreSaveLoadRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}

	entry := session.Entry{
		ID:        "s1",
		Title:     "secret session",
		UpdatedAt: time.Unix(100, 0),
	}
	if err := store.Save(entry); err != nil {
		t.Fatalf("save: %v", err)
	}

	encPath := filepath.Join(dataDir, "sessions", "s1.json.enc")
	plainPath := filepath.Join(dataDir, "sessions", "s1.json")

	if _, err := os.Stat(encPath); err != nil {
		t.Fatalf("encrypted file not created: %v", err)
	}
	if _, err := os.Stat(plainPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext file should not exist, stat err = %v", err)
	}

	sealed, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read encrypted file: %v", err)
	}
	if string(sealed) == `{"id":"s1","title":"secret session"}` {
		t.Fatalf("stored file contains plaintext JSON")
	}
	if contains(sealed, []byte("secret session")) {
		t.Fatalf("stored file leaks plaintext content")
	}

	got, err := store.Load("s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.ID != entry.ID || got.Title != entry.Title {
		t.Fatalf("roundtrip mismatch: got %+v, want %+v", got, entry)
	}
}

func TestLocalEncryptedStoreWrongKeyLoadFails(t *testing.T) {
	dataDir := t.TempDir()
	key := testEncryptionKey()

	store, err := session.NewEncryptedStore(dataDir, key)
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}
	if err := store.Save(session.Entry{ID: "s1", Title: "top secret"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	wrongKey := make([]byte, len(key))
	for i := range wrongKey {
		wrongKey[i] = key[i] ^ 0xff
	}
	wrongStore, err := session.NewEncryptedStore(dataDir, wrongKey)
	if err != nil {
		t.Fatalf("new wrong-key store: %v", err)
	}
	if _, err := wrongStore.Load("s1"); err == nil {
		t.Fatalf("expected load with wrong key to fail")
	}
}

func TestLocalEncryptedStoreListEncryptedSessions(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}

	base := time.Unix(200, 0)
	if err := store.Save(session.Entry{ID: "b", Title: "B", UpdatedAt: base.Add(time.Second)}); err != nil {
		t.Fatalf("save b: %v", err)
	}
	if err := store.Save(session.Entry{ID: "a", Title: "A", UpdatedAt: base}); err != nil {
		t.Fatalf("save a: %v", err)
	}

	got, err := store.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %+v", len(got), got)
	}
	if got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("unexpected order: %+v", got)
	}

	// Reopen with same key and ensure encrypted files are still listed.
	reopened, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("reopen encrypted store: %v", err)
	}
	gotAgain, err := reopened.List()
	if err != nil {
		t.Fatalf("reopen list: %v", err)
	}
	if len(gotAgain) != 2 {
		t.Fatalf("reopen expected 2 sessions, got %d", len(gotAgain))
	}
}

func TestLocalEncryptedStoreListRecognizesLegacyPlaintext(t *testing.T) {
	dataDir := t.TempDir()
	plainStore, err := session.NewStore(dataDir)
	if err != nil {
		t.Fatalf("new plain store: %v", err)
	}
	if err := plainStore.Save(session.Entry{ID: "legacy", Title: "old", UpdatedAt: time.Unix(1, 0)}); err != nil {
		t.Fatalf("save legacy: %v", err)
	}

	encStore, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}
	got, err := encStore.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != "legacy" || got[0].Title != "old" {
		t.Fatalf("legacy plaintext not listed: %+v", got)
	}

	loaded, err := encStore.Load("legacy")
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if loaded.Title != "old" {
		t.Fatalf("legacy title = %q, want %q", loaded.Title, "old")
	}
}

func TestLocalEncryptedStoreDeleteRemovesBothFileTypes(t *testing.T) {
	dataDir := t.TempDir()
	store, err := session.NewEncryptedStore(dataDir, testEncryptionKey())
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}
	if err := store.Save(session.Entry{ID: "s1", Title: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Simulate a leftover plaintext file from an older version.
	plainPath := filepath.Join(dataDir, "sessions", "s1.json")
	if err := os.WriteFile(plainPath, []byte(`{"id":"s1","title":"old"}`), 0o600); err != nil {
		t.Fatalf("write legacy plaintext: %v", err)
	}

	if err := store.Delete("s1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "sessions", "s1.json.enc")); !os.IsNotExist(err) {
		t.Fatalf("encrypted file still exists after delete: %v", err)
	}
	if _, err := os.Stat(plainPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext file still exists after delete: %v", err)
	}
}

func contains(data, sub []byte) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(data); i++ {
		if string(data[i:i+len(sub)]) == string(sub) {
			return true
		}
	}
	return false
}
