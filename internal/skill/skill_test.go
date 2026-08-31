package skill

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLocalScansMarkdownFiles(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "b.md"), []byte("# B\n\nB description\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "a.md"), []byte("---\nname: A Skill\ndescription: A desc\nenabled: false\n---\n# A\n\nA body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "ignore.txt"), []byte("not skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillsDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadLocal(dir)
	if err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0].Name != "A Skill" || entries[0].Description != "A desc" || entries[0].Enabled {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Name != "b" || entries[1].Description != "B description" || !entries[1].Enabled {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}
	if entries[1].Source != "local" {
		t.Fatalf("expected local source, got %q", entries[1].Source)
	}
}

func TestLoadLocalMissingDir(t *testing.T) {
	entries, err := LoadLocal(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLocal missing dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty, got %#v", entries)
	}
}

func TestParseLocalPlainMarkdown(t *testing.T) {
	e := ParseLocal("agent-browser", "# agent-browser\n\n浏览器自动化工具。\n")
	if e.Name != "agent-browser" {
		t.Fatalf("unexpected name: %q", e.Name)
	}
	if e.Description != "浏览器自动化工具。" {
		t.Fatalf("unexpected description: %q", e.Description)
	}
	if !e.Enabled {
		t.Fatal("expected enabled by default")
	}
}

func TestParseLocalFrontmatter(t *testing.T) {
	e := ParseLocal("file.md", `---
name: custom-name
description: custom description
enabled: false
---
# Body
`)
	if e.Name != "custom-name" || e.Description != "custom description" || e.Enabled {
		t.Fatalf("unexpected parsed entry: %#v", e)
	}
}

func TestParseAvailableSkillsMarkdownList(t *testing.T) {
	text := `hello

<available_skills>
- agent-browser: 浏览器自动化
- bash: 执行 shell 命令
- file-read
</available_skills>

done`
	entries := ParseAvailableSkills(text)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0].Name != "agent-browser" || entries[0].Description != "浏览器自动化" {
		t.Fatalf("unexpected first: %#v", entries[0])
	}
	if entries[1].Name != "bash" || entries[1].Description != "执行 shell 命令" {
		t.Fatalf("unexpected second: %#v", entries[1])
	}
	if entries[2].Name != "file-read" || entries[2].Description != "" {
		t.Fatalf("unexpected third: %#v", entries[2])
	}
	if entries[0].Source != "remote" || !entries[0].Enabled {
		t.Fatalf("unexpected source/enabled: %#v", entries[0])
	}
}

func TestParseAvailableSkillsJSON(t *testing.T) {
	text := `<available_skills>
[{"name":"web","description":"web tools"},{"name":"code","summary":"code tools"}]
</available_skills>`
	entries := ParseAvailableSkills(text)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0].Name != "web" || entries[0].Description != "web tools" {
		t.Fatalf("unexpected first: %#v", entries[0])
	}
	if entries[1].Name != "code" || entries[1].Description != "code tools" {
		t.Fatalf("unexpected second: %#v", entries[1])
	}
}

func TestParseRemoteValue(t *testing.T) {
	got := ParseRemoteValue([]any{
		map[string]any{"name": "alpha", "description": "Alpha"},
		"beta: Beta",
	})
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
	}
	if got[0].Name != "alpha" || got[0].Description != "Alpha" {
		t.Fatalf("unexpected first: %#v", got[0])
	}
	if got[1].Name != "beta" || got[1].Description != "Beta" {
		t.Fatalf("unexpected second: %#v", got[1])
	}
}

func TestMergeLocalPriorityAndDedupe(t *testing.T) {
	local := []Entry{{Name: "common", Description: "local desc", Source: "local", Enabled: true}}
	remote := []Entry{
		{Name: "common", Description: "remote desc", Source: "remote", Enabled: true},
		{Name: "remote-only", Description: "remote only", Source: "remote", Enabled: true},
	}
	merged := Merge(local, remote)
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged entries, got %d: %#v", len(merged), merged)
	}
	if merged[0].Name != "common" || merged[0].Description != "local desc" || merged[0].Source != "local" {
		t.Fatalf("expected local priority, got %#v", merged[0])
	}
	if merged[1].Name != "remote-only" || merged[1].Source != "remote" {
		t.Fatalf("unexpected second: %#v", merged[1])
	}
}

