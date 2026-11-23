// Package session 的同步与批量导入导出实现。
//
// 这些函数既接受数据目录（<data-dir>，会话位于其 sessions/ 子目录），
// 也接受直接指向 sessions/ 的目录，方便命令行使用与测试。
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExportReport 汇总一次批量导出的结果，区分明文/加密与跳过/失败。
type ExportReport struct {
	Paths     []string
	Plaintext int
	Encrypted int
	Skipped   []string
	Failed    []string
}

// ImportReport 汇总一次批量导入的结果。
type ImportReport struct {
	Imported []string
	Skipped  []string
	Failed   []string
}

// ExportAll 将 dir 中的全部会话导出为 JSON/加密文件到 outDir。
// 返回每个导出文件的路径。
func ExportAll(dir, outDir string) ([]string, error) {
	report, err := ExportAllWithReport(dir, outDir)
	if err != nil {
		return nil, err
	}
	return report.Paths, nil
}

// ExportAllWithReport 导出明文 <id>.json 与加密 <id>.json.enc 会话。
// 加密文件按密文原样复制，不需要主密钥；同一 ID 两种格式同时存在时优先加密文件。
func ExportAllWithReport(dir, outDir string) (*ExportReport, error) {
	if dir == "" {
		return nil, errors.New("session: export source dir is empty")
	}
	if outDir == "" {
		return nil, errors.New("session: export out dir is empty")
	}

	sessionsDir, err := resolveSessionsDir(dir)
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(sessionsDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("session: read sessions dir: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, fmt.Errorf("session: create export dir: %w", err)
	}

	type candidate struct {
		encrypted string
		plain     string
	}
	candidates := make(map[string]*candidate)
	report := &ExportReport{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		switch {
		case strings.HasSuffix(name, ".json.enc"):
			id := strings.TrimSuffix(name, ".json.enc")
			c := candidates[id]
			if c == nil {
				c = &candidate{}
				candidates[id] = c
			}
			c.encrypted = filepath.Join(sessionsDir, name)
		case strings.HasSuffix(name, ".json"):
			id := strings.TrimSuffix(name, ".json")
			c := candidates[id]
			if c == nil {
				c = &candidate{}
				candidates[id] = c
			}
			c.plain = filepath.Join(sessionsDir, name)
		}
	}

	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		if err := validateID(id); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: invalid id", id))
			continue
		}
		c := candidates[id]
		if c.encrypted != "" {
			data, err := os.ReadFile(c.encrypted)
			if err != nil {
				report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			path := filepath.Join(outDir, id+".json.enc")
			if err := writeFileAtomic(outDir, filepath.Base(path), data, 0o600); err != nil {
				report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", id, err))
				continue
			}
			report.Paths = append(report.Paths, path)
			report.Encrypted++
			if c.plain != "" {
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s.json: duplicate of encrypted session %s", id, id))
			}
			continue
		}

		entry, err := readEntryFile(c.plain, id)
		if err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		data = append(data, '\n')
		path := filepath.Join(outDir, id+".json")
		if err := writeFileAtomic(outDir, filepath.Base(path), data, 0o600); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		report.Paths = append(report.Paths, path)
		report.Plaintext++
	}

	return report, nil
}

// ImportAll 从 inDir 导入会话到 dataDir 的 sessions/ 目录。
// 已存在的会话 ID 不会被覆盖，而是记录为跳过；返回每个文件的处理结果。
func ImportAll(inDir, dataDir string) ([]string, error) {
	report, err := ImportAllWithReport(inDir, dataDir)
	if err != nil {
		return nil, err
	}
	results := make([]string, 0, len(report.Imported)+len(report.Skipped)+len(report.Failed))
	for _, item := range report.Imported {
		results = append(results, "imported "+item)
	}
	for _, item := range report.Skipped {
		results = append(results, "skip "+item)
	}
	for _, item := range report.Failed {
		results = append(results, "failed "+item)
	}
	return results, nil
}

