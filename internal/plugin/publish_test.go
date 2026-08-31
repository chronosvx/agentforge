package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPreparePublishSignsAndInstallFromMarketVerifies(t *testing.T) {
	privPEM, pubPEM := newSignatureTestKey(t)
	payload, sum := makePluginZip(t, "signedpub", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	entry := MarketIndexEntry{
		Name:        "  signedpub  ",
		Version:     " 1.0.0 ",
		Description: "  Signed published plugin  ",
		URL:         srv.URL + "/signedpub.zip",
	}
	published, err := PreparePublish(entry, privPEM)
	if err != nil {
		t.Fatalf("PreparePublish: %v", err)
	}

	if published.SHA256 != sum {
		t.Fatalf("PreparePublish SHA256 = %q, want %q", published.SHA256, sum)
	}
	if published.Signature.Signature == "" {
		t.Fatal("PreparePublish returned empty signature")
	}
	if published.Signature.PublicKeyID == "" {
		t.Fatal("PreparePublish returned empty PublicKeyID")
	}

	index := []MarketIndex{{
		Name:        published.Name,
		Version:     published.Version,
		Description: published.Description,
		URL:         published.URL,
		SHA256:      published.SHA256,
		Signature:   published.Signature,
	}}

	ts := NewTrustStore()
	if err := ts.AddPublicKey(published.Signature.PublicKeyID, pubPEM); err != nil {
		t.Fatalf("AddPublicKey: %v", err)
	}

	dest := t.TempDir()
	p, err := InstallFromMarket(context.Background(), index, "signedpub", dest, ts)
	if err != nil {
		t.Fatalf("InstallFromMarket with published signature: %v", err)
	}
	if p.Manifest.Name != "signedpub" || p.Manifest.Version != "1.0.0" {
		t.Fatalf("unexpected installed plugin: %#v", p.Manifest)
	}
}

func TestAppendToIndexDeduplicatesArrayFormat(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(indexPath, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 第一次写入：追加到顶层数组。
	first := PublishEntry{
		Name:        "demo",
		Version:     "1.0.0",
		Description: "Demo",
		URL:         "https://example.com/demo.zip",
		SHA256:      "oldsha",
		Signature:   Signature{PublicKeyID: "key-1", Signature: "oldsig"},
	}
	if err := AppendToIndex(indexPath, first); err != nil {
		t.Fatalf("AppendToIndex create: %v", err)
	}
	entries, err := LoadMarketIndex(mustReadFile(t, indexPath))
	if err != nil {
		t.Fatalf("LoadMarketIndex after create: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "demo" || entries[0].Version != "1.0.0" {
		t.Fatalf("unexpected created index: %#v", entries)
	}

	// 第二次写入同一条目：应更新而不是新增。
	updated := PublishEntry{
		Name:        "demo",
		Version:     "2.0.0",
		Description: "Demo v2",
		URL:         "https://example.com/demo-v2.zip",
		SHA256:      "newsha",
		Signature:   Signature{PublicKeyID: "key-2", Signature: "newsig"},
	}
	if err := AppendToIndex(indexPath, updated); err != nil {
		t.Fatalf("AppendToIndex update: %v", err)
	}
	entries, err = LoadMarketIndex(mustReadFile(t, indexPath))
	if err != nil {
		t.Fatalf("LoadMarketIndex after update: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected deduplicated single entry, got %d: %#v", len(entries), entries)
	}
	got := entries[0]
	if got.Version != "2.0.0" || got.SHA256 != "newsha" || got.URL != "https://example.com/demo-v2.zip" {
		t.Fatalf("unexpected updated entry: %#v", got)
	}
	if got.Signature.PublicKeyID != "key-2" || got.Signature.Signature != "newsig" {
		t.Fatalf("unexpected updated signature: %#v", got.Signature)
	}
}

func TestAppendToIndexCreatesObjectFormat(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "market.json")
	entry := PublishEntry{
		Name:        "brand-new",
		Version:     "1.0.0",
		Description: "New plugin",
		URL:         "https://example.com/new.zip",
		SHA256:      "sha",
		Signature:   Signature{PublicKeyID: "key", Signature: "sig"},
	}
	if err := AppendToIndex(indexPath, entry); err != nil {
		t.Fatalf("AppendToIndex create: %v", err)
	}
	entries, err := LoadMarketIndex(mustReadFile(t, indexPath))
	if err != nil {
		t.Fatalf("LoadMarketIndex after create: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "brand-new" {
		t.Fatalf("unexpected created entries: %#v", entries)
	}
}

func TestAppendToIndexUpdatesObjectFormat(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "market.json")
	initial := `{"plugins":[
		{"name":"alpha","version":"0.1.0","description":"Alpha","url":"https://example.com/alpha.zip","sha256":"old"}
	]}`
	if err := os.WriteFile(indexPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := PublishEntry{
		Name:        "alpha",
		Version:     "1.0.0",
		Description: "Alpha updated",
		URL:         "https://example.com/alpha-v1.zip",
		SHA256:      "newsha",
		Signature:   Signature{PublicKeyID: "key-1", Signature: "sig"},
	}
	if err := AppendToIndex(indexPath, entry); err != nil {
		t.Fatalf("AppendToIndex object: %v", err)
	}

	entries, err := LoadMarketIndex(mustReadFile(t, indexPath))
	if err != nil {
		t.Fatalf("LoadMarketIndex: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one entry after object update, got %d: %#v", len(entries), entries)
	}
	if entries[0].Version != "1.0.0" || entries[0].SHA256 != "newsha" {
		t.Fatalf("unexpected object updated entry: %#v", entries[0])
	}
}

func TestAppendToIndexRejectsUnsupportedFormat(t *testing.T) {
	indexPath := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(indexPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := AppendToIndex(indexPath, PublishEntry{Name: "demo", Version: "1.0.0"})
	if err == nil {
		t.Fatal("expected AppendToIndex to reject unsupported index content")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// 确保 PublishEntry 与 MarketIndex 的 JSON 字段兼容。
func TestPublishEntryJSONCompatibleWithMarketIndex(t *testing.T) {
	entry := PublishEntry{
		Name:        "compat",
		Version:     "1.0.0",
		Description: "Compat",
		URL:         "https://example.com/compat.zip",
		SHA256:      "sha",
		Signature:   Signature{PublicKeyID: "key", Signature: "sig"},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	var mi MarketIndex
	if err := json.Unmarshal(data, &mi); err != nil {
		t.Fatalf("PublishEntry JSON not compatible with MarketIndex: %v", err)
	}
	if mi.Name != "compat" || mi.SHA256 != "sha" || mi.Signature.PublicKeyID != "key" {
		t.Fatalf("unexpected decoded MarketIndex: %#v", mi)
	}
}
