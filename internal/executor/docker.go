package executor

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// DockerExecutor runs commands inside a container using the docker CLI.
type DockerExecutor struct {
	// Image is the container image to run.
	Image string
	// BinPath optionally overrides the docker executable. It is useful for
	// tests and for installations where docker is not on PATH.
	BinPath string
}

// NewDockerExecutor returns a DockerExecutor for the given image.
func NewDockerExecutor(image string) *DockerExecutor {
	return &DockerExecutor{Image: image}
}

func (e *DockerExecutor) bin() string {
	if e.BinPath != "" {
		return e.BinPath
	}
	return "docker"
}

// Available reports whether the docker CLI exists and the daemon is reachable.
func (e *DockerExecutor) Available() bool {
	bin := e.bin()
	if _, err := exec.LookPath(bin); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "version", "--format", "{{.Server.Version}}")
	return cmd.Run() == nil
}

// Run executes the command in a throwaway container.
//
// When Spec.WorkDir is set, it is mounted at /work and used as the container
// working directory. Spec.Env entries are passed through with -e.
func (e *DockerExecutor) Run(ctx context.Context, spec Spec) (Result, error) {
	if strings.TrimSpace(e.Image) == "" {
		return Result{}, errors.New("executor: docker image is empty")
	}
	if strings.TrimSpace(spec.Command) == "" {
		return Result{}, errors.New("executor: command is empty")
	}
	if !e.Available() {
		return Result{}, errors.New("executor: docker is not available")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	args := []string{"run", "--rm"}
	if spec.WorkDir != "" {
		args = append(args, "-v", spec.WorkDir+":/work", "-w", "/work")
	}
	for _, env := range spec.Env {
		args = append(args, "-e", env)
	}
	args = append(args, e.Image, spec.Command)
	args = append(args, spec.Args...)

	cmd := exec.CommandContext(ctx, e.bin(), args...)
	return runCommand(cmd)
}
