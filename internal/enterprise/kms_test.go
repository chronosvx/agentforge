package enterprise_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/chronosvx/agentforge/internal/enterprise"
)

func TestLocalKMSRoundTrip(t *testing.T) {
	kms, err := enterprise.NewLocalKMS(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("new kms: %v", err)
	}

	plain := []byte("enterprise secret payload")
	sealed, err := kms.Encrypt(context.Background(), plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(sealed, plain) {
		t.Fatalf("ciphertext must not equal plaintext")
	}

	got, err := kms.Decrypt(context.Background(), sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plain)
	}
}

func TestLocalKMSAllKeyLengths(t *testing.T) {
	for _, n := range []int{16, 24, 32} {
		kms, err := enterprise.NewLocalKMS(bytes.Repeat([]byte{byte(n)}, n))
		if err != nil {
			t.Fatalf("new kms with %d-byte key: %v", n, err)
		}
		sealed, err := kms.Encrypt(context.Background(), []byte("hello"))
		if err != nil {
			t.Fatalf("encrypt with %d-byte key: %v", n, err)
		}
		got, err := kms.Decrypt(context.Background(), sealed)
		if err != nil {
			t.Fatalf("decrypt with %d-byte key: %v", n, err)
		}
		if string(got) != "hello" {
			t.Fatalf("decrypt with %d-byte key = %q, want hello", n, got)
		}
	}
}

func TestLocalKMSRejectsInvalidKeyLength(t *testing.T) {
	if _, err := enterprise.NewLocalKMS([]byte("short")); err == nil {
		t.Fatalf("expected error for short key")
	}
	if _, err := enterprise.NewLocalKMS([]byte("this-is-a-too-long-key-for-aes!")); err == nil {
		t.Fatalf("expected error for invalid key length")
	}
}

func TestLocalKMSWrongKeyFails(t *testing.T) {
	kms, err := enterprise.NewLocalKMS(bytes.Repeat([]byte{0x01}, 32))
	if err != nil {
		t.Fatalf("new kms: %v", err)
	}
	sealed, err := kms.Encrypt(context.Background(), []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	wrong, err := enterprise.NewLocalKMS(bytes.Repeat([]byte{0x02}, 32))
	if err != nil {
		t.Fatalf("new wrong kms: %v", err)
	}
	if _, err := wrong.Decrypt(context.Background(), sealed); err == nil {
		t.Fatalf("expected error when decrypting with wrong key")
	}
}

func TestLocalKMSFromConfig(t *testing.T) {
	raw := bytes.Repeat([]byte{0xAB}, 32)
	hexKey := hex.EncodeToString(raw)
	b64Key := base64.StdEncoding.EncodeToString(raw)

	for name, encoded := range map[string]string{
		"hex":     hexKey,
		"base64":  b64Key,
		"invalid": "not-a-valid-key",
	} {
		t.Run(name, func(t *testing.T) {
			kms, err := enterprise.NewLocalKMSFromConfig(encoded)
			if name == "invalid" {
				if err == nil {
					t.Fatalf("expected error for invalid config key")
				}
				return
			}
			if err != nil {
				t.Fatalf("new kms from config: %v", err)
			}
			sealed, err := kms.Encrypt(context.Background(), []byte("cfg"))
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			got, err := kms.Decrypt(context.Background(), sealed)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if string(got) != "cfg" {
				t.Fatalf("got %q, want cfg", got)
			}
		})
	}
}

func TestLocalKMSContextCancel(t *testing.T) {
	kms, err := enterprise.NewLocalKMS(bytes.Repeat([]byte{0x10}, 32))
	if err != nil {
		t.Fatalf("new kms: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := kms.Encrypt(ctx, []byte("data")); err == nil {
		t.Fatalf("expected error for cancelled context")
	}
	if _, err := kms.Decrypt(ctx, []byte("data")); err == nil {
		t.Fatalf("expected error for cancelled context")
	}
}
