package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// helperBin 是测试用外部插件可执行文件，TestMain 中编译一次。
var helperBin string

const helperSource = `package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "slow":
			time.Sleep(5 * time.Second)
			fmt.Println("done")
			return
		case "cwd":
			wd, _ := os.Getwd()
			fmt.Println(wd)
			return
		case "--hook":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "hook requires name and event")
				os.Exit(2)
			}
			hook := os.Args[2]
			event := os.Args[3]
			if out := os.Getenv("DSH_PLUGIN_HOOK_OUT"); out != "" {
				_ = os.WriteFile(out, []byte(hook+"\n"+event+"\n"), 0o644)
			}
			if strings.Contains(event, "\"fail\":true") {
				fmt.Fprintln(os.Stderr, "hook failed")
				os.Exit(3)
			}
			fmt.Printf("hook %s %s\n", hook, event)
			return
		}
	}
	fmt.Printf("hello %s\n", strings.Join(os.Args[1:], " "))
}
`

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dsh-plugin-helper-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(helperSource), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	helperBin = filepath.Join(dir, "helper")
	if runtime.GOOS == "windows" {
		helperBin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", helperBin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build helper: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func writePluginManifest(t *testing.T, dir, name string, m Manifest, execPath string) {
	t.Helper()
	if m.Name == "" {
		m.Name = name
	}
	if len(m.Commands) == 0 {
		m.Commands = []string{name}
	}
	fm := fileManifest{Manifest: m, Executable: execPath}
	data, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+ManifestSuffix), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// copyTestHelper 把测试用 helper 可执行文件复制到插件目录，并返回可供 manifest 使用的相对路径。
func copyTestHelper(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(helperBin)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "helper")
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return "helper"
}

func TestLoadDirListAndRunExternal(t *testing.T) {
	dir := t.TempDir()
	writePluginManifest(t, dir, "hello", Manifest{
		Name:        "hello",
		Version:     "1.0.0",
		Description: "Say hello",
		Commands:    []string{"hi", "greet"},
		Hooks:       []string{"message"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, dir))

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	plugins := m.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %#v", len(plugins), plugins)
	}
	p := plugins[0]
	if p.Manifest.Name != "hello" || p.Manifest.Version != "1.0.0" || p.Manifest.Description != "Say hello" {
		t.Fatalf("unexpected manifest: %#v", p.Manifest)
	}
	if len(p.Manifest.Commands) != 2 || p.Manifest.Commands[0] != "hi" || p.Manifest.Commands[1] != "greet" {
		t.Fatalf("unexpected commands: %#v", p.Manifest.Commands)
	}
	if len(p.Manifest.Hooks) != 1 || p.Manifest.Hooks[0] != "message" {
		t.Fatalf("unexpected hooks: %#v", p.Manifest.Hooks)
	}
	if p.IsBuiltin() {
		t.Fatal("external plugin should not be builtin")
	}
	if p.Executable == "" {
		t.Fatal("external plugin should have executable path")
	}

	got, err := m.RunCommand("hello", []string{"world"})
	if err != nil {
		t.Fatalf("RunCommand(hello): %v", err)
	}
	if got != "hello world" {
		t.Fatalf("unexpected output: %q", got)
	}

	got, err = m.RunCommand("greet", []string{"gopher"})
	if err != nil {
		t.Fatalf("RunCommand(greet): %v", err)
	}
	if got != "hello gopher" {
		t.Fatalf("unexpected alias output: %q", got)
	}
}

func TestLoadDirMissingDir(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.LoadDir(filepath.Join(t.TempDir(), "not-exist")); err != nil {
		t.Fatalf("LoadDir missing dir should be nil, got %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("expected empty list, got %#v", got)
	}
}

func TestLoadDirNestedManifest(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugins", "hello")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePluginManifest(t, pluginDir, "hello", Manifest{
		Name:        "hello",
		Version:     "0.1.0",
		Description: "Nested hello",
		Commands:    []string{"hello"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, pluginDir))

	m := NewManager(root)
	if err := m.LoadDir(root); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	got, err := m.RunCommand("hello", []string{"nested"})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if got != "hello nested" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestLoadDirResolvesAdjacentExecutable(t *testing.T) {
	dir := t.TempDir()
	writePluginManifest(t, dir, "hello", Manifest{
		Name:        "hello",
		Version:     "1.0.0",
		Description: "Adjacent executable",
		Commands:    []string{"hello"},
	}, "")

	data, err := os.ReadFile(helperBin)
	if err != nil {
		t.Fatal(err)
	}
	execPath := filepath.Join(dir, "hello")
	if err := os.WriteFile(execPath, data, 0o755); err != nil {
		t.Fatal(err)
	}

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	plugins := m.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %#v", len(plugins), plugins)
	}
	if plugins[0].Executable != execPath {
		t.Fatalf("expected executable %q, got %q", execPath, plugins[0].Executable)
	}
}

