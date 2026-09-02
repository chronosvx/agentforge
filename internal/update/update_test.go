package update_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/update"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
		wantErr bool
	}{
		{name: "patch update", current: "1.2.3", latest: "1.2.4", want: true},
		{name: "minor update", current: "1.2.3", latest: "1.3.0", want: true},
		{name: "major update", current: "1.2.3", latest: "2.0.0", want: true},
		{name: "same version", current: "v1.2.3", latest: "1.2.3", want: false},
		{name: "current newer", current: "2.0.0", latest: "1.9.9", want: false},
		{name: "prerelease lower", current: "1.2.3", latest: "1.2.3-rc.1", want: false},
		{name: "prerelease higher", current: "1.2.3-rc.1", latest: "1.2.3", want: true},
		{name: "numeric prerelease compare", current: "1.0.0-alpha.2", latest: "1.0.0-alpha.10", want: true},
		{name: "build metadata ignored", current: "1.2.3+foo", latest: "1.2.3+bar", want: false},
		{name: "invalid current", current: "abc", latest: "1.2.3", wantErr: true},
		{name: "invalid latest", current: "1.2.3", latest: "v1.2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := update.Check(tt.current, tt.latest)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Check(%q, %q) expected error, got nil", tt.current, tt.latest)
				}
				return
			}
			if err != nil {
				t.Fatalf("Check(%q, %q): %v", tt.current, tt.latest, err)
			}
			if got != tt.want {
				t.Fatalf("Check(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestLatestFromGitHubWithMock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/chronosvx/agentforge/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("unexpected Accept header: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	got, err := update.LatestFromGitHubWithClient(context.Background(), "chronosvx", "agentforge", srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("LatestFromGitHubWithClient: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", got)
	}
}

func TestLatestFromGitHubHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := update.LatestFromGitHubWithClient(context.Background(), "chronosvx", "missing", srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLatestFromGitHubRequiresOwnerRepo(t *testing.T) {
	if _, err := update.LatestFromGitHub("", "repo"); err == nil {
		t.Fatal("expected error for empty owner")
	}
	if _, err := update.LatestFromGitHub("owner", ""); err == nil {
		t.Fatal("expected error for empty repo")
	}
}

func TestAssetName(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
	}{
		{goos: "darwin", goarch: "amd64", want: "agentforge-darwin-amd64"},
		{goos: "darwin", goarch: "arm64", want: "agentforge-darwin-arm64"},
		{goos: "linux", goarch: "amd64", want: "agentforge-linux-amd64"},
		{goos: "linux", goarch: "arm64", want: "agentforge-linux-arm64"},
		{goos: "windows", goarch: "amd64", want: "agentforge-windows-amd64.exe"},
		{goos: "windows", goarch: "arm64", want: "agentforge-windows-arm64.exe"},
	}
	for _, tt := range tests {
		if got := update.AssetName(tt.goos, tt.goarch); got != tt.want {
			t.Errorf("AssetName(%q, %q) = %q, want %q", tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestDownloadReleaseAssetWithMock(t *testing.T) {
	const (
		asset = "agentforge-darwin-arm64"
		body  = "new agentforge binary"
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/chronosvx/agentforge-install/releases/download/v0.6.1/" + asset
		if r.URL.Path != wantPath {
			t.Errorf("unexpected path: %s, want %s", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "download.bin")
	err := update.DownloadReleaseAssetWithClient(context.Background(), "chronosvx", "agentforge-install", "v0.6.1", asset, dest, srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("DownloadReleaseAssetWithClient: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("downloaded content = %q, want %q", got, body)
	}
}

func TestDownloadReleaseAssetHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer srv.Close()

	err := update.DownloadReleaseAssetWithClient(context.Background(), "chronosvx", "agentforge-install", "v9.9.9", "nope", filepath.Join(t.TempDir(), "nope"), srv.URL, srv.Client())
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChecksumSuccess(t *testing.T) {
	dir := t.TempDir()
	assetName := "agentforge-linux-amd64"
	assetPath := filepath.Join(dir, assetName)
	content := []byte("linux amd64 binary")
	if err := os.WriteFile(assetPath, content, 0o755); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	sum := sha256.Sum256(content)
	checksumsPath := filepath.Join(dir, "checksums.txt")
	line := fmt.Sprintf("%x  %s\n", sum, assetName)
	if err := os.WriteFile(checksumsPath, []byte(line), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	if err := update.VerifyChecksum(assetPath, checksumsPath, assetName); err != nil {
		t.Fatalf("VerifyChecksum: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	assetName := "agentforge-linux-amd64"
	assetPath := filepath.Join(dir, assetName)
	content := []byte("actual binary")
	if err := os.WriteFile(assetPath, content, 0o755); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	checksumsPath := filepath.Join(dir, "checksums.txt")
	line := strings.Repeat("0", 64) + "  " + assetName + "\n"
	if err := os.WriteFile(checksumsPath, []byte(line), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	err := update.VerifyChecksum(assetPath, checksumsPath, assetName)
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	dir := t.TempDir()
	assetPath := filepath.Join(dir, "agentforge-linux-amd64")
	if err := os.WriteFile(assetPath, []byte("x"), 0o755); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	checksumsPath := filepath.Join(dir, "checksums.txt")
	if err := os.WriteFile(checksumsPath, []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  agentforge-darwin-amd64\n"), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	err := update.VerifyChecksum(assetPath, checksumsPath, "agentforge-linux-amd64")
	if err == nil {
		t.Fatal("expected missing-entry error")
	}
	if !strings.Contains(err.Error(), "does not contain an entry") {
		t.Fatalf("unexpected error: %v", err)
	}
}
