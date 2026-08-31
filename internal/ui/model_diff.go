package ui

import (
	"fmt"
	"github.com/chronosvx/agentforge/internal/i18n"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (m *Model) handleApplyKey(key string) bool {
	if len(m.applyFiles) == 0 {
		return false
	}
	switch key {
	case "j", "down":
		if m.applyCursor < len(m.applyFiles)-1 {
			m.applyCursor++
		}
		return true
	case "k", "up":
		if m.applyCursor > 0 {
			m.applyCursor--
		}
		return true
	case " ":
		file := m.applyFiles[m.applyCursor]
		if m.applySelected == nil {
			m.applySelected = map[string]bool{}
		}
		m.applySelected[file] = !m.applySelected[file]
		return true
	case "g":
		m.pendingApply = false
		out, err := m.applyAcceptedDiffsWithFilter(m.applySelected)
		if err != nil {
			m.status = "git apply error: " + err.Error()
		} else {
			m.status = "git apply ok: " + out
		}
		return true
	case "esc":
		m.pendingApply = false
		m.status = "git apply cancelled"
		return true
	}
	return false
}

// handleDiffKey 处理 Diff Review 面板的键盘操作，返回是否已消费。
func (m *Model) handleDiffKey(key string) bool {
	if len(m.diffs) == 0 {
		return false
	}
	if m.pendingApply {
		return m.handleApplyKey(key)
	}
	switch key {
	case "j", "down":
		if m.diffCursor < len(m.diffs)-1 {
			m.diffCursor++
		}
		return true
	case "k", "up":
		if m.diffCursor > 0 {
			m.diffCursor--
		}
		return true
	case "a":
		if m.diffDecisions == nil {
			m.diffDecisions = map[int]string{}
		}
		m.diffDecisions[m.diffCursor] = "accepted"
		m.status = fmt.Sprintf("diff %d accepted", m.diffCursor+1)
		return true
	case "r":
		if m.diffDecisions == nil {
			m.diffDecisions = map[int]string{}
		}
		m.diffDecisions[m.diffCursor] = "rejected"
		m.status = fmt.Sprintf("diff %d rejected", m.diffCursor+1)
		return true
	case "p":
		path, err := m.savePatch()
		if err != nil {
			m.status = "save patch error: " + err.Error()
		} else {
			m.status = "patch saved: " + path
		}
		return true
	case "g":
		if !m.pendingApply {
			m.pendingApply = true
			m.applyFiles = acceptedDiffFiles(m.diffs, m.diffDecisions)
			m.applyCursor = 0
			m.applySelected = map[string]bool{}
			for _, f := range m.applyFiles {
				m.applySelected[f] = true
			}
			m.status = i18n.T("status.apply_confirm")
			return true
		}
		m.pendingApply = false
		out, err := m.applyAcceptedDiffsWithFilter(m.applySelected)
		if err != nil {
			m.status = "git apply error: " + err.Error()
		} else {
			m.status = "git apply ok: " + out
		}
		return true
	case "b":
		if m.lastBackupDir == "" {
			m.status = "no backup available"
			return true
		}
		repoDir := m.cfg.Workspace
		if repoDir == "" {
			repoDir = "."
		}
		if err := m.restoreBackup(m.lastBackupDir, repoDir); err != nil {
			m.status = "rollback error: " + err.Error()
		} else {
			m.status = "rolled back from " + m.lastBackupDir
		}
		return true
	case "esc":
		m.pendingApply = false
		m.status = "git apply cancelled"
		return true
	}
	return false
}

func (m *Model) savePatch() (string, error) {
	var b strings.Builder
	for i, raw := range m.diffs {
		if m.diffDecisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		diff, _ := item["diff"].(string)
		if diff != "" {
			b.WriteString(diff)
			if !strings.HasSuffix(diff, "\n") {
				b.WriteString("\n")
			}
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no accepted diffs")
	}

	dir := filepath.Join(m.cfg.DataDir, "patches")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%d.patch", sanitizeSessionID(m.sessionID), time.Now().UnixMilli())
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func acceptedDiffFiles(diffs []any, decisions map[int]string) []string {
	seen := map[string]bool{}
	var out []string
	for i, raw := range diffs {
		if decisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, file)
	}
	return out
}

func cleanDiffPath(file string) (string, error) {
	if file == "" {
		return "", fmt.Errorf("empty diff path")
	}
	if strings.ContainsAny(file, `\`) || filepath.IsAbs(file) || strings.HasPrefix(file, "/") {
		return "", fmt.Errorf("invalid diff path: %q", file)
	}
	clean := filepath.Clean(file)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid diff path: %q", file)
	}
	return clean, nil
}

func (m *Model) backupFiles(files []string, repoDir string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to backup")
	}
	backupRoot := filepath.Join(m.cfg.DataDir, "backups")
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		return "", err
	}
	dir := filepath.Join(backupRoot, fmt.Sprintf("%s-%d", sanitizeSessionID(m.sessionID), time.Now().UnixMilli()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, file := range files {
		safe, err := cleanDiffPath(file)
		if err != nil {
			return "", err
		}
		src := filepath.Join(repoDir, safe)
		data, err := os.ReadFile(src)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		dst := filepath.Join(dir, safe)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (m *Model) restoreBackup(backupDir, repoDir string) error {
	return filepath.Walk(backupDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(backupDir, path)
		if err != nil {
			return err
		}
		if _, err := cleanDiffPath(rel); err != nil {
			return err
		}
		dst := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, info.Mode().Perm())
	})
}

func (m *Model) applyAcceptedDiffs() (string, error) {
	return m.applyAcceptedDiffsWithFilter(nil)
}

func (m *Model) applyAcceptedDiffsWithFilter(filter map[string]bool) (string, error) {
	var b strings.Builder
	var files []string
	for i, raw := range m.diffs {
		if m.diffDecisions[i] != "accepted" {
			continue
		}
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		diff, _ := item["diff"].(string)
		if diff == "" {
			continue
		}
		if filter != nil && !filter[file] {
			continue
		}
		safe, err := cleanDiffPath(file)
		if err != nil {
			return "", err
		}
		b.WriteString(diff)
		if !strings.HasSuffix(diff, "\n") {
			b.WriteString("\n")
		}
		files = append(files, safe)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("no accepted diffs selected")
	}

	tmp, err := os.CreateTemp("", "agentforge-apply-*.patch")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	repoDir := m.cfg.Workspace
	if repoDir == "" {
		repoDir = "."
	}

	backupDir, err := m.backupFiles(files, repoDir)
	if err != nil {
		return "", fmt.Errorf("backup before apply: %w", err)
	}
	m.lastBackupDir = backupDir

	check := exec.Command("git", "-C", repoDir, "apply", "--check", tmpPath)
	if out, err := check.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git apply --check: %v: %s", err, strings.TrimSpace(string(out)))
	}

	apply := exec.Command("git", "-C", repoDir, "apply", tmpPath)
	if out, err := apply.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git apply: %v: %s", err, strings.TrimSpace(string(out)))
	}

	m.auditEvent("diff.apply", map[string]any{
		"sessionId": m.sessionID,
		"files":     files,
		"backupDir": backupDir,
	})
	return "patch applied (backup: " + backupDir + ")", nil
}
func (m *Model) renderApplyConfirm() string {
	var b strings.Builder
	b.WriteString(i18n.T("panel.apply_confirm") + "\n")
	for i, file := range m.applyFiles {
		marker := " "
		if i == m.applyCursor {
			marker = ">"
		}
		sel := " "
		if m.applySelected[file] {
			sel = "x"
		}
		b.WriteString(fmt.Sprintf("%s [%s] %s\n", marker, sel, file))
	}
	return b.String()
}
func (m *Model) renderDiffPanel() string {
	if len(m.diffs) == 0 {
		return i18n.T("panel.diff_empty")
	}
	if m.pendingApply {
		return m.renderApplyConfirm()
	}
	var b strings.Builder
	b.WriteString(i18n.T("panel.diff_title") + "\n")
	for i, raw := range m.diffs {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		file, _ := item["file"].(string)
		diff, _ := item["diff"].(string)
		marker := " "
		if i == m.diffCursor {
			marker = ">"
		}
		decision := "pending"
		if d, ok := m.diffDecisions[i]; ok {
			decision = d
		}
		b.WriteString(fmt.Sprintf("\n%s[%d] %s  [%s]\n%s\n", marker, i+1, sanitizeTerminalText(file), decision, sanitizeTerminalText(diff)))
	}
	return b.String()
}
