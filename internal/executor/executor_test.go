package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const executorHelperSource = `package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	name = strings.TrimSuffix(name, filepath.Ext(name))
	switch name {
	case "docker":
		if len(os.Args) > 1 && os.Args[1] == "version" {
			fmt.Println("Docker version 20.10.0")
			return
		}
		fmt.Printf("docker %s\n", strings.Join(os.Args[1:], " "))
	case "ssh":
		fmt.Printf("ssh %s\n", strings.Join(os.Args[1:], " "))
	default:
		fmt.Fprintln(os.Stderr, "unknown executor helper mode")
		os.Exit(2)
	}
}
`

var (
	dockerBin string
	sshBin    string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dsh-executor-helper-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(executorHelperSource), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	helper := filepath.Join(dir, "helper"+ext)
	cmd := exec.Command("go", "build", "-o", helper, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build executor helper: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	dockerBin = filepath.Join(dir, "docker"+ext)
	sshBin = filepath.Join(dir, "ssh"+ext)
	if err := copyExecutable(helper, dockerBin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	if err := copyExecutable(helper, sshBin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func copyExecutable(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return err
	}
	return nil
}

func TestLocalExecutorEcho(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	e := NewLocalExecutor()
	res, err := e.Run(context.Background(), Spec{
		Command: exe,
		Args:    []string{"-test.run=TestHelperProcess", "--", "echo"},
		Env:     []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=echo", "GO_HELPER_ARG=hello"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Stdout != "hello\n" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
	if res.Stderr != "" {
		t.Fatalf("unexpected stderr: %q", res.Stderr)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestLocalExecutorTimeout(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e := NewLocalExecutor()
	_, err = e.Run(ctx, Spec{
		Command: exe,
		Args:    []string{"-test.run=TestHelperProcess", "--", "slow"},
		Env:     []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=slow"},
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("expected context deadline exceeded, got %v", ctx.Err())
	}
}

func TestLocalExecutorNonZeroExit(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}

	e := NewLocalExecutor()
	res, err := e.Run(context.Background(), Spec{
		Command: exe,
		Args:    []string{"-test.run=TestHelperProcess", "--", "exit"},
		Env:     []string{"GO_WANT_HELPER_PROCESS=1", "GO_HELPER_CMD=exit", "GO_HELPER_ARG=3"},
	})
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	if res.ExitCode != 3 {
		t.Fatalf("expected exit code 3, got %d", res.ExitCode)
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

func TestDockerExecutorRun(t *testing.T) {
	d := &DockerExecutor{Image: "test-image", BinPath: dockerBin}
	workDir := t.TempDir()

	if !d.Available() {
		t.Skip("docker not available")
	}

	res, err := d.Run(context.Background(), Spec{
		Command: "echo",
		Args:    []string{"hi"},
		WorkDir: workDir,
		Env:     []string{"A=B"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Stdout, "docker run --rm") {
		t.Fatalf("expected docker run invocation, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "-v "+workDir+":/work") {
		t.Fatalf("expected workdir mount, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "-w /work") {
		t.Fatalf("expected workdir flag, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "-e A=B") {
		t.Fatalf("expected env flag, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "test-image echo hi") {
		t.Fatalf("expected image and command, got %q", res.Stdout)
	}
}

func TestSSHExecutorRun(t *testing.T) {
	s := &SSHExecutor{Host: "example.com", User: "alice", Port: 2222, BinPath: sshBin}

	res, err := s.Run(context.Background(), Spec{
		Command: "echo",
		Args:    []string{"hi"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Stdout, "-p 2222") {
		t.Fatalf("expected port flag, got %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "alice@example.com -- echo hi") {
		t.Fatalf("expected ssh invocation, got %q", res.Stdout)
	}
}

func TestDockerExecutorSkipsWhenUnavailable(t *testing.T) {
	d := NewDockerExecutor("busybox:latest")
	if !d.Available() {
		t.Skip("docker not available")
	}
}

func TestSSHExecutorSkipsWhenUnavailable(t *testing.T) {
	s := NewSSHExecutor("localhost", "", 22)
	if !s.Available() {
		t.Skip("ssh not available")
	}
}

func TestDockerExecutorRunWithoutDockerFails(t *testing.T) {
	d := &DockerExecutor{Image: "test-image", BinPath: filepath.Join(t.TempDir(), "missing-docker")}
	if d.Available() {
		t.Fatal("expected missing docker to be unavailable")
	}
	_, err := d.Run(context.Background(), Spec{Command: "echo"})
	if err == nil {
		t.Fatal("expected error when docker is unavailable")
	}
	if !strings.Contains(err.Error(), "docker is not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSSHExecutorRunWithoutSSHFails(t *testing.T) {
	s := &SSHExecutor{Host: "localhost", BinPath: filepath.Join(t.TempDir(), "missing-ssh")}
	if s.Available() {
		t.Fatal("expected missing ssh to be unavailable")
	}
	_, err := s.Run(context.Background(), Spec{Command: "echo"})
	if err == nil {
		t.Fatal("expected error when ssh is unavailable")
	}
	if !strings.Contains(err.Error(), "ssh is not available") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLocalExecutorRejectsEmptyCommand(t *testing.T) {
	_, err := NewLocalExecutor().Run(context.Background(), Spec{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "command is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
