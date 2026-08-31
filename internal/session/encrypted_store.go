// Package session 的本地加密存储实现。
//
// EncryptedStore 包装 Store，将会话 JSON 使用 internal/security 的
// AES-GCM 加密后写入 <data-dir>/sessions/<id>.json.enc。
// 为兼容旧数据，Load/List 仍可读取原有 <id>.json 明文文件。
package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/chronosvx/agentforge/internal/security"
)

// EncryptedStore 提供与 Store 相同的本地会话 API，但落盘内容经过 AES-GCM 加密。
//
// 新保存的会话写入 <id>.json.enc；已存在的同名 <id>.json 明文文件会在保存后
// 被清理，避免敏感内容继续以明文留在磁盘上。
type EncryptedStore struct {
	store *Store
	key   []byte

	mu sync.Mutex
}

// NewEncryptedStore 创建本地加密会话存储，并确保 <dir>/sessions 目录存在。
func NewEncryptedStore(dir string, key []byte) (*EncryptedStore, error) {
	if dir == "" {
		return nil, errors.New("session: encrypted store: data dir is empty")
	}
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, fmt.Errorf("session: encrypted store: invalid key length %d: must be 16, 24, or 32 bytes", len(key))
	}

	store, err := NewStore(dir)
	if err != nil {
		return nil, err
	}

	return &EncryptedStore{
		store: store,
		key:   append([]byte(nil), key...),
	}, nil
}

// List 返回所有已保存的会话，按 UpdatedAt 倒序排列；时间相同时按 ID 升序。
//
// 同时识别 <id>.json.enc（加密）与旧版 <id>.json（明文）文件；同一 ID 同时存在
// 两种文件时优先使用加密文件。
func (s *EncryptedStore) List() ([]Entry, error) {
	sessionsDir := s.store.sessionsDir()
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, fmt.Errorf("session: list encrypted sessions: %w", err)
	}

	result := make([]Entry, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		var id string
		var data []byte

		switch {
		case strings.HasSuffix(name, ".json.enc"):
			id = strings.TrimSuffix(name, ".json.enc")
			sealed, err := os.ReadFile(filepath.Join(sessionsDir, name))
			if err != nil {
				continue
			}
			plain, err := security.Decrypt(sealed, s.key)
			if err != nil {
				continue
			}
			data = plain
		case strings.HasSuffix(name, ".json"):
			id = strings.TrimSuffix(name, ".json")
			// 同一 ID 已有加密文件时跳过旧明文，避免重复列出。
			if _, err := os.Stat(filepath.Join(sessionsDir, id+".json.enc")); err == nil {
				continue
			}
			data, err = os.ReadFile(filepath.Join(sessionsDir, name))
			if err != nil {
				continue
			}
		default:
			continue
		}

		var sess Entry
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if sess.ID == "" {
			sess.ID = id
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

// Load 读取指定 ID 的会话；优先读取加密文件，不存在时回退到旧版明文文件。
func (s *EncryptedStore) Load(id string) (*Entry, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}

	sessionsDir := s.store.sessionsDir()
	encPath := filepath.Join(sessionsDir, id+".json.enc")
	sealed, err := os.ReadFile(encPath)
	if err == nil {
		plain, err := security.Decrypt(sealed, s.key)
		if err != nil {
			return nil, fmt.Errorf("session: decrypt %s: %w", id, err)
		}
		var entry Entry
		if err := json.Unmarshal(plain, &entry); err != nil {
			return nil, fmt.Errorf("session: decode %s: %w", encPath, err)
		}
		if entry.ID == "" {
			entry.ID = id
		}
		return &entry, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	// 兼容旧数据：没有加密文件时读取明文 JSON。
	plainPath := filepath.Join(sessionsDir, id+".json")
	data, err := os.ReadFile(plainPath)
	if err != nil {
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("session: decode %s: %w", plainPath, err)
	}
	if entry.ID == "" {
		entry.ID = id
	}
	return &entry, nil
}

// Save 原子地加密保存一个会话；ID 为空或包含路径分隔符时返回错误。
//
// 写入文件为 <id>.json.enc。保存成功后，同名的旧版 <id>.json 明文文件会被删除。
func (s *EncryptedStore) Save(entry Entry) error {
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

	sealed, err := security.Encrypt(data, s.key)
	if err != nil {
		return fmt.Errorf("session: encrypt %s: %w", entry.ID, err)
	}

	sessionsDir := s.store.sessionsDir()
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return fmt.Errorf("session: create sessions dir: %w", err)
	}

	tmp, err := os.CreateTemp(sessionsDir, ".tmp-"+entry.ID+"-*")
	if err != nil {
		return fmt.Errorf("session: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(sealed); err != nil {
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

	encPath := filepath.Join(sessionsDir, entry.ID+".json.enc")
	if err := os.Rename(tmpName, encPath); err != nil {
		return fmt.Errorf("session: rename temp file: %w", err)
	}

	// 清理可能存在的旧明文文件，确保密文模式下不残留明文会话。
	plainPath := filepath.Join(sessionsDir, entry.ID+".json")
	if err := os.Remove(plainPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("session: remove plaintext %s: %w", plainPath, err)
	}

	return nil
}

// Delete 删除指定 ID 的会话；同时清理加密文件与旧版明文文件。
func (s *EncryptedStore) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sessionsDir := s.store.sessionsDir()
	encPath := filepath.Join(sessionsDir, id+".json.enc")
	plainPath := filepath.Join(sessionsDir, id+".json")

	encErr := os.Remove(encPath)
	plainErr := os.Remove(plainPath)

	if encErr != nil && !os.IsNotExist(encErr) {
		return fmt.Errorf("session: delete encrypted %s: %w", id, encErr)
	}
	if plainErr != nil && !os.IsNotExist(plainErr) {
		return fmt.Errorf("session: delete plaintext %s: %w", id, plainErr)
	}
	if encErr != nil && plainErr != nil {
		return os.ErrNotExist
	}

	return nil
}
