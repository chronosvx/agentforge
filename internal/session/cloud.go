// Package session 的云端同步实现。
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// SyncToRemote 将 dataDir 中的全部本地会话上传到 remote。
// 返回已上传的会话 ID（按字典序排序）。
func SyncToRemote(ctx context.Context, dataDir string, remote RemoteStore) ([]string, error) {
	if dataDir == "" {
		return nil, errors.New("session: sync to remote: data dir is empty")
	}
	if remote == nil {
		return nil, errors.New("session: sync to remote: remote is nil")
	}

	sessionsDir, err := resolveSessionsDir(dataDir)
	if err != nil {
		return nil, err
	}
	entries, err := readSessionsDir(sessionsDir)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("session: sync to remote: encode %s: %w", entry.ID, err)
		}
		if err := remote.Put(ctx, entry.ID, data); err != nil {
			return nil, fmt.Errorf("session: sync to remote: put %s: %w", entry.ID, err)
		}
		ids = append(ids, entry.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

// SyncFromRemote 将 remote 中的全部会话下载到 dataDir。
// 已存在且本地 UpdatedAt 比远程更新的会话会被跳过。
// 返回实际下载的会话 ID（按字典序排序）。
func SyncFromRemote(ctx context.Context, dataDir string, remote RemoteStore) ([]string, error) {
	if dataDir == "" {
		return nil, errors.New("session: sync from remote: data dir is empty")
	}
	if remote == nil {
		return nil, errors.New("session: sync from remote: remote is nil")
	}

	sessionsDir, err := resolveSessionsDir(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return nil, fmt.Errorf("session: sync from remote: create sessions dir: %w", err)
	}

	ids, err := remote.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("session: sync from remote: list: %w", err)
	}
	sort.Strings(ids)

	downloaded := make([]string, 0, len(ids))
	for _, id := range ids {
		if err := validateID(id); err != nil {
			return nil, fmt.Errorf("session: sync from remote: invalid remote id %q: %w", id, err)
		}
		data, err := remote.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("session: sync from remote: get %s: %w", id, err)
		}

		var entry Entry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("session: sync from remote: decode %s: %w", id, err)
		}
		if entry.ID == "" {
			entry.ID = id
		}
		if err := validateID(entry.ID); err != nil {
			return nil, fmt.Errorf("session: sync from remote: invalid session id %q: %w", entry.ID, err)
		}

		existing, err := loadSessionFile(sessionsDir, entry.ID)
		if err != nil {
			return nil, fmt.Errorf("session: sync from remote: load local %s: %w", entry.ID, err)
		}
		// 本地不旧于远程时跳过：本地较新或时间相同都无需覆盖。
		if existing != nil && !existing.UpdatedAt.Before(entry.UpdatedAt) {
			continue
		}

		if err := writeSession(sessionsDir, entry); err != nil {
			return nil, fmt.Errorf("session: sync from remote: write %s: %w", entry.ID, err)
		}
		downloaded = append(downloaded, entry.ID)
	}

	return downloaded, nil
}

// loadSessionFile 读取 sessions 目录中的单个会话；不存在时返回 (nil, nil)。
func loadSessionFile(sessionsDir, id string) (*Entry, error) {
	data, err := os.ReadFile(filepath.Join(sessionsDir, id+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	if entry.ID == "" {
		entry.ID = id
	}
	return &entry, nil
}