// ImportAllWithReport 导入明文与加密会话；同 ID 双格式时优先加密文件。
func ImportAllWithReport(inDir, dataDir string) (*ImportReport, error) {
	if inDir == "" {
		return nil, errors.New("session: import source dir is empty")
	}
	if dataDir == "" {
		return nil, errors.New("session: import data dir is empty")
	}

	sessionsDir, err := resolveSessionsDir(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("session: create sessions dir: %w", err)
	}

	files, err := os.ReadDir(inDir)
	if err != nil {
		return nil, fmt.Errorf("session: read import dir: %w", err)
	}

	type candidate struct {
		encrypted string
		plain     string
	}
	candidates := make(map[string]*candidate)
	report := &ImportReport{}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		switch {
		case strings.HasSuffix(name, ".json.enc"):
			id := strings.TrimSuffix(name, ".json.enc")
			c := candidates[id]
			if c == nil {
				c = &candidate{}
				candidates[id] = c
			}
			c.encrypted = filepath.Join(inDir, name)
		case strings.HasSuffix(name, ".json"):
			id := strings.TrimSuffix(name, ".json")
			c := candidates[id]
			if c == nil {
				c = &candidate{}
				candidates[id] = c
			}
			c.plain = filepath.Join(inDir, name)
		}
	}

	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, name := range ids {
		if err := validateID(name); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: invalid id", name))
			continue
		}
		c := candidates[name]

		if c.encrypted != "" {
			if targetExists(sessionsDir, name) {
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s: already exists", name))
				if c.plain != "" {
					report.Skipped = append(report.Skipped, fmt.Sprintf("%s.json: duplicate of encrypted session %s", name, name))
				}
				continue
			}
			data, err := os.ReadFile(c.encrypted)
			if err != nil {
				report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			if err := writeFileAtomic(sessionsDir, name+".json.enc", data, 0o600); err != nil {
				report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", name, err))
				continue
			}
			report.Imported = append(report.Imported, name+" (encrypted)")
			if c.plain != "" {
				report.Skipped = append(report.Skipped, fmt.Sprintf("%s.json: duplicate of encrypted session %s", name, name))
			}
			continue
		}

		entry, err := readEntryFile(c.plain, name)
		if err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		if err := validateID(entry.ID); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: invalid id %q", name, entry.ID))
			continue
		}
		if targetExists(sessionsDir, entry.ID) {
			report.Skipped = append(report.Skipped, fmt.Sprintf("%s: %s already exists", name, entry.ID))
			continue
		}
		if err := writeSession(sessionsDir, entry); err != nil {
			report.Failed = append(report.Failed, fmt.Sprintf("%s: %v", entry.ID, err))
			continue
		}
		report.Imported = append(report.Imported, entry.ID)
	}

	return report, nil
}

func targetExists(sessionsDir, id string) bool {
	if _, err := os.Stat(filepath.Join(sessionsDir, id+".json")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(sessionsDir, id+".json.enc")); err == nil {
		return true
	}
	return false
}

func readEntryFile(path, fallbackID string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return Entry{}, fmt.Errorf("invalid JSON")
	}
	if entry.ID == "" {
		entry.ID = fallbackID
	}
	return entry, nil
}

