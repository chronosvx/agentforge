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

// ExportAll 将 dir 中的全部会话导出为 JSON 文件到 outDir。
// 返回每个导出文件的路径。
func ExportAll(dir, outDir string) ([]string, error) {
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
	entries, err := readSessionsDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, fmt.Errorf("session: create export dir: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := validateID(entry.ID); err != nil {
			return nil, fmt.Errorf("session: export invalid id %q: %w", entry.ID, err)
		}
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("session: encode %s: %w", entry.ID, err)
		}
		data = append(data, '\n')

		path := filepath.Join(outDir, entry.ID+".json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, fmt.Errorf("session: write export %s: %w", path, err)
		}
		paths = append(paths, path)
	}

	return paths, nil
}

// ImportAll 从 inDir 导入 JSON 会话到 dataDir 的 sessions/ 目录。
// 已存在的会话 ID 不会被覆盖，而是记录为跳过；返回每个文件的处理结果。
func ImportAll(inDir, dataDir string) ([]string, error) {
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

	results := make([]string, 0)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		name := file.Name()
		path := filepath.Join(inDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			results = append(results, fmt.Sprintf("skip %s: %v", name, err))
			continue
		}

		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			results = append(results, fmt.Sprintf("skip %s: invalid JSON", name))
			continue
		}
		if entry.ID == "" {
			entry.ID = strings.TrimSuffix(name, ".json")
		}
		if err := validateID(entry.ID); err != nil {
			results = append(results, fmt.Sprintf("skip %s: invalid id %q", name, entry.ID))
			continue
		}

		target := filepath.Join(sessionsDir, entry.ID+".json")
		if _, err := os.Stat(target); err == nil {
			results = append(results, fmt.Sprintf("skip %s: %s already exists", name, entry.ID))
			continue
		}

		if err := writeSession(sessionsDir, entry); err != nil {
			return nil, fmt.Errorf("session: import %s: %w", entry.ID, err)
		}
		results = append(results, fmt.Sprintf("imported %s", entry.ID))
	}

	return results, nil
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
