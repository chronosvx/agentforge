package security_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/chronosvx/agentforge/internal/security"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	plain := []byte(`{"sessionId":"s1","title":"本地会话"}`)

	sealed, err := security.Encrypt(plain, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(sealed, plain) {
		t.Fatalf("ciphertext must not equal plaintext")
	}

	got, err := security.Decrypt(sealed, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plain)
	}
}

func TestEncryptDecryptSupportsAllKeyLengths(t *testing.T) {
	for _, n := range []int{16, 24, 32} {
		key := bytes.Repeat([]byte{byte(n)}, n)
		sealed, err := security.Encrypt([]byte("hello"), key)
		if err != nil {
			t.Fatalf("encrypt with %d-byte key: %v", n, err)
		}
		got, err := security.Decrypt(sealed, key)
		if err != nil {
			t.Fatalf("decrypt with %d-byte key: %v", n, err)
		}
		if string(got) != "hello" {
			t.Fatalf("decrypt with %d-byte key = %q, want hello", n, got)
		}
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	sealed, err := security.Encrypt([]byte("secret"), key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	wrong := bytes.Repeat([]byte{0x02}, 32)
	if _, err := security.Decrypt(sealed, wrong); err == nil {
		t.Fatalf("expected error when decrypting with wrong key")
	}
}

func TestEncryptRejectsInvalidKeyLength(t *testing.T) {
	if _, err := security.Encrypt([]byte("data"), []byte("short")); err == nil {
		t.Fatalf("expected error for short key")
	}
	if _, err := security.Decrypt([]byte("data"), []byte("short")); err == nil {
		t.Fatalf("expected error for short decrypt key")
	}
}

func TestLoadKeyHexAndBase64(t *testing.T) {
	raw := bytes.Repeat([]byte{0xAB}, 32)
	hexKey := hex.EncodeToString(raw)
	b64Key := base64.StdEncoding.EncodeToString(raw)
	rawB64Key := base64.RawStdEncoding.EncodeToString(raw)
	urlB64Key := base64.URLEncoding.EncodeToString(raw)
	rawURLB64Key := base64.RawURLEncoding.EncodeToString(raw)

	for name, input := range map[string]string{
		"hex":         hexKey,
		"base64":      b64Key,
		"raw base64":  rawB64Key,
		"url base64":  urlB64Key,
		"raw url b64": rawURLB64Key,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := security.LoadKey(input)
			if err != nil {
				t.Fatalf("load key: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("key mismatch: got %x, want %x", got, raw)
			}
		})
	}
}

func TestLoadKeyRejectsInvalid(t *testing.T) {
	invalid := []string{
		"",
		"not-a-key",
		hex.EncodeToString([]byte("too short")),
		base64.StdEncoding.EncodeToString([]byte("too short")),
	}
	for _, input := range invalid {
		if _, err := security.LoadKey(input); err == nil {
			t.Fatalf("expected error for %q", input)
		}
	}
}
