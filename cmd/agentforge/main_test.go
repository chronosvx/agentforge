package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/gitflow"
	"github.com/chronosvx/agentforge/internal/repomode"
	"github.com/chronosvx/agentforge/internal/session"
)

func TestExtractHeadlessOptions(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless", "--provider", "deepseek", "--prompt", "hello", "--mock",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless {
		t.Fatal("expected headless=true")
	}
	if prompt != "hello" {
		t.Fatalf("prompt = %q, want %q", prompt, "hello")
	}
	if format != "text" {
		t.Fatalf("format = %q, want default text", format)
	}
	want := []string{"--provider", "deepseek", "--mock"}
	if strings.Join(rest, " ") != strings.Join(want, " ") {
		t.Fatalf("rest = %v, want %v", rest, want)
	}
}

func TestExtractHeadlessOptionsEqualsForm(t *testing.T) {
	headless, prompt, format, rest, err := extractHeadlessOptions([]string{
		"--headless=true", "--prompt=hello", "--format=json", "--provider", "deepseek",
	})
	if err != nil {
		t.Fatalf("extract headless options: %v", err)
	}
	if !headless || prompt != "hello" {
		t.Fatalf("unexpected headless=%v prompt=%q", headless, prompt)
	}
	if format != "json" {
		t.Fatalf("format = %q, want json", format)
	}
	if len(rest) != 2 || rest[0] != "--provider" || rest[1] != "deepseek" {
		t.Fatalf("unexpected rest: %v", rest)
	}
}

func TestExtractHeadlessOptionsMissingPromptValue(t *testing.T) {
	if _, _, _, _, err := extractHeadlessOptions([]string{"--headless", "--prompt"}); err == nil {
		t.Fatal("expected error for missing --prompt value")
	}
}

func TestParseExecArgsLocal(t *testing.T) {
	opts, err := parseExecArgs([]string{
		"--local", "--cmd", "echo hi", "--timeout", "5s", "--env", "A=B",
	})
	if err != nil {
		t.Fatalf("parse exec args: %v", err)
	}
	if !opts.local || opts.docker || opts.ssh {
		t.Fatalf("unexpected mode: %+v", opts)
	}
	if opts.cmd != "echo hi" {
		t.Fatalf("cmd = %q, want %q", opts.cmd, "echo hi")
	}
	if opts.timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s", opts.timeout)
	}
	if len(opts.env) != 1 || opts.env[0] != "A=B" {
		t.Fatalf("env = %v, want [A=B]", opts.env)
	}
}

func TestParseExecArgsRequiresSingleMode(t *testing.T) {
	if _, err := parseExecArgs([]string{"--cmd", "echo hi"}); err == nil {
		t.Fatal("expected error when no mode is set")
	}
	if _, err := parseExecArgs([]string{"--local", "--docker", "--cmd", "echo hi"}); err == nil {
		t.Fatal("expected error when multiple modes are set")
	}
}

func TestParseExecArgsRequiresModeSpecificFlags(t *testing.T) {
	if _, err := parseExecArgs([]string{"--docker", "--cmd", "echo hi"}); err == nil {
		t.Fatal("expected error when docker image is missing")
	}
	if _, err := parseExecArgs([]string{"--ssh", "--cmd", "echo hi"}); err == nil {
		t.Fatal("expected error when ssh host is missing")
	}
	if _, err := parseExecArgs([]string{"--local"}); err == nil {
		t.Fatal("expected error when --cmd is missing")
	}
}

func TestSplitCommandLine(t *testing.T) {
	parts, err := splitCommandLine(`echo "hello world" 'single quoted'`)
	if err != nil {
		t.Fatalf("split command line: %v", err)
	}
	want := []string{"echo", "hello world", "single quoted"}
	if strings.Join(parts, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("parts = %q, want %q", parts, want)
	}
	if _, err := splitCommandLine(`echo "unterminated`); err == nil {
		t.Fatal("expected unterminated quote error")
	}
}

func TestRunExecLocalSuccess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := fmt.Sprintf("%q -test.run=TestHelperProcess -- echo", exe)
	if err := runExec([]string{
		"--local", "--cmd", cmd,
		"--env", "GO_WANT_HELPER_PROCESS=1",
		"--env", "GO_HELPER_CMD=echo",
		"--env", "GO_HELPER_ARG=hello",
	}); err != nil {
		t.Fatalf("runExec local success: %v", err)
	}
}

func TestRunExecLocalTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := fmt.Sprintf("%q -test.run=TestHelperProcess -- slow", exe)
	if err := runExec([]string{
		"--local", "--cmd", cmd, "--timeout", "100ms",
		"--env", "GO_WANT_HELPER_PROCESS=1",
		"--env", "GO_HELPER_CMD=slow",
	}); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRunExecLocalNonZeroExit(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	cmd := fmt.Sprintf("%q -test.run=TestHelperProcess -- exit", exe)
	err = runExec([]string{
		"--local", "--cmd", cmd,
		"--env", "GO_WANT_HELPER_PROCESS=1",
		"--env", "GO_HELPER_CMD=exit",
		"--env", "GO_HELPER_ARG=3",
	})
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	if !strings.Contains(err.Error(), "exit code 3") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExecDockerUnavailableSkips(t *testing.T) {
	if _, err := exec.LookPath("docker"); err == nil {
		t.Skip("docker is available; skipping unavailable-path test")
	}
	err := runExec([]string{"--docker", "--image", "busybox", "--cmd", "echo hi"})
	if err == nil {
		t.Fatal("expected error when docker is unavailable")
	}
	if !strings.Contains(err.Error(), "docker is not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunExecSSHUnavailableSkips(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err == nil {
		t.Skip("ssh is available; skipping unavailable-path test")
	}
	err := runExec([]string{"--ssh", "--host", "example.com", "--cmd", "echo hi"})
	if err == nil {
		t.Fatal("expected error when ssh is unavailable")
	}
	if !strings.Contains(err.Error(), "ssh is not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	switch os.Getenv("GO_HELPER_CMD") {
	case "echo":
		fmt.Fprintln(os.Stdout, os.Getenv("GO_HELPER_ARG"))
	case "slow":
		time.Sleep(5 * time.Second)
	case "exit":
		fmt.Fprintln(os.Stderr, "exit requested")
		os.Exit(3)
	default:
		fmt.Fprintln(os.Stderr, "unknown helper command")
		os.Exit(2)
	}
	os.Exit(0)
}
func TestBuildRepoContext(t *testing.T) {
	got := buildRepoContext(&repomode.Info{
		Root:          "/tmp/repo",
		Name:          "repo",
		TestDirs:      []string{"a", "b"},
		RecentChanges: []string{"abc123 feat", "def456 fix"},
	})
	want := "Repo context:\nRoot: /tmp/repo\nName: repo\nTestDirs: a,b\nRecent changes:\nabc123 feat\ndef456 fix"
	if got != want {
		t.Fatalf("buildRepoContext() = %q, want %q", got, want)
	}
}

func TestBuildRepoContextEmptyRecentChanges(t *testing.T) {
	got := buildRepoContext(&repomode.Info{
		Root:     "/tmp/repo",
		Name:     "repo",
		TestDirs: nil,
	})
	want := "Repo context:\nRoot: /tmp/repo\nName: repo\nTestDirs: \nRecent changes:"
	if got != want {
		t.Fatalf("buildRepoContext() = %q, want %q", got, want)
	}
}

func TestMaybeCheckUpdateSkipsWhenEnvSet(t *testing.T) {
	t.Setenv("AGENTFORGE_NO_UPDATE_CHECK", "1")
	maybeCheckUpdate()
}

func TestRunAgentLLMRequiresProvider(t *testing.T) {
	err := runAgent([]string{"--planner", "llm", "--prompt", "hello"})
	if err == nil || !strings.Contains(err.Error(), "--provider is required") {
		t.Fatalf("runAgent llm without provider error = %v, want --provider is required", err)
	}
}

func TestRunAgentUnknownPlanner(t *testing.T) {
	err := runAgent([]string{"--planner", "bogus", "--prompt", "hello"})
	if err == nil || !strings.Contains(err.Error(), "unknown planner") {
		t.Fatalf("runAgent unknown planner error = %v, want unknown planner", err)
	}
}

func TestGitflowCommitCleanTreeReportsNothingToCommit(t *testing.T) {
	dir := initCleanGitRepo(t)

	_, err := gitflow.Commit(context.Background(), gitflow.Config{WorkDir: dir, CommitMsg: "noop"})
	if err == nil {
		t.Fatal("Commit() on a clean tree succeeded, want an error")
	}
	if !strings.Contains(err.Error(), "nothing to commit") && !strings.Contains(err.Error(), "no changes added") {
		t.Fatalf("Commit() clean-tree error = %v, want it to include clean-tree output", err)
	}
}

func TestRunAgentRefusesCommitWhenTestsFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses sh -c")
	}
	dir := t.TempDir()
	err := runAgent([]string{
		"--yes",
		"--workdir", dir,
		"--commit",
		"--test", "exit 1",
		"--prompt", "edit file.txt hello",
	})
	if err == nil || !strings.Contains(err.Error(), "tests failed; refusing to commit") {
		t.Fatalf("runAgent error = %v, want tests-failed refusal", err)
	}
}

