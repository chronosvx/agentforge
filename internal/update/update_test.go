package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
