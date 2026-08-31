package contextx

import (
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

// ErrCheckpointNotFound 表示指定 ID 的 checkpoint 不存在。
var ErrCheckpointNotFound = errors.New("contextx: checkpoint not found")

// Checkpoint 是某一时刻窗口的不可变快照。
//
// 可保存在内存中，也可由 CheckpointStore 写入本地 JSON 文件。
type Checkpoint struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"createdAt"`
	Messages   []Message `json:"messages"`
	TokenCount int64     `json:"tokenCount"`
	Note       string    `json:"note,omitempty"`
}

// CheckpointStore 提供 Checkpoint 的内存存储，并可选地同步到本地文件。
//
// 默认 NewCheckpointStore 只保留内存；NewFileCheckpointStore 会额外把
// checkpoint 写入 <dir>/<id>.json，用于进程重启后恢复。
type CheckpointStore struct {
	mu     sync.RWMutex
	dir    string
	memory map[string]Checkpoint
}

// NewCheckpointStore 返回仅使用内存的 checkpoint 存储。
func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{memory: make(map[string]Checkpoint)}
}

// NewFileCheckpointStore 返回内存 + 文件双写 checkpoint 存储。
//
// dir 会被创建（权限 0700），checkpoint 文件权限为 0600。
func NewFileCheckpointStore(dir string) (*CheckpointStore, error) {
	if dir == "" {
		return nil, errors.New("contextx: checkpoint dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("contextx: create checkpoint dir: %w", err)
	}
	return &CheckpointStore{
		dir:    dir,
		memory: make(map[string]Checkpoint),
	}, nil
}

// Save 保存 checkpoint 到内存；若配置了文件目录，同时原子写入本地文件。
func (s *CheckpointStore) Save(cp Checkpoint) error {
	if err := validateCheckpointID(cp.ID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.memory == nil {
		s.memory = make(map[string]Checkpoint)
	}
	s.memory[cp.ID] = cloneCheckpoint(cp)

	if s.dir == "" {
		return nil
	}
	return s.writeFileLocked(cp)
}

// Load 读取指定 ID 的 checkpoint。
//
// 优先返回内存副本；未命中且配置了文件目录时尝试从文件加载，并回填内存缓存。
func (s *CheckpointStore) Load(id string) (Checkpoint, error) {
	if err := validateCheckpointID(id); err != nil {
		return Checkpoint{}, err
	}

	s.mu.RLock()
	cp, ok := s.memory[id]
	s.mu.RUnlock()
	if ok {
		return cloneCheckpoint(cp), nil
	}

	if s.dir == "" {
		return Checkpoint{}, ErrCheckpointNotFound
	}

	path := s.path(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Checkpoint{}, ErrCheckpointNotFound
		}
		return Checkpoint{}, fmt.Errorf("contextx: read checkpoint %s: %w", path, err)
	}

	var loaded Checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Checkpoint{}, fmt.Errorf("contextx: decode checkpoint %s: %w", path, err)
	}
	if loaded.ID == "" {
		loaded.ID = id
	}

	s.mu.Lock()
	if s.memory == nil {
		s.memory = make(map[string]Checkpoint)
	}
	s.memory[id] = cloneCheckpoint(loaded)
	s.mu.Unlock()

	return loaded, nil
}

// Delete 删除指定 ID 的 checkpoint；同时清理内存与可选文件。
func (s *CheckpointStore) Delete(id string) error {
	if err := validateCheckpointID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, inMemory := s.memory[id]
	delete(s.memory, id)

	if s.dir == "" {
		if !inMemory {
			return ErrCheckpointNotFound
		}
		return nil
	}

	err := os.Remove(s.path(id))
	if err != nil {
		if os.IsNotExist(err) {
			if !inMemory {
				return ErrCheckpointNotFound
			}
			return nil
		}
		return fmt.Errorf("contextx: delete checkpoint %s: %w", id, err)
	}
	return nil
}

// List 返回所有 checkpoint，按 CreatedAt 倒序排列；时间相同时按 ID 升序。
//
// 文件存储会合并目录中的 checkpoint 文件；内存中尚未落盘的 checkpoint 也会包含。
func (s *CheckpointStore) List() ([]Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := make(map[string]Checkpoint)
	for id, cp := range s.memory {
		seen[id] = cloneCheckpoint(cp)
	}

	if s.dir != "" {
		entries, err := os.ReadDir(s.dir)
		if err != nil {
			return nil, fmt.Errorf("contextx: list checkpoint dir: %w", err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			path := filepath.Join(s.dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var cp Checkpoint
			if err := json.Unmarshal(data, &cp); err != nil {
				continue
			}
			if cp.ID == "" {
				cp.ID = id
			}
			// 内存中的同 ID 数据优先（可能比磁盘更新）。
			if _, ok := seen[id]; !ok {
				seen[id] = cp
			}
		}
	}

	result := make([]Checkpoint, 0, len(seen))
	for _, cp := range seen {
		result = append(result, cp)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

// writeFileLocked 将 checkpoint 原子写入本地 JSON 文件。调用方需持有锁。
func (s *CheckpointStore) writeFileLocked(cp Checkpoint) error {
	if s.dir == "" {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("contextx: create checkpoint dir: %w", err)
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("contextx: encode checkpoint %s: %w", cp.ID, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, ".checkpoint-"+cp.ID+"-*")
	if err != nil {
		return fmt.Errorf("contextx: create temp checkpoint: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("contextx: write temp checkpoint: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("contextx: sync temp checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("contextx: close temp checkpoint: %w", err)
	}
	if err := os.Rename(tmpName, s.path(cp.ID)); err != nil {
		return fmt.Errorf("contextx: rename temp checkpoint: %w", err)
	}
	return nil
}

func (s *CheckpointStore) path(id string) string {
	return filepath.Join(s.dir, id+".json")
}

func validateCheckpointID(id string) error {
	if id == "" {
		return errors.New("contextx: empty checkpoint id")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\:*?"<>|`) {
		return fmt.Errorf("contextx: invalid checkpoint id %q", id)
	}
	return nil
}

func cloneCheckpoint(cp Checkpoint) Checkpoint {
	cp.Messages = append([]Message(nil), cp.Messages...)
	return cp
}
