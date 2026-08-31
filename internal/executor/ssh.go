package executor

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// SSHExecutor runs commands on a remote host using the ssh CLI.
type SSHExecutor struct {
	// Host is the remote host or user@host destination.
	Host string
	// User optionally overrides the remote user.
	User string
	// Port optionally overrides the SSH port.
	Port int
	// BinPath optionally overrides the ssh executable. It is useful for tests.
	BinPath string
}

// NewSSHExecutor returns an SSHExecutor for the given host, user and port.
// An empty user and a zero port use ssh's defaults.
func NewSSHExecutor(host, user string, port int) *SSHExecutor {
	return &SSHExecutor{Host: host, User: user, Port: port}
}

func (e *SSHExecutor) bin() string {
	if e.BinPath != "" {
		return e.BinPath
	}
	return "ssh"
}

// Available reports whether an ssh client is installed and a host is configured.
func (e *SSHExecutor) Available() bool {
	if strings.TrimSpace(e.Host) == "" {
		return false
	}
	_, err := exec.LookPath(e.bin())
	return err == nil
}

// Run executes the command on the remote host with `ssh host -- cmd`.
func (e *SSHExecutor) Run(ctx context.Context, spec Spec) (Result, error) {
	if strings.TrimSpace(e.Host) == "" {
		return Result{}, errors.New("executor: ssh host is empty")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return Result{}, errors.New("executor: command is empty")
	}
	if !e.Available() {
		return Result{}, errors.New("executor: ssh is not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, e.bin(), e.commandArgs(spec)...)
	return runCommand(cmd)
}

func (e *SSHExecutor) commandArgs(spec Spec) []string {
	args := []string{}
	if e.Port > 0 {
		args = append(args, "-p", strconv.Itoa(e.Port))
	}

	target := e.Host
	if e.User != "" && !strings.Contains(target, "@") {
		target = e.User + "@" + target
	}
	args = append(args, target, "--")
	if spec.Command != "" {
		args = append(args, spec.Command)
	}
	args = append(args, spec.Args...)
	return args
}