func TestToggle(t *testing.T) {
	entries := []Entry{{Name: "a", Enabled: true}, {Name: "b", Enabled: false}}
	if !Toggle(entries, "a") || entries[0].Enabled {
		t.Fatalf("expected a toggled off: %#v", entries[0])
	}
	if !Toggle(entries, "b") || !entries[1].Enabled {
		t.Fatalf("expected b toggled on: %#v", entries[1])
	}
	if Toggle(entries, "missing") {
		t.Fatal("expected false for missing name")
	}
}

func TestInstallFromDirCopiesMarkdown(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	writeTestSkill(t, src, "alpha.md", "---\nname: Alpha\ndescription: Alpha skill\n---\n# Alpha\n")
	writeTestSkill(t, src, "beta.md", "# Beta\n\nBeta skill\n")
	if err := os.WriteFile(filepath.Join(src, "ignore.txt"), []byte("not skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := InstallFromDir(src, dest)
	if err != nil {
		t.Fatalf("InstallFromDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0].Name != "Alpha" || entries[0].Source != "local" {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].Name != "beta" || entries[1].Description != "Beta skill" {
		t.Fatalf("unexpected second entry: %#v", entries[1])
	}

	for _, name := range []string{"alpha.md", "beta.md"} {
		if _, err := os.Stat(filepath.Join(dest, "skills", name)); err != nil {
			t.Fatalf("installed file %s missing: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "ignore.txt")); !os.IsNotExist(err) {
		t.Fatalf("non-markdown file should not be copied: %v", err)
	}
}

func TestInstallFromDirErrors(t *testing.T) {
	if _, err := InstallFromDir(filepath.Join(t.TempDir(), "missing"), t.TempDir()); err == nil {
		t.Fatal("expected error for missing source directory")
	}

	file := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallFromDir(file, t.TempDir()); err == nil {
		t.Fatal("expected error when source is a file")
	}

	if _, err := InstallFromDir(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for empty destination")
	}
}

func TestInstallFromRepoClonesLocalRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestSkill(t, repo, "repo-skill.md", "# Repo Skill\n\nFrom git repo\n")

	runGit(t, repo, "init")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "add skill")

	dest := t.TempDir()
	entries, err := InstallFromRepo(repo, dest)
	if err != nil {
		t.Fatalf("InstallFromRepo: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "repo-skill" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "repo-skill.md")); err != nil {
		t.Fatalf("installed repo skill missing: %v", err)
	}
}

func TestInstallFromRepoReturnsErrorForInvalidRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	notRepo := filepath.Join(t.TempDir(), "not-a-repo")
	if err := os.MkdirAll(notRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := InstallFromRepo(notRepo, t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid repository")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncFromManifestWithHTTPAndLocalDirs(t *testing.T) {
	dest := t.TempDir()
	srcA := t.TempDir()
	srcB := t.TempDir()
	writeTestSkill(t, srcA, "a.md", "# A\n\nA skill\n")
	writeTestSkill(t, srcB, "b.md", "# B\n\nB skill\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data, _ := json.Marshal([]map[string]string{
			{"name": "a", "url": srcA, "description": "A skill"},
			{"name": "b", "url": srcB, "description": "B skill"},
		})
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	entries, err := SyncFromManifest(srv.URL, dest)
	if err != nil {
		t.Fatalf("SyncFromManifest: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(entries), entries)
	}
	if entries[0].Name != "a" || entries[1].Name != "b" {
		t.Fatalf("unexpected entries: %#v", entries)
	}
	for _, name := range []string{"a.md", "b.md"} {
		if _, err := os.Stat(filepath.Join(dest, "skills", name)); err != nil {
			t.Fatalf("synced file %s missing: %v", name, err)
		}
	}
}

func TestSyncFromManifestObjectFormAndErrors(t *testing.T) {
	dest := t.TempDir()
	src := t.TempDir()
	writeTestSkill(t, src, "obj.md", "# Obj\n\nObj skill\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		data, _ := json.Marshal(map[string]any{
			"skills": []map[string]string{
				{"name": "obj", "url": src, "description": "Obj skill"},
			},
		})
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	entries, err := SyncFromManifest(srv.URL, dest)
	if err != nil {
		t.Fatalf("SyncFromManifest object form: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "obj" {
		t.Fatalf("unexpected entries: %#v", entries)
	}

	if _, err := SyncFromManifest(srv.URL+"/missing", dest); err == nil {
		t.Fatal("expected error for HTTP non-200")
	}
}

func writeTestSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, strings.TrimSpace(string(out)))
	}
}
