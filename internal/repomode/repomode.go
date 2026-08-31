package repomode

import (
	"context"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Info contains a compact snapshot of a repository or directory.
type Info struct {
	Root          string
	Name          string
	IsGit         bool
	Files         []string
	Readme        string
	GoMod         string
	TestDirs      []string
	RecentChanges []string
	Status        string
}

const (
	maxFiles     = 100
	maxTestDirs  = 50
	maxReadmeLen = 8000
	maxGoModLen  = 4000
)

// Scan inspects dir, locating the git root when dir is inside a git repository.
// Non-git directories are scanned using the supplied dir as the root.
func Scan(ctx context.Context, dir string) (*Info, error) {
	if dir == "" {
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	root := absDir
	isGit := false
	if out, err := gitOutput(ctx, absDir, "rev-parse", "--show-toplevel"); err == nil {
		if line := strings.TrimSpace(out); line != "" {
			root = filepath.Clean(line)
			isGit = true
		}
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	info := &Info{
		Root:  root,
		Name:  filepath.Base(root),
		IsGit: isGit,
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if len(info.Files) >= maxFiles {
			break
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info.Files = append(info.Files, entry.Name())
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	info.Readme = readFirstFile(ctx, root, []string{"README.md", "README"}, maxReadmeLen)
	info.GoMod = readFirstFile(ctx, root, []string{"go.mod"}, maxGoModLen)
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	testDirs, err := findTestDirs(ctx, root)
	if err != nil {
		return nil, err
	}
	info.TestDirs = testDirs

	if isGit {
		if out, err := gitOutput(ctx, root, "log", "--oneline", "-10"); err == nil {
			info.RecentChanges = splitLines(out)
		}
		if out, err := gitOutput(ctx, root, "status", "--short"); err == nil {
			info.Status = strings.TrimSpace(out)
		}
	}
	if err := checkContext(ctx); err != nil {
		return nil, err
	}

	return info, nil
}

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", dir}, args...)
	out, err := exec.CommandContext(ctx, "git", cmdArgs...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func splitLines(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func readFirstFile(ctx context.Context, root string, names []string, maxLen int64) string {
	for _, name := range names {
		if err := checkContext(ctx); err != nil {
			return ""
		}
		path := filepath.Join(root, name)
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if !fi.Mode().IsRegular() || fi.Mode()&os.ModeSymlink != 0 {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxLen))
		_ = f.Close()
		if readErr != nil {
			continue
		}
		return cleanText(string(data))
	}
	return ""
}

func cleanText(s string) string {
	return strings.TrimSpace(s)
}

func findTestDirs(ctx context.Context, root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := checkContext(ctx); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() || path == root {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), "_test.go") {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				dirs = append(dirs, rel)
				break
			}
		}
		if len(dirs) >= maxTestDirs {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	return dirs, nil
}
