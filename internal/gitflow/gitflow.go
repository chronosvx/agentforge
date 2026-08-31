// Package gitflow provides small helpers for automating Git and GitHub
// workflow operations used by agent-driven changes.
package gitflow

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Config contains the settings needed by the gitflow helpers.
type Config struct {
	WorkDir          string
	Branch           string
	CommitMsg        string
	PRTitle          string
	PRBody           string
	Push             bool
	AllowSecretFiles bool
}

// Result describes the outcome of an automated agent workflow.
type Result struct {
	Branch    string
	CommitSHA string
	Diff      string
	Pushed    bool
	PRURL     string
}

// EnsureBranch creates and checks out cfg.Branch. If cfg.Branch is empty it
// defaults to "agentflow/<timestamp>". If the branch already exists it is
// checked out instead of created.
func EnsureBranch(ctx context.Context, cfg Config) error {
	branch := cfg.Branch
	if branch == "" {
		branch = fmt.Sprintf("agentflow/%d", time.Now().Unix())
	}

	if _, err := runGit(ctx, cfg.WorkDir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err := runGit(ctx, cfg.WorkDir, "checkout", branch)
		return err
	}

	_, err := runGit(ctx, cfg.WorkDir, "checkout", "-b", branch)
	return err
}

// Diff returns the current working tree diff produced by git diff.
func Diff(ctx context.Context, workDir string) (string, error) {
	out, err := runGit(ctx, workDir, "diff")
	if err != nil {
		return "", err
	}
	return out, nil
}

// CheckNoSecretFiles scans workDir for common secret files and returns an error
// if any are found. It skips the .git directory.
func CheckNoSecretFiles(workDir string) error {
	var found []string
	err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != workDir && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if isSecretFileName(d.Name()) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) > 0 {
		return fmt.Errorf("gitflow: refusing to commit; secret files present: %s", strings.Join(found, ", "))
	}
	return nil
}

func isSecretFileName(name string) bool {
	lower := strings.ToLower(name)
	switch {
	case lower == ".env", lower == ".netrc", lower == "credentials.json",
		lower == "secrets.json", lower == "id_rsa", lower == "id_ed25519":
		return true
	case strings.HasPrefix(lower, ".env."):
		return true
	case strings.HasSuffix(lower, ".pem"), strings.HasSuffix(lower, ".key"),
		strings.HasSuffix(lower, ".secrets.json"):
		return true
	}
	return false
}

// Commit stages all changes, commits them with cfg.CommitMsg (or a default
// message when empty), and returns the resulting commit SHA.
func Commit(ctx context.Context, cfg Config) (string, error) {
	if !cfg.AllowSecretFiles {
		if err := CheckNoSecretFiles(cfg.WorkDir); err != nil {
			return "", err
		}
	}
	if _, err := runGit(ctx, cfg.WorkDir, "add", "-A"); err != nil {
		return "", err
	}

	msg := cfg.CommitMsg
	if msg == "" {
		msg = "agentflow: automated change"
	}

	if _, err := runGit(ctx, cfg.WorkDir, "commit", "-m", msg); err != nil {
		return "", err
	}

	out, err := runGit(ctx, cfg.WorkDir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Push pushes the current branch to origin and sets the upstream.
func Push(ctx context.Context, cfg Config) error {
	_, err := runGit(ctx, cfg.WorkDir, "push", "-u", "origin", cfg.Branch)
	return err
}

// CreatePR creates a GitHub pull request with gh. It uses cfg.PRTitle when set,
// otherwise it falls back to cfg.CommitMsg. The command is run in cfg.WorkDir.
// The trimmed stdout from gh is returned.
func CreatePR(ctx context.Context, cfg Config) (string, error) {
	title := cfg.PRTitle
	if title == "" {
		title = cfg.CommitMsg
	}

	args := []string{
		"pr", "create",
		"--title", title,
		"--body", cfg.PRBody,
		"--head", cfg.Branch,
	}

	out, err := runGH(ctx, cfg.WorkDir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runGit(ctx context.Context, workDir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", workDir}, args...)
	return runCmd(ctx, "", "git", fullArgs...)
}

func runGH(ctx context.Context, workDir string, args ...string) (string, error) {
	return runCmd(ctx, workDir, "gh", args...)
}

func runCmd(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		out := strings.TrimSpace(stdout.String())
		errText := strings.TrimSpace(stderr.String())
		detail := out
		if errText != "" {
			if detail != "" {
				detail += "\n"
			}
			detail += errText
		}
		cmdline := name
		if len(args) > 0 {
			cmdline += " " + strings.Join(args, " ")
		}
		if detail != "" {
			return "", fmt.Errorf("%s: %w: %s", cmdline, err, detail)
		}
		return "", fmt.Errorf("%s: %w", cmdline, err)
	}
	return stdout.String(), nil
}