func writeFileAtomic(dir, name string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(dir, ".tmp-"+name+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}

// SyncDir 在 srcDir 与 dstDir 之间按 UpdatedAt 做双向同步：
// 只存在于一侧的会话复制到另一侧；两侧都存在时较新的覆盖较旧的。
// 返回 map[会话ID]操作，操作值为 "src->dst" 或 "dst->src"。
func SyncDir(srcDir, dstDir string) (map[string]string, error) {
	if srcDir == "" {
		return nil, errors.New("session: sync src dir is empty")
	}
	if dstDir == "" {
		return nil, errors.New("session: sync dst dir is empty")
	}

	srcSessionsDir, err := resolveSessionsDir(srcDir)
	if err != nil {
		return nil, err
	}
	dstSessionsDir, err := resolveSessionsDir(dstDir)
	if err != nil {
		return nil, err
	}

	srcEntries, err := readSessionsDir(srcSessionsDir)
	if err != nil {
		return nil, err
	}
	dstEntries, err := readSessionsDir(dstSessionsDir)
	if err != nil {
		return nil, err
	}

	srcMap := make(map[string]Entry, len(srcEntries))
	for _, entry := range srcEntries {
		srcMap[entry.ID] = entry
	}
	dstMap := make(map[string]Entry, len(dstEntries))
	for _, entry := range dstEntries {
		dstMap[entry.ID] = entry
	}

	ops := make(map[string]string)

	// 只存在于 src 的会话复制到 dst。
	for id, entry := range srcMap {
		if _, ok := dstMap[id]; !ok {
			if err := writeSession(dstSessionsDir, entry); err != nil {
				return nil, fmt.Errorf("session: sync %s to dst: %w", id, err)
			}
			ops[id] = "src->dst"
		}
	}

	// 只存在于 dst 的会话复制到 src。
	for id, entry := range dstMap {
		if _, ok := srcMap[id]; !ok {
			if err := writeSession(srcSessionsDir, entry); err != nil {
				return nil, fmt.Errorf("session: sync %s to src: %w", id, err)
			}
			ops[id] = "dst->src"
		}
	}

	// 两侧都存在时，较新的覆盖较旧的。
	for id, srcEntry := range srcMap {
		dstEntry, ok := dstMap[id]
		if !ok {
			continue
		}
		if srcEntry.UpdatedAt.After(dstEntry.UpdatedAt) {
			if err := writeSession(dstSessionsDir, srcEntry); err != nil {
				return nil, fmt.Errorf("session: sync %s to dst: %w", id, err)
			}
			ops[id] = "src->dst"
		} else if dstEntry.UpdatedAt.After(srcEntry.UpdatedAt) {
			if err := writeSession(srcSessionsDir, dstEntry); err != nil {
				return nil, fmt.Errorf("session: sync %s to src: %w", id, err)
			}
			ops[id] = "dst->src"
		}
	}

	return ops, nil
}

// resolveSessionsDir 把用户传入的目录解析为实际存放会话 JSON 的目录。
// 兼容两种用法：传入数据目录（含 sessions/ 子目录），或直接传入 sessions 目录。
func resolveSessionsDir(dir string) (string, error) {
	if dir == "" {
		return "", errors.New("session: empty directory")
	}

	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		sub := filepath.Join(dir, "sessions")
		if subInfo, err := os.Stat(sub); err == nil && subInfo.IsDir() {
			return sub, nil
		}
		if filepath.Base(dir) == "sessions" {
			return dir, nil
		}
		// 如果目录里直接有 JSON 文件，按 sessions 目录处理。
		entries, err := os.ReadDir(dir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
					return dir, nil
				}
			}
		}
		return sub, nil
	}

	// 目录尚不存在时按数据目录处理（写入时会创建 sessions/）。
	return filepath.Join(dir, "sessions"), nil
}

// readSessionsDir 读取 sessions 目录下的全部会话。
func readSessionsDir(sessionsDir string) ([]Entry, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("session: read sessions dir: %w", err)
	}

	result := make([]Entry, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(sessionsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var sess Entry
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if sess.ID == "" {
			sess.ID = strings.TrimSuffix(entry.Name(), ".json")
		}
		if err := validateID(sess.ID); err != nil {
			continue
		}

		result = append(result, sess)
	}

	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// writeSession 原子地把一个会话写入指定的 sessions 目录。
func writeSession(sessionsDir string, entry Entry) error {
	if err := validateID(entry.ID); err != nil {
		return err
	}
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return fmt.Errorf("session: create sessions dir: %w", err)
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode %s: %w", entry.ID, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(sessionsDir, ".tmp-"+entry.ID+"-*")
	if err != nil {
		return fmt.Errorf("session: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("session: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("session: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("session: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, filepath.Join(sessionsDir, entry.ID+".json")); err != nil {
		return fmt.Errorf("session: rename temp file: %w", err)
	}

	return nil
}
