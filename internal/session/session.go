// Package session 负责会话状态与本地持久化。
//
// 会话以 JSON 文件形式保存在 <data-dir>/sessions/<id>.json 下，
// 写入时使用临时文件加 rename 的方式保证原子性。
package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
)

// Entry 表示一条本地持久化的会话记录。
type Entry struct {
	ID        string           `json:"id"`
	Workspace string           `json:"workspace"`
	Title     string           `json:"title"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
	Messages  []client.Message `json:"messages"`
	Context   map[string]any   `json:"context,omitempty"`
	Cost      map[string]any   `json:"cost,omitempty"`
	Diffs     []any            `json:"diffs,omitempty"`
	Todos     []any            `json:"todos,omitempty"`
	Tools     []any            `json:"tools,omitempty"`
}

// Store 提供会话记录的本地文件存储。
type Store struct {
	dir string

	mu    sync.Mutex
	cache map[string][]byte // 最近一次成功写入的编码结果，用于跳过重复全量写盘。
}

// NewStore 创建会话存储，并确保 <dir>/sessions 目录存在。
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("session: data dir is empty")
	}

	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("session: create sessions dir: %w", err)
	}

	return &Store{dir: dir, cache: make(map[string][]byte)}, nil
}

// List 返回所有已保存的会话，按 UpdatedAt 倒序排列；时间相同时按 ID 升序。
func (s *Store) List() ([]Entry, error) {
	sessionsDir := s.sessionsDir()
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("session: list sessions: %w", err)
	}

	result := make([]Entry, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(sessionsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// 单个坏文件不应拖垮整个列表。
			continue
		}

		var sess Entry
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if sess.ID == "" {
			sess.ID = strings.TrimSuffix(entry.Name(), ".json")
		}

		result = append(result, sess)
	}

	// 最近更新的会话排在前面；时间相同时按 ID 升序，保证顺序确定。
	sort.Slice(result, func(i, j int) bool {
		if !result[i].UpdatedAt.Equal(result[j].UpdatedAt) {
			return result[i].UpdatedAt.After(result[j].UpdatedAt)
		}
		return result[i].ID < result[j].ID
	})

	return result, nil
}

// Load 读取指定 ID 的会话。
func (s *Store) Load(id string) (*Entry, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}

	path := s.path(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("session: decode %s: %w", path, err)
	}
	if entry.ID == "" {
		entry.ID = id
	}

	return &entry, nil
}

// Save 原子地保存一个会话；ID 为空或包含路径分隔符时返回错误。
//
// 对连续保存相同内容的情况，Store 会利用内存缓存跳过重复的全量写盘；
// 文件被外部删除时仍会重新写入，保证语义与之前一致。
func (s *Store) Save(entry Entry) error {
	if err := validateID(entry.ID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("session: encode %s: %w", entry.ID, err)
	}
	data = append(data, '\n')

	// 如果最近一次写入的内容与当前完全一致，且文件仍存在，则无需重复落盘。
	if cached, ok := s.cache[entry.ID]; ok && bytes.Equal(cached, data) {
		if _, err := os.Stat(s.path(entry.ID)); err == nil {
			return nil
		}
	}

	sessionsDir := s.sessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return fmt.Errorf("session: create sessions dir: %w", err)
	}

	tmp, err := os.CreateTemp(sessionsDir, ".tmp-"+entry.ID+"-*")
	if err != nil {
		return fmt.Errorf("session: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 失败时清理；成功后 rename 后文件已不在

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

	if err := os.Rename(tmpName, s.path(entry.ID)); err != nil {
		return fmt.Errorf("session: rename temp file: %w", err)
	}

	if s.cache == nil {
		s.cache = make(map[string][]byte)
	}
	s.cache[entry.ID] = append([]byte(nil), data...)
	return nil
}

// Delete 删除指定 ID 的会话；文件不存在时返回 os.ErrNotExist。
func (s *Store) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path(id)); err != nil {
		return fmt.Errorf("session: delete %s: %w", id, err)
	}
	delete(s.cache, id)
	return nil
}

func (s *Store) sessionsDir() string {
	return filepath.Join(s.dir, "sessions")
}

func (s *Store) path(id string) string {
	return filepath.Join(s.sessionsDir(), id+".json")
}

// validateID 防止会话 ID 被用作路径穿越。
func validateID(id string) error {
	if id == "" {
		return errors.New("session: empty session id")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\:*?"<>|`) {
		return fmt.Errorf("session: invalid session id %q", id)
	}
	return nil
}