func TestRunMultiAgentPrintsFailuresBeforeAggregateError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses sh -c")
	}
	dir := t.TempDir()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	runErr := runMultiAgent([]string{"--yes", "--workdir", dir, "--tasks", "run false"})
	_ = w.Close()
	out, _ := io.ReadAll(r)

	if runErr == nil {
		t.Fatal("runMultiAgent() error = nil, want aggregate error")
	}
	if !strings.Contains(string(out), "task-0: ERROR") {
		t.Fatalf("runMultiAgent output does not include per-task failure before aggregate error:\n%s", out)
	}
}

func initCleanGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runTestGit(t, dir, "init")
	runTestGit(t, dir, "config", "user.name", "Test User")
	runTestGit(t, dir, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	runTestGit(t, dir, "add", ".")
	runTestGit(t, dir, "commit", "-m", "initial commit")
	return dir
}

func runTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestNewSessionStoreEncrypted(t *testing.T) {
	cfg := &config.Config{
		DataDir:         t.TempDir(),
		EncryptSessions: true,
		SecretKey:       "000102030405060708090a0b0c0d0e0f",
	}
	store, err := newSessionStore(cfg)
	if err != nil {
		t.Fatalf("newSessionStore() error = %v", err)
	}
	if _, ok := store.(*session.EncryptedStore); !ok {
		t.Fatalf("newSessionStore() = %T, want *session.EncryptedStore", store)
	}
}

func TestNewSessionStoreEncryptedRequiresKey(t *testing.T) {
	cfg := &config.Config{DataDir: t.TempDir(), EncryptSessions: true}
	if _, err := newSessionStore(cfg); err == nil {
		t.Fatal("newSessionStore() error = nil, want missing key error")
	}
}

func TestWarnPlaintextSecrets(t *testing.T) {
	cfg := &config.Config{
		ProviderKey: "sk-test",
		APIToken:    "token-test",
		SecretKey:   "key-test",
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stderr = w
	warnPlaintextSecrets(cfg)
	_ = w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)

	text := string(out)
	for _, want := range []string{"provider key provided as plaintext", "api token provided as plaintext", "secret key provided as plaintext"} {
		if !strings.Contains(text, want) {
			t.Fatalf("warning output does not contain %q:\n%s", want, text)
		}
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	src := t.TempDir()
	sessionDir := filepath.Join(src, "sessions")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "a.json"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}

	out := filepath.Join(t.TempDir(), "backup.zip")
	if err := runBackup([]string{"--out", out, "--data-dir", src}); err != nil {
		t.Fatalf("runBackup error: %v", err)
	}

	dst := t.TempDir()
	if err := runRestore([]string{"--in", out, "--data-dir", dst}); err != nil {
		t.Fatalf("runRestore error: %v", err)
	}

	restored := filepath.Join(dst, filepath.Base(src), "sessions", "a.json")
	data, err := os.ReadFile(restored)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("restored content = %q, want hello", data)
	}
}

func TestRunInitWritesConfig(t *testing.T) {
	dir := t.TempDir()
	if err := runInit([]string{"--api-url", "http://127.0.0.1:54695", "--data-dir", dir}); err != nil {
		t.Fatalf("runInit error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(data), "http://127.0.0.1:54695") {
		t.Fatalf("config does not contain api-url: %s", data)
	}
}

func TestRunMCPListTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := req["id"]
		method, _ := req["method"].(string)
		resp := map[string]any{"jsonrpc": "2.0", "id": id}
		switch method {
		case "initialize":
			resp["result"] = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
			}
		case "tools/list":
			resp["result"] = map[string]any{
				"tools": []any{
					map[string]any{"name": "echo", "description": "Echo tool", "inputSchema": map[string]any{}},
				},
			}
		default:
			resp["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	os.Stdout = w
	runErr := runMCP([]string{"--url", srv.URL, "--timeout", "5s"})
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)

	if runErr != nil {
		t.Fatalf("runMCP error: %v", runErr)
	}
	if !strings.Contains(string(out), "MCP connected") || !strings.Contains(string(out), "echo") {
		t.Fatalf("unexpected MCP output:\n%s", out)
	}
}

func TestRunDoctorDoesNotFail(t *testing.T) {
	if err := runDoctor([]string{"--data-dir", t.TempDir()}); err != nil {
		t.Fatalf("runDoctor error: %v", err)
	}
}
