package session_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/chronosvx/agentforge/internal/session"
)

// mapRemote 是一个内存版 RemoteStore，用于测试。
type mapRemote struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMapRemote() *mapRemote {
	return &mapRemote{data: make(map[string][]byte)}
}

func (m *mapRemote) Put(_ context.Context, id string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[id] = append([]byte(nil), data...)
	return nil
}

func (m *mapRemote) Get(_ context.Context, id string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.data[id]
	if !ok {
		return nil, io.ErrUnexpectedEOF
	}
	return append([]byte(nil), data...), nil
}

func (m *mapRemote) List(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.data))
	for id := range m.data {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func TestHTTPStorePutGetList(t *testing.T) {
	var mu sync.Mutex
	blobs := make(map[string][]byte)

	mux := http.NewServeMux()
	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/sessions/")
		switch r.Method {
		case http.MethodPut:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			blobs[id] = body
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			mu.Lock()
			body, ok := blobs[id]
			mu.Unlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("list") != "1" {
			http.Error(w, "missing list=1", http.StatusBadRequest)
			return
		}
		mu.Lock()
		ids := make([]string, 0, len(blobs))
		for id := range blobs {
			ids = append(ids, id)
		}
		mu.Unlock()
		sort.Strings(ids)
		_ = json.NewEncoder(w).Encode(ids)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	store := session.NewHTTPStore(srv.URL + "/sessions/")
	ctx := context.Background()

	if err := store.Put(ctx, "s1", []byte("hello world")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Put(ctx, "s2", []byte("second")); err != nil {
		t.Fatalf("put second: %v", err)
	}

	got, err := store.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != "hello world" {
		t.Fatalf("get body = %q, want %q", got, "hello world")
	}

	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 || ids[0] != "s1" || ids[1] != "s2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestHTTPStoreErrorRedactsSecrets(t *testing.T) {
	const secret = "sk-1234567890abcdefghijklmnopqrstuvwxyz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"apiKey":"`+secret+`"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := session.NewHTTPStore(srv.URL)
	_, err := store.Get(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error from server")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("HTTPStore error leaked secret: %v", err)
	}
	if !strings.Contains(err.Error(), "***") {
		t.Fatalf("HTTPStore error missing redaction marker: %v", err)
	}
}

func TestRemoteEncryptedStoreRoundTrip(t *testing.T) {
	remote := newMapRemote()
	keyHex := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	enc, err := session.NewRemoteEncryptedStore(remote, keyHex)
	if err != nil {
		t.Fatalf("new encrypted store: %v", err)
	}

	ctx := context.Background()
	plain := []byte(`{"id":"s1","title":"secret session"}`)
	if err := enc.Put(ctx, "s1", plain); err != nil {
		t.Fatalf("encrypted put: %v", err)
	}

	raw := remote.data["s1"]
	if string(raw) == string(plain) {
		t.Fatalf("stored data is not encrypted")
	}

	got, err := enc.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("encrypted get: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plain)
	}
}

func TestRemoteEncryptedStoreUsesEnvKey(t *testing.T) {
	remote := newMapRemote()
	keyHex := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	t.Setenv("DSH_SYNC_KEY", keyHex)

	enc, err := session.NewRemoteEncryptedStore(remote, "")
	if err != nil {
		t.Fatalf("new encrypted store from env: %v", err)
	}

	ctx := context.Background()
	plain := []byte("env-key-data")
	if err := enc.Put(ctx, "a", plain); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := enc.Get(ctx, "a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got, plain)
	}
}

func TestRemoteEncryptedStoreRejectsMissingKey(t *testing.T) {
	t.Setenv("DSH_SYNC_KEY", "")
	if _, err := session.NewRemoteEncryptedStore(newMapRemote(), ""); err == nil {
		t.Fatalf("expected error for missing key")
	}
}

func TestDecodeSyncKeyAcceptHexAndBase64(t *testing.T) {
	remote := newMapRemote()

	// 32-byte key in hex.
	hexKey := "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
	if _, err := session.NewRemoteEncryptedStore(remote, hexKey); err != nil {
		t.Fatalf("hex key rejected: %v", err)
	}

	// Same key in base64.
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	b64Key := "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
	if _, err := session.NewRemoteEncryptedStore(remote, b64Key); err != nil {
		t.Fatalf("base64 key rejected: %v", err)
	}
	_ = raw
}
