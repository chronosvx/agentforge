package executor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Spec describes a command to execute.
type Spec struct {
	Command string
	Args    []string
	WorkDir string
	Env     []string
	Timeout time.Duration
}

// Result contains the captured output and exit status of a command.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Executor runs commands and returns their captured output.
type Executor interface {
	Run(ctx context.Context, spec Spec) (Result, error)
}

// LocalExecutor runs commands directly on the local machine without a shell.
type LocalExecutor struct{}

// NewLocalExecutor returns a LocalExecutor.
func NewLocalExecutor() *LocalExecutor {
	return &LocalExecutor{}
}

// Run executes spec.Command directly using os/exec. It never invokes a shell.
func (e *LocalExecutor) Run(ctx context.Context, spec Spec) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(spec.Command) == "" {
		return Result{}, errors.New("executor: command is empty")
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, spec.Command, spec.Args...)
	cmd.Dir = spec.WorkDir
	cmd.Env = mergeEnv(spec.Env)
	return runCommand(cmd)
}

// mergeEnv returns the current environment with extra KEY=VALUE entries applied.
// Duplicate keys in extra replace the inherited value instead of creating a
// second entry, which keeps behavior predictable across platforms.
func mergeEnv(extra []string) []string {
	if len(extra) == 0 {
		return nil
	}

	merged := os.Environ()
	index := make(map[string]int, len(merged)+len(extra))
	for i, kv := range merged {
		if k, _, ok := strings.Cut(kv, "="); ok {
			index[k] = i
		}
	}
	for _, kv := range extra {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			merged = append(merged, kv)
			continue
		}
		if i, exists := index[k]; exists {
			merged[i] = kv
		} else {
			index[k] = len(merged)
			merged = append(merged, kv)
		}
	}
	return merged
}

func runCommand(cmd *exec.Cmd) (Result, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if err == nil {
		return res, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
	}
	return res, err
}
