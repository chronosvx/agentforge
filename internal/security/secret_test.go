package security

import (
	"bytes"
	"encoding/hex"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func hexKey(t *testing.T, b byte, n int) string {
	t.Helper()
	return hex.EncodeToString(bytes.Repeat([]byte{b}, n))
}

func TestEncryptedFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	key := hexKey(t, 0x42, 32)

	store, err := NewEncryptedFileStore(dir, key)
	if err != nil {
		t.Fatalf("NewEncryptedFileStore: %v", err)
	}

	if _, err := store.Get("svc", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing error = %v, want ErrNotFound", err)
	}

	if err := store.Set("svc", "acct", "sk-123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-123" {
		t.Fatalf("Get = %q, want sk-123", got)
	}

	// 重复 Set 应覆盖旧值。
	if err := store.Set("svc", "acct", "sk-456"); err != nil {
		t.Fatalf("Set duplicate: %v", err)
	}
	got, err = store.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get after duplicate: %v", err)
	}
	if got != "sk-456" {
		t.Fatalf("Get after duplicate = %q, want sk-456", got)
	}

	if err := store.Delete("svc", "acct"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("svc", "acct"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete error = %v, want ErrNotFound", err)
	}

	// 幂等删除。
	if err := store.Delete("svc", "acct"); err != nil {
		t.Fatalf("Delete missing should be nil, got %v", err)
	}
}

func TestEncryptedFileStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	key := hexKey(t, 0x11, 32)

	store, err := NewEncryptedFileStore(dir, key)
	if err != nil {
		t.Fatalf("NewEncryptedFileStore: %v", err)
	}
	if err := store.Set("agentforge", "openai", "sk-persist"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reopened, err := NewEncryptedFileStore(dir, key)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	got, err := reopened.Get("agentforge", "openai")
	if err != nil {
		t.Fatalf("Reopened Get: %v", err)
	}
	if got != "sk-persist" {
		t.Fatalf("Reopened Get = %q, want sk-persist", got)
	}
}

func TestEncryptedFileStoreWrongKeyFails(t *testing.T) {
	dir := t.TempDir()
	keyA := hexKey(t, 0xAA, 32)
	keyB := hexKey(t, 0xBB, 32)

	storeA, err := NewEncryptedFileStore(dir, keyA)
	if err != nil {
		t.Fatalf("NewEncryptedFileStore A: %v", err)
	}
	if err := storeA.Set("svc", "acct", "top-secret"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	storeB, err := NewEncryptedFileStore(dir, keyB)
	if err != nil {
		t.Fatalf("NewEncryptedFileStore B: %v", err)
	}
	if _, err := storeB.Get("svc", "acct"); err == nil {
		t.Fatal("expected error when reading with wrong master key")
	}
}

func TestEncryptedFileStoreRequiresKey(t *testing.T) {
	t.Setenv(EnvSecretKey, "")
	dir := t.TempDir()

	if _, err := NewEncryptedFileStore(dir, ""); !errors.Is(err, ErrNoSecretKey) {
		t.Fatalf("NewEncryptedFileStore without key error = %v, want ErrNoSecretKey", err)
	}
	if _, err := NewEncryptedFileStore(dir, "not-a-valid-key"); err == nil {
		t.Fatal("expected error for invalid master key")
	}
}

func TestEncryptedFileStoreEnvKey(t *testing.T) {
	dir := t.TempDir()
	key := hexKey(t, 0x33, 32)
	t.Setenv(EnvSecretKey, key)

	store, err := NewEncryptedFileStore(dir, "")
	if err != nil {
		t.Fatalf("NewEncryptedFileStore with env: %v", err)
	}
	if err := store.Set("svc", "acct", "env-key"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "env-key" {
		t.Fatalf("Get = %q, want env-key", got)
	}
}

func TestNewStoreSelectsEncryptedFile(t *testing.T) {
	dir := t.TempDir()
	key := hexKey(t, 0x44, 32)
	t.Setenv(EnvSecretKey, key)

	s, err := NewStore(dir, false)
	if err != nil {
		t.Fatalf("NewStore(false): %v", err)
	}
	if _, ok := s.(*EncryptedFileStore); !ok {
		t.Fatalf("NewStore(false) type = %T, want *EncryptedFileStore", s)
	}
	if err := s.Set("svc", "acct", "value"); err != nil {
		t.Fatalf("Set via NewStore: %v", err)
	}
	got, err := s.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get via NewStore: %v", err)
	}
	if got != "value" {
		t.Fatalf("Get via NewStore = %q, want value", got)
	}
}

func TestNewKeychainStoreRejectsUnsupportedPlatform(t *testing.T) {
	if _, err := newKeychainStore("plan9"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("newKeychainStore(plan9) error = %v, want ErrUnsupported", err)
	}
}

func TestKeychainStoreNotFound(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "linux"} {
		t.Run(goos, func(t *testing.T) {
			k := &KeychainStore{
				platform: goos,
				runner: func(name string, args ...string) ([]byte, error) {
					return []byte("item not found"), errors.New("exit status 1")
				},
			}
			if _, err := k.Get("svc", "acct"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("Get missing error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestKeychainStoreDarwinCommands(t *testing.T) {
	testKeychainCommands(t, "darwin")
}

func TestKeychainStoreWindowsCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows Credential Manager API does not use the command runner")
	}
	testKeychainCommands(t, "windows")
}

func TestKeychainStoreLinuxCommands(t *testing.T) {
	testKeychainCommands(t, "linux")
}

func testKeychainCommands(t *testing.T, goos string) {
	t.Helper()

	var calls [][]string
	runner := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		if name == "security" || name == "powershell" || name == "secret-tool" {
			// find/lookup/get should return a value.
			return []byte("stored-value\n"), nil
		}
		return nil, nil
	}
	k := &KeychainStore{runner: runner, platform: goos}

	if err := k.Set("agentforge", "openai", "sk-123"); err != nil {
		t.Fatalf("%s Set: %v", goos, err)
	}
	got, err := k.Get("agentforge", "openai")
	if err != nil {
		t.Fatalf("%s Get: %v", goos, err)
	}
	if got != "stored-value" {
		t.Fatalf("%s Get = %q, want stored-value", goos, got)
	}
	if err := k.Delete("agentforge", "openai"); err != nil {
		t.Fatalf("%s Delete: %v", goos, err)
	}

	if len(calls) != 3 {
		t.Fatalf("%s call count = %d, want 3", goos, len(calls))
	}

	switch goos {
	case "darwin":
		if calls[0][0] != "security" || calls[0][1] != "add-generic-password" {
			t.Fatalf("darwin Set call = %v, want security add-generic-password", calls[0])
		}
		if calls[1][0] != "security" || calls[1][1] != "find-generic-password" {
			t.Fatalf("darwin Get call = %v, want security find-generic-password", calls[1])
		}
		if calls[2][0] != "security" || calls[2][1] != "delete-generic-password" {
			t.Fatalf("darwin Delete call = %v, want security delete-generic-password", calls[2])
		}
	case "windows":
		for i, want := range []string{"CredWrite", "CredRead", "CredDelete"} {
			script := strings.Join(calls[i], " ")
			if !strings.Contains(script, want) {
				t.Fatalf("windows call %d = %v, want it to contain %s", i, calls[i], want)
			}
		}
	case "linux":
		if calls[0][0] != "secret-tool" || calls[0][1] != "store" {
			t.Fatalf("linux Set call = %v, want secret-tool store", calls[0])
		}
		if calls[1][0] != "secret-tool" || calls[1][1] != "lookup" {
			t.Fatalf("linux Get call = %v, want secret-tool lookup", calls[1])
		}
		if calls[2][0] != "secret-tool" || calls[2][1] != "clear" {
			t.Fatalf("linux Delete call = %v, want secret-tool clear", calls[2])
		}
	}
}
