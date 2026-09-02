package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chronosvx/agentforge/internal/update"
)

func TestMaybeCheckUpdateUsesLatestFromPublicStub(t *testing.T) {
	oldLatest := latestFromPublic
	t.Cleanup(func() { latestFromPublic = oldLatest })

	latestFromPublic = func(context.Context) (string, error) { return "v9.9.9", nil }
	if got := maybeCheckUpdate(); got != "v9.9.9" {
		t.Fatalf("maybeCheckUpdate() = %q, want v9.9.9", got)
	}
}

func TestNormalizeReleaseTag(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "0.6.2", want: "v0.6.2"},
		{in: "v0.6.2", want: "v0.6.2"},
		{in: "V0.6.2", want: "v0.6.2"},
		{in: "v0.6.2-rc.1", want: "v0.6.2-rc.1"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		if got := normalizeReleaseTag(tt.in); got != tt.want {
			t.Errorf("normalizeReleaseTag(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRunUpdateSuccessFromHTTPServer(t *testing.T) {
	oldExe := currentExecutablePath
	oldDownload := downloadReleaseAsset
	t.Cleanup(func() {
		currentExecutablePath = oldExe
		downloadReleaseAsset = oldDownload
	})

	dir := t.TempDir()
	exePath := filepath.Join(dir, "agentforge")
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}
	oldContent := []byte("old agentforge binary")
	if err := os.WriteFile(exePath, oldContent, 0o755); err != nil {
		t.Fatalf("write old executable: %v", err)
	}

	asset := update.AssetName(runtime.GOOS, runtime.GOARCH)
	newContent := []byte("new agentforge binary from release")
	sum := sha256.Sum256(newContent)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chronosvx/agentforge-install/releases/download/v1.2.3/" + asset:
			_, _ = w.Write(newContent)
		case "/chronosvx/agentforge-install/releases/download/v1.2.3/checksums.txt":
			fmt.Fprintf(w, "%x  %s\n", sum, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var gotVersion string
	currentExecutablePath = func() (string, error) { return exePath, nil }
	downloadReleaseAsset = func(ctx context.Context, owner, repo, version, assetName, destPath string) error {
		gotVersion = version
		return update.DownloadReleaseAssetWithClient(ctx, owner, repo, version, assetName, destPath, srv.URL, srv.Client())
	}

	if err := runUpdate([]string{"--version", "1.2.3"}); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	if gotVersion != "v1.2.3" {
		t.Fatalf("requested version = %q, want v1.2.3", gotVersion)
	}

	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read replaced executable: %v", err)
	}
	if string(got) != string(newContent) {
		t.Fatalf("executable content = %q, want %q", got, newContent)
	}

	backup, err := os.ReadFile(exePath + ".bak")
	if err != nil {
		t.Fatalf("read backup executable: %v", err)
	}
	if string(backup) != string(oldContent) {
		t.Fatalf("backup content = %q, want %q", backup, oldContent)
	}
}

func TestRunUpdateUsesLatestWhenNoVersion(t *testing.T) {
	oldExe := currentExecutablePath
	oldDownload := downloadReleaseAsset
	oldLatest := latestFromPublic
	t.Cleanup(func() {
		currentExecutablePath = oldExe
		downloadReleaseAsset = oldDownload
		latestFromPublic = oldLatest
	})

	dir := t.TempDir()
	exePath := filepath.Join(dir, "agentforge")
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatalf("write old executable: %v", err)
	}

	asset := update.AssetName(runtime.GOOS, runtime.GOARCH)
	content := []byte("new from latest")
	sum := sha256.Sum256(content)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chronosvx/agentforge-install/releases/download/v2.0.0/" + asset:
			_, _ = w.Write(content)
		case "/chronosvx/agentforge-install/releases/download/v2.0.0/checksums.txt":
			fmt.Fprintf(w, "%x  %s\n", sum, asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	latestFromPublic = func(context.Context) (string, error) { return "v2.0.0", nil }
	currentExecutablePath = func() (string, error) { return exePath, nil }
	downloadReleaseAsset = func(ctx context.Context, owner, repo, version, assetName, destPath string) error {
		return update.DownloadReleaseAssetWithClient(ctx, owner, repo, version, assetName, destPath, srv.URL, srv.Client())
	}

	if err := runUpdate(nil); err != nil {
		t.Fatalf("runUpdate: %v", err)
	}
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("executable content = %q, want %q", got, content)
	}
}

func TestRunUpdateChecksumFailurePreservesExecutable(t *testing.T) {
	oldExe := currentExecutablePath
	oldDownload := downloadReleaseAsset
	t.Cleanup(func() {
		currentExecutablePath = oldExe
		downloadReleaseAsset = oldDownload
	})

	dir := t.TempDir()
	exePath := filepath.Join(dir, "agentforge")
	if runtime.GOOS == "windows" {
		exePath += ".exe"
	}
	oldContent := []byte("old agentforge binary")
	if err := os.WriteFile(exePath, oldContent, 0o755); err != nil {
		t.Fatalf("write old executable: %v", err)
	}

	asset := update.AssetName(runtime.GOOS, runtime.GOARCH)
	newContent := []byte("tampered or wrong binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chronosvx/agentforge-install/releases/download/v9.9.9/" + asset:
			_, _ = w.Write(newContent)
		case "/chronosvx/agentforge-install/releases/download/v9.9.9/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", strings.Repeat("0", 64), asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	currentExecutablePath = func() (string, error) { return exePath, nil }
	downloadReleaseAsset = func(ctx context.Context, owner, repo, version, assetName, destPath string) error {
		return update.DownloadReleaseAssetWithClient(ctx, owner, repo, version, assetName, destPath, srv.URL, srv.Client())
	}

	err := runUpdate([]string{"--version", "v9.9.9"})
	if err == nil {
		t.Fatal("runUpdate succeeded, want checksum failure")
	}
	if !strings.Contains(err.Error(), "checksum") && !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := os.ReadFile(exePath)
	if readErr != nil {
		t.Fatalf("read executable after failure: %v", readErr)
	}
	if string(got) != string(oldContent) {
		t.Fatalf("executable was modified on failure: %q, want %q", got, oldContent)
	}
	if _, statErr := os.Stat(exePath + ".bak"); !os.IsNotExist(statErr) {
		t.Fatalf("backup should not be created on checksum failure, stat err = %v", statErr)
	}
}

func TestRunUpdateRejectsUnexpectedArgs(t *testing.T) {
	err := runUpdate([]string{"--version", "v1.2.3", "unexpected"})
	if err == nil {
		t.Fatal("expected error for unexpected positional argument")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("unexpected error: %v", err)
	}
}