func TestLoadDirRejectsAbsoluteExecutable(t *testing.T) {
	dir := t.TempDir()
	writePluginManifest(t, dir, "abs", Manifest{
		Name:        "abs",
		Version:     "1.0.0",
		Commands:    []string{"abs"},
		Permissions: Permissions{AllowExec: true},
	}, helperBin)

	m := NewManager(dir)
	err := m.LoadDir(dir)
	if err == nil {
		t.Fatal("expected LoadDir to reject absolute executable path")
	}
	if !strings.Contains(err.Error(), "must be a relative path") {
		t.Fatalf("expected relative path error, got %v", err)
	}
}

func TestLoadDirRejectsParentExecutableEscape(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 在插件目录外放一个可执行文件，manifest 使用 ../ 试图逃逸。
	outside := filepath.Join(root, "outside")
	data, err := os.ReadFile(helperBin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, data, 0o755); err != nil {
		t.Fatal(err)
	}
	writePluginManifest(t, pluginDir, "escape", Manifest{
		Name:        "escape",
		Version:     "1.0.0",
		Commands:    []string{"escape"},
		Permissions: Permissions{AllowExec: true},
	}, "../outside")

	m := NewManager(pluginDir)
	err = m.LoadDir(pluginDir)
	if err == nil {
		t.Fatal("expected LoadDir to reject parent path escape")
	}
	if !strings.Contains(err.Error(), "escapes plugin directory") {
		t.Fatalf("expected escape error, got %v", err)
	}
}

