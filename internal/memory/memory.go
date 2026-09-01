// Package memory 提供项目级记忆的本地持久化与简单检索。
//
// 记忆条目以 JSON 数组形式保存在 <data-dir>/memory/<project>.json 下，
// 每个文件对应一个项目的全部记忆条目，写入时使用临时文件加 rename
// 的方式保证原子性。
package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry 表示一条项目记忆。
type Entry struct {
	ID        string    `json:"id"`
	Project   string    `json:"project,omitempty"`
	Key       string    `json:"key"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MemoryStore 提供项目记忆条目的本地文件存储。
type MemoryStore struct {
	dir string

	mu sync.Mutex
}

// NewMemoryStore 创建项目记忆存储，并确保 <dir>/memory 目录存在。
func NewMemoryStore(dir string) (*MemoryStore, error) {
	if dir == "" {
		return nil, errors.New("memory: data dir is empty")
	}
	memoryDir := filepath.Join(dir, "memory")
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		return nil, fmt.Errorf("memory: create memory dir: %w", err)
	}
	return &MemoryStore{dir: dir}, nil
}

// NewStore 是 NewMemoryStore 的别名，便于按存储包惯例调用。
func NewStore(dir string) (*MemoryStore, error) {
	return NewMemoryStore(dir)
}

// Save 保存一条项目记忆。entry.Project 与 entry.Key 不能为空。
//
// 同项目下相同 Key 的条目会被覆盖；UpdatedAt 为零值时自动设置为当前 UTC 时间。
func (s *MemoryStore) Save(ctx context.Context, entry Entry) error {
	if err := validateProject(entry.Project); err != nil {
		return err
	}
	if err := validateKey(entry.Key); err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = entry.Key
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}
	if entry.Tags == nil {
		entry.Tags = []string{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readLocked(entry.Project)
	if err != nil {
		return err
	}

	replaced := false
	for i := range entries {
		if entries[i].Key == entry.Key {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}

	return s.writeLocked(entry.Project, entries)
}

// Get 返回指定项目下某个 Key 的记忆条目；不存在时返回 os.ErrNotExist。
func (s *MemoryStore) Get(ctx context.Context, project, key string) (*Entry, error) {
	if err := validateProject(project); err != nil {
		return nil, err
	}
	if err := validateKey(key); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readLocked(project)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Key == key {
			out := entries[i]
			return &out, nil
		}
	}
	return nil, fmt.Errorf("memory: get %s/%s: %w", project, key, os.ErrNotExist)
}

// List 返回指定项目的全部记忆条目，按 UpdatedAt 倒序、Key 升序排列。
func (s *MemoryStore) List(ctx context.Context, project string) ([]Entry, error) {
	if err := validateProject(project); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readLocked(project)
	if err != nil {
		return nil, err
	}
	sortEntries(entries)
	return entries, nil
}

// Delete 删除指定项目下某个 Key 的记忆条目；不存在时返回 os.ErrNotExist。
func (s *MemoryStore) Delete(ctx context.Context, project, key string) error {
	if err := validateProject(project); err != nil {
		return err
	}
	if err := validateKey(key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.readLocked(project)
	if err != nil {
		return err
	}

	idx := -1
	for i := range entries {
		if entries[i].Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("memory: delete %s/%s: %w", project, key, os.ErrNotExist)
	}

	entries = append(entries[:idx], entries[idx+1:]...)
	return s.writeLocked(project, entries)
}

// Projects 返回已有记忆条目的项目名列表，按字典序排列。
func (s *MemoryStore) Projects(ctx context.Context) ([]string, error) {
	if s == nil || s.dir == "" {
		return []string{}, nil
	}
	memoryDir := filepath.Join(s.dir, "memory")
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("memory: list projects: %w", err)
	}
	projects := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		projects = append(projects, strings.TrimSuffix(entry.Name(), ".json"))
	}
	sort.Strings(projects)
	return projects, nil
}

// Path 返回指定项目记忆文件的路径。
func (s *MemoryStore) Path(project string) string {
	if s == nil || s.dir == "" {
		return ""
	}
	return filepath.Join(s.dir, "memory", project+".json")
}

func (s *MemoryStore) readLocked(project string) ([]Entry, error) {
	path := s.Path(project)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("memory: read %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return []Entry{}, nil
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err == nil {
		if entries == nil {
			entries = []Entry{}
		}
		normalizeProject(entries, project)
		return entries, nil
	}

	// 兼容按 Key 索引的 JSON 对象格式：{"key": Entry, ...}
	var byKey map[string]Entry
	if err := json.Unmarshal(data, &byKey); err == nil {
		entries = make([]Entry, 0, len(byKey))
		for key, entry := range byKey {
			if entry.Key == "" {
				entry.Key = key
			}
			entries = append(entries, entry)
		}
		normalizeProject(entries, project)
		return entries, nil
	}

	// 兼容带 entries 字段的包装格式：{"entries": [Entry, ...]}
	var wrapped struct {
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil {
		if wrapped.Entries == nil {
			wrapped.Entries = []Entry{}
		}
		normalizeProject(wrapped.Entries, project)
		return wrapped.Entries, nil
	}

	return nil, fmt.Errorf("memory: decode %s: unsupported JSON shape", path)
}

func normalizeProject(entries []Entry, project string) {
	for i := range entries {
		if entries[i].Project == "" {
			entries[i].Project = project
		}
	}
}

func (s *MemoryStore) writeLocked(project string, entries []Entry) error {
	path := s.Path(project)
	sortEntries(entries)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("memory: encode %s: %w", path, err)
	}
	data = append(data, '\n')

	memoryDir := filepath.Dir(path)
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		return fmt.Errorf("memory: create memory dir: %w", err)
	}

	tmp, err := os.CreateTemp(memoryDir, ".tmp-"+project+"-*")
	if err != nil {
		return fmt.Errorf("memory: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("memory: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("memory: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("memory: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("memory: rename temp file: %w", err)
	}
	return nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].UpdatedAt.Equal(entries[j].UpdatedAt) {
			return entries[i].UpdatedAt.After(entries[j].UpdatedAt)
		}
		return entries[i].Key < entries[j].Key
	})
}

// validateProject 防止项目名被用作路径穿越。
func validateProject(project string) error {
	if project == "" {
		return errors.New("memory: empty project")
	}
	if project == "." || project == ".." || strings.ContainsAny(project, `/\:*?"<>|`) {
		return fmt.Errorf("memory: invalid project %q", project)
	}
	return nil
}

// validateKey 仅要求 Key 非空；Key 不参与文件路径，因此不限制特殊字符。
func validateKey(key string) error {
	if key == "" {
		return errors.New("memory: empty key")
	}
	return nil
}
