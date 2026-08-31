package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func makePluginZip(t *testing.T, name, version string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fm := fileManifest{
		Manifest: Manifest{
			Name:        name,
			Version:     version,
			Description: "Test plugin",
			Commands:    []string{name},
		},
		Executable: "hello",
	}
	manifestData, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	mf, err := zw.Create(name + ManifestSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mf.Write(manifestData); err != nil {
		t.Fatal(err)
	}

	hdr := &zip.FileHeader{Name: "hello", Method: zip.Deflate}
	hdr.SetMode(0o755)
	ef, err := zw.CreateHeader(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ef.Write([]byte("#!/bin/sh\necho hello\n")); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func writeSimplePluginDir(t *testing.T, dir, name string) {
	t.Helper()
	fm := fileManifest{
		Manifest: Manifest{
			Name:        name,
			Version:     "1.0.0",
			Description: "Test plugin",
			Commands:    []string{name},
		},
		Executable: "hello",
	}
	data, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+ManifestSuffix), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hello"), []byte("#!/bin/sh\necho hello\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestLoadMarketIndexParsesArrayAndObject(t *testing.T) {
	arrayData := []byte(`[
		{"name":"alpha","version":"1.0.0","description":"Alpha","url":"https://example.com/alpha.zip","sha256":"abc"},
		{"name":"beta","version":"2.0.0","description":"Beta","url":"https://example.com/beta.zip"}
	]`)
	got, err := LoadMarketIndex(arrayData)
	if err != nil {
		t.Fatalf("LoadMarketIndex array: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
	}
	if got[0].Name != "alpha" || got[0].Version != "1.0.0" || got[0].SHA256 != "abc" {
		t.Fatalf("unexpected first entry: %#v", got[0])
	}
	if got[1].Name != "beta" || got[1].SHA256 != "" {
		t.Fatalf("unexpected second entry: %#v", got[1])
	}

	objData := []byte(`{"plugins":[{"name":"gamma","version":"0.1.0","description":"Gamma","url":"https://example.com/gamma.zip","sha256":"def"}]}`)
	got, err = LoadMarketIndex(objData)
	if err != nil {
		t.Fatalf("LoadMarketIndex object: %v", err)
	}
	if len(got) != 1 || got[0].Name != "gamma" || got[0].SHA256 != "def" {
		t.Fatalf("unexpected object entries: %#v", got)
	}
}

func TestLoadMarketIndexRejectsInvalid(t *testing.T) {
	if _, err := LoadMarketIndex([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if _, err := LoadMarketIndex(nil); err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestFetchMarketIndexHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{
			"plugins": []MarketIndex{{Name: "remote", Version: "1.2.3", Description: "Remote", URL: "https://example.com/remote.zip"}},
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	got, err := FetchMarketIndex(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchMarketIndex: %v", err)
	}
	if len(got) != 1 || got[0].Name != "remote" || got[0].Version != "1.2.3" {
		t.Fatalf("unexpected fetched entries: %#v", got)
	}
}

func TestInstallFromMarketHTTPZipVerifiesSHA256(t *testing.T) {
	payload, sum := makePluginZip(t, "demo", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	index := []MarketIndex{{
		Name:        "demo",
		Version:     "1.0.0",
		Description: "Demo plugin",
		URL:         srv.URL + "/demo.zip",
		SHA256:      sum,
	}}
	dest := t.TempDir()
	p, err := InstallFromMarket(context.Background(), index, "demo", dest)
	if err != nil {
		t.Fatalf("InstallFromMarket: %v", err)
	}
	if p.Manifest.Name != "demo" || p.Manifest.Version != "1.0.0" {
		t.Fatalf("unexpected installed manifest: %#v", p.Manifest)
	}
	if p.Executable == "" {
		t.Fatal("expected installed executable path")
	}
	if _, err := os.Stat(filepath.Join(dest, "demo", "demo"+ManifestSuffix)); err != nil {
		t.Fatalf("installed manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "demo", "hello")); err != nil {
		t.Fatalf("installed executable missing: %v", err)
	}
}

func TestInstallFromMarketHTTPRejectsSHA256Mismatch(t *testing.T) {
	payload, _ := makePluginZip(t, "demo", "1.0.0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	index := []MarketIndex{{
		Name:   "demo",
		URL:    srv.URL + "/demo.zip",
		SHA256: strings.Repeat("0", 64),
	}}
	dest := t.TempDir()
	_, err := InstallFromMarket(context.Background(), index, "demo", dest)
	if err == nil {
		t.Fatal("expected SHA256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("expected SHA256 mismatch error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dest, "demo")); !os.IsNotExist(statErr) {
		t.Fatalf("plugin directory should not remain after hash mismatch: %v", statErr)
	}
}

func TestInstallFromMarketHTTPRawFile(t *testing.T) {
	payload := []byte("#!/bin/sh\necho raw plugin\n")
	sum := sha256.Sum256(payload)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	index := []MarketIndex{{
		Name:   "rawdemo",
		URL:    srv.URL + "/rawdemo",
		SHA256: hex.EncodeToString(sum[:]),
	}}
	dest := t.TempDir()
	p, err := InstallFromMarket(context.Background(), index, "rawdemo", dest)
	if err != nil {
		t.Fatalf("InstallFromMarket raw: %v", err)
	}
	if p.Manifest.Name != "rawdemo" || len(p.Manifest.Commands) != 1 || p.Manifest.Commands[0] != "rawdemo" {
		t.Fatalf("unexpected raw plugin manifest: %#v", p.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dest, "rawdemo", "rawdemo.dsh-plugin.json")); err != nil {
		t.Fatalf("generated manifest missing: %v", err)
	}
}

func TestInstallFromMarketGitClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSimplePluginDir(t, repo, "gitdemo")

	runGit(t, repo, "init")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "add plugin")

	sum, err := hashDir(repo)
	if err != nil {
		t.Fatalf("hashDir repo: %v", err)
	}
	index := []MarketIndex{{
		Name:   "gitdemo",
		URL:    repo,
		SHA256: sum,
	}}
	dest := t.TempDir()
	p, err := InstallFromMarket(context.Background(), index, "gitdemo", dest)
	if err != nil {
		t.Fatalf("InstallFromMarket git: %v", err)
	}
	if p.Manifest.Name != "gitdemo" {
		t.Fatalf("unexpected git plugin manifest: %#v", p.Manifest)
	}
	if _, err := os.Stat(filepath.Join(dest, "gitdemo", "hello")); err != nil {
		t.Fatalf("cloned executable missing: %v", err)
	}
}

func TestInstallFromMarketNotFound(t *testing.T) {
	dest := t.TempDir()
	_, err := InstallFromMarket(context.Background(), nil, "missing", dest)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