func TestManifestMissingPermissionsDeniedByDefault(t *testing.T) {
	dir := t.TempDir()
	copyTestHelper(t, dir)
	manifest := `{"name":"noperm","version":"1.0.0","description":"No permissions","commands":["noperm"],"executable":"helper"}`
	if err := os.WriteFile(filepath.Join(dir, "noperm"+ManifestSuffix), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	plugins := m.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %#v", len(plugins), plugins)
	}
	if plugins[0].Manifest.Permissions != (Permissions{}) {
		t.Fatalf("expected default empty permissions, got %#v", plugins[0].Manifest.Permissions)
	}
	_, err := m.RunCommand("noperm", nil)
	if err == nil {
		t.Fatal("expected RunCommand to be denied without AllowExec")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestManifestPermissionsParsed(t *testing.T) {
	dir := t.TempDir()
	copyTestHelper(t, dir)
	manifest := `{"name":"perm","version":"1.0.0","description":"Perm plugin","commands":["perm"],"executable":"helper","permissions":{"allowExec":true,"allowNetwork":true,"allowFileWrite":true,"allowReadAnyPath":true}}`
	if err := os.WriteFile(filepath.Join(dir, "perm"+ManifestSuffix), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	plugins := m.List()
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d: %#v", len(plugins), plugins)
	}
	want := Permissions{AllowNetwork: true, AllowExec: true, AllowFileWrite: true, AllowReadAnyPath: true}
	if plugins[0].Manifest.Permissions != want {
		t.Fatalf("unexpected permissions: %#v", plugins[0].Manifest.Permissions)
	}
}

func TestExternalCommandUsesManagerWorkDir(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writePluginManifest(t, pluginDir, "pwd", Manifest{
		Name:        "pwd",
		Commands:    []string{"pwd"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, pluginDir))

	m := NewManager(dataDir)
	if err := m.LoadDir(pluginDir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	got, err := m.RunCommand("pwd", []string{"cwd"})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(dataDir) {
		t.Fatalf("expected working dir %q, got %q", dataDir, got)
	}
}

func TestBuiltinEcho(t *testing.T) {
	m := NewManager(t.TempDir())
	err := m.RegisterBuiltin("echo", func(args []string) (string, error) {
		return strings.Join(args, " "), nil
	})
	if err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}

	got, err := m.RunCommand("echo", []string{"hello", "world"})
	if err != nil {
		t.Fatalf("RunCommand(echo): %v", err)
	}
	if got != "hello world" {
		t.Fatalf("unexpected output: %q", got)
	}

	plugins := m.List()
	if len(plugins) != 1 || !plugins[0].IsBuiltin() {
		t.Fatalf("expected builtin plugin in list, got %#v", plugins)
	}
}

func TestRunCommandNotFound(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.RunCommand("missing", nil)
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !errors.Is(err, ErrCommandNotFound) {
		t.Fatalf("expected ErrCommandNotFound, got %v", err)
	}
}

func TestExternalCommandTimeout(t *testing.T) {
	dir := t.TempDir()
	writePluginManifest(t, dir, "slow", Manifest{
		Name:        "slow",
		Version:     "1.0.0",
		Description: "Slow command",
		Commands:    []string{"slow"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, dir))

	m := NewManager(dir)
	m.SetTimeout(100 * time.Millisecond)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	start := time.Now()
	_, err := m.RunCommand("slow", []string{"slow"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout message, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestRegisterDuplicateBuiltinCanReplace(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.RegisterBuiltin("echo", func(args []string) (string, error) {
		return "first", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterBuiltin("echo", func(args []string) (string, error) {
		return "second", nil
	}); err != nil {
		t.Fatalf("replacing builtin should be allowed: %v", err)
	}
	got, err := m.RunCommand("echo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "second" {
		t.Fatalf("expected replacement output, got %q", got)
	}
}

func TestRegisterHookAndEmitBuiltin(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.RegisterBuiltin("echo", func(args []string) (string, error) {
		return strings.Join(args, " "), nil
	}); err != nil {
		t.Fatalf("RegisterBuiltin: %v", err)
	}

	wantErr := errors.New("hook boom")
	var calls int
	var events []map[string]any
	if err := m.RegisterHook("echo", HookOnMessage, func(event map[string]any) error {
		calls++
		events = append(events, event)
		return wantErr
	}); err != nil {
		t.Fatalf("RegisterHook: %v", err)
	}
	if err := m.RegisterHook("echo", HookOnMessage, func(event map[string]any) error {
		calls++
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("RegisterHook: %v", err)
	}

	event := map[string]any{"role": "user", "content": "hello world"}
	errs := m.Emit(HookOnMessage, event)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %#v", errs)
	}
	if calls != 2 {
		t.Fatalf("expected 2 hook calls, got %d", calls)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0]["content"] != "hello world" || events[1]["content"] != "hello world" {
		t.Fatalf("hook did not receive original event: %#v", events)
	}
	if !errors.Is(errs[0], wantErr) {
		t.Fatalf("expected hook error to be preserved, got %v", errs[0])
	}

	// 未注册的事件不应触发任何内置钩子。
	calls = 0
	events = nil
	if errs := m.Emit(HookOnCost, event); len(errs) != 0 {
		t.Fatalf("expected no errors for unregistered hook, got %#v", errs)
	}
	if calls != 0 {
		t.Fatalf("expected no hook calls for unregistered hook, got %d", calls)
	}
}

func TestRegisterHookValidation(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.RegisterHook("", HookOnMessage, func(map[string]any) error { return nil }); err == nil {
		t.Fatal("expected error when plugin name is empty")
	}
	if err := m.RegisterHook("echo", "", func(map[string]any) error { return nil }); err == nil {
		t.Fatal("expected error when hook name is empty")
	}
	if err := m.RegisterHook("echo", HookOnMessage, nil); err == nil {
		t.Fatal("expected error when hook callback is nil")
	}
}

func TestEmitExternalPluginHook(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "hook.out")
	writePluginManifest(t, dir, "hooker", Manifest{
		Name:        "hooker",
		Version:     "1.0.0",
		Description: "Hooker",
		Commands:    []string{"hooker"},
		Hooks:       []string{"on_message", "on_cost"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, dir))

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	t.Setenv("DSH_PLUGIN_HOOK_OUT", marker)

	event := map[string]any{"role": "user", "content": "hello world"}
	if errs := m.Emit(HookOnMessage, event); len(errs) != 0 {
		t.Fatalf("expected no errors, got %#v", errs)
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read hook marker: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 marker lines, got %q", string(data))
	}
	if lines[0] != "on_message" {
		t.Fatalf("expected on_message hook, got %q", lines[0])
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &got); err != nil {
		t.Fatalf("decode event JSON: %v", err)
	}
	if got["role"] != "user" || got["content"] != "hello world" {
		t.Fatalf("unexpected event received by external plugin: %#v", got)
	}

	// 未在 manifest 中声明的事件不应调用外部插件。
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if errs := m.Emit(HookOnDiff, event); len(errs) != 0 {
		t.Fatalf("expected no errors for undeclared hook, got %#v", errs)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected external plugin not to be called for undeclared hook, marker exists")
	}
}

func TestEmitExternalPluginHookError(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "hook.out")
	writePluginManifest(t, dir, "hooker", Manifest{
		Name:        "hooker",
		Version:     "1.0.0",
		Description: "Hooker",
		Commands:    []string{"hooker"},
		Hooks:       []string{"on_message"},
		Permissions: Permissions{AllowExec: true},
	}, copyTestHelper(t, dir))

	m := NewManager(dir)
	if err := m.LoadDir(dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	t.Setenv("DSH_PLUGIN_HOOK_OUT", marker)

	errs := m.Emit(HookOnMessage, map[string]any{"fail": true})
	if len(errs) != 1 {
		t.Fatalf("expected 1 error from external hook, got %#v", errs)
	}
	if !strings.Contains(errs[0].Error(), "hooker") {
		t.Fatalf("expected error to mention plugin name, got %v", errs[0])
	}
}
