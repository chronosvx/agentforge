package agentflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// worktreeEntry 记录 Git 工作区中一个脏路径的状态与内容指纹。
type worktreeEntry struct {
	Status string
	Hash   string
}

type worktreeState map[string]worktreeEntry

// captureWorktreeState 读取 git status 并对每个脏/未跟踪文件计算内容指纹。
// 该函数只读，不会改变索引或工作树。
func captureWorktreeState(ctx context.Context, dir string) (worktreeState, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	state := make(worktreeState)
	records := strings.Split(string(out), "\x00")
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if len(rec) < 4 {
			continue
		}
		status := rec[:2]
		path := rec[3:]
		// -z 模式下重命名/复制记录后紧跟原始路径，跳过它。
		if status[0] == 'R' || status[0] == 'C' || status[1] == 'R' || status[1] == 'C' {
			i++
		}
		state[path] = worktreeEntry{Status: status, Hash: fileFingerprint(filepath.Join(dir, path))}
	}
	return state, nil
}

func fileFingerprint(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "<missing>"
		}
		return "<error>"
	}
	if info.IsDir() {
		return "<dir>"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "<error>"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// splitWorktreeChanges 把运行后的脏路径分成三类：
// 本任务改动、运行前已存在且未变、以及运行前已脏又被任务修改的冲突路径。
func splitWorktreeChanges(before, after worktreeState) (task, preExisting, conflicted []string) {
	for path, afterEntry := range after {
		beforeEntry, existed := before[path]
		if !existed {
			task = append(task, path)
			continue
		}
		if beforeEntry.Hash == afterEntry.Hash && beforeEntry.Status == afterEntry.Status {
			preExisting = append(preExisting, path)
			continue
		}
		task = append(task, path)
		conflicted = append(conflicted, path)
	}
	sort.Strings(task)
	sort.Strings(preExisting)
	sort.Strings(conflicted)
	return task, preExisting, conflicted
}

func mergeSorted(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, path := range list {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// worktreeDiff 生成指定路径相对 HEAD 的完整 Diff（含已暂存、未暂存和未跟踪文件）。
func worktreeDiff(ctx context.Context, dir string, paths []string, state worktreeState) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}

	var tracked []string
	var untracked []string
	for _, path := range paths {
		if entry, ok := state[path]; ok && entry.Status == "??" {
			untracked = append(untracked, path)
			continue
		}
		tracked = append(tracked, path)
	}

	var b strings.Builder
	if len(tracked) > 0 {
		args := append([]string{"-C", dir, "diff", "HEAD", "--no-color", "--"}, tracked...)
		out, err := exec.CommandContext(ctx, "git", args...).Output()
		if err != nil {
			return "", err
		}
		b.Write(out)
	}

	for _, path := range untracked {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteByte('\n')
		}
		b.WriteString(newFileDiff(dir, path))
	}

	return b.String(), nil
}

// newFileDiff 为未跟踪文件生成最小 unified diff，避免依赖平台相关的
// `git diff --no-index /dev/null` 行为。
func newFileDiff(dir, path string) string {
	full := filepath.Join(dir, path)
	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Sprintf("diff --git a/%s b/%s\nnew file (unreadable: %v)\n", path, path, err)
	}

	mode := "100644"
	if info, err := os.Stat(full); err == nil && info.Mode().Perm()&0o111 != 0 {
		mode = "100755"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode %s\n", path, path, mode)
	if bytes.IndexByte(data, 0) >= 0 {
		fmt.Fprintf(&b, "Binary files /dev/null and b/%s differ\n", path)
		return b.String()
	}

	text := string(data)
	lines := strings.Split(text, "\n")
	trailingNewline := strings.HasSuffix(text, "\n")
	if trailingNewline {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return b.String()
	}

	fmt.Fprintf(&b, "--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, len(lines))
	for _, line := range lines {
		b.WriteString("+" + line + "\n")
	}
	if !trailingNewline {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}
