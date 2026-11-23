// Package collab 提供会话协作数据的本地持久化：评论、审批记录与会话回放事件。
//
// 所有数据按会话 ID 保存在 <data-dir>/collab/<session>.json 中，文件内容为
// JSON 对象，包含 comments、approvals 与 replayEvents 三个数组。写入使用
// 临时文件加 rename 的方式保证原子性。
package collab

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

// Comment 表示一条会话评论。
type Comment struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	Author    string    `json:"author"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"createdAt"`
}

// Approval 表示一次动作审批记录。
type Approval struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionId"`
	ActionID  string    `json:"actionId"`
	Approver  string    `json:"approver"`
	Decision  string    `json:"decision"`
	CreatedAt time.Time `json:"createdAt"`
}

// ReplayEvent 表示一次可回放的会话操作事件。
type ReplayEvent struct {
	Seq       int       `json:"seq"`
	Type      string    `json:"type"`
	Data      any       `json:"data,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// CollabStore 提供评论、审批与回放事件的本地文件存储。
type CollabStore struct {
	dir string

	mu sync.Mutex
}

// NewStore 创建协作存储，并确保 <dir>/collab 目录存在。
func NewStore(dir string) (*CollabStore, error) {
	if dir == "" {
		return nil, errors.New("collab: data dir is empty")
	}

	collabDir := filepath.Join(dir, "collab")
	if err := os.MkdirAll(collabDir, 0o700); err != nil {
		return nil, fmt.Errorf("collab: create collab dir: %w", err)
	}

	return &CollabStore{dir: dir}, nil
}

// NewCollabStore 是 NewStore 的别名，便于按类型名直接识别协作存储。
func NewCollabStore(dir string) (*CollabStore, error) {
	return NewStore(dir)
}

// AddComment 保存一条评论。
func (s *CollabStore) AddComment(comment Comment) error {
	if err := validateSessionID(comment.SessionID); err != nil {
		return err
	}

	return s.update(comment.SessionID, func(d *sessionData) {
		d.Comments = append(d.Comments, comment)
	})
}

// AddApproval 保存一条审批记录。
func (s *CollabStore) AddApproval(approval Approval) error {
	if err := validateSessionID(approval.SessionID); err != nil {
		return err
	}

	return s.update(approval.SessionID, func(d *sessionData) {
		d.Approvals = append(d.Approvals, approval)
	})
}

// AddReplayEvent 保存一条回放事件。
func (s *CollabStore) AddReplayEvent(sessionID string, event ReplayEvent) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}

	return s.update(sessionID, func(d *sessionData) {
		d.ReplayEvents = append(d.ReplayEvents, event)
	})
}

// ListComments 返回指定会话的全部评论，按添加顺序排列。
func (s *CollabStore) ListComments(sessionID string) ([]Comment, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return nil, err
	}
	comments := data.Comments
	if comments == nil {
		comments = []Comment{}
	}
	return append([]Comment(nil), comments...), nil
}

// ListApprovals 返回指定会话的全部审批记录，按添加顺序排列。
func (s *CollabStore) ListApprovals(sessionID string) ([]Approval, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return nil, err
	}
	approvals := data.Approvals
	if approvals == nil {
		approvals = []Approval{}
	}
	return append([]Approval(nil), approvals...), nil
}

// Replay 返回指定会话的回放事件，按 Seq 升序排列。
//
// 相同 Seq 的事件保持写入顺序，便于调用方得到稳定的回放顺序。
func (s *CollabStore) Replay(sessionID string) ([]ReplayEvent, error) {
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return nil, err
	}

	events := data.ReplayEvents
	if events == nil {
		events = []ReplayEvent{}
	}
	events = append([]ReplayEvent(nil), events...)
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Seq < events[j].Seq
	})
	return events, nil
}

// sessionData 是单个会话协作文件的磁盘格式。
type sessionData struct {
	Comments     []Comment     `json:"comments"`
	Approvals    []Approval    `json:"approvals"`
	ReplayEvents []ReplayEvent `json:"replayEvents"`
}

// update 在锁内执行读-改-写。
func (s *CollabStore) update(sessionID string, mutate func(*sessionData)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked(sessionID)
	if err != nil {
		return err
	}
	mutate(data)
	return s.saveLocked(sessionID, data)
}

// loadLocked 读取指定会话的协作数据；文件不存在时返回空数据。
func (s *CollabStore) loadLocked(sessionID string) (*sessionData, error) {
	data, err := os.ReadFile(s.path(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return &sessionData{}, nil
		}
		return nil, fmt.Errorf("collab: read %s: %w", s.path(sessionID), err)
	}

	var d sessionData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("collab: decode %s: %w", s.path(sessionID), err)
	}
	return &d, nil
}

// saveLocked 原子地写入指定会话的协作数据。
func (s *CollabStore) saveLocked(sessionID string, d *sessionData) error {
	collabDir := filepath.Join(s.dir, "collab")
	if err := os.MkdirAll(collabDir, 0o700); err != nil {
		return fmt.Errorf("collab: create collab dir: %w", err)
	}

	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("collab: encode %s: %w", sessionID, err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(collabDir, ".tmp-"+sessionID+"-*")
	if err != nil {
		return fmt.Errorf("collab: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // 失败时清理；成功后 rename 后文件已不在

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("collab: write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("collab: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("collab: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, s.path(sessionID)); err != nil {
		return fmt.Errorf("collab: rename temp file: %w", err)
	}
	return nil
}

func (s *CollabStore) path(sessionID string) string {
	return filepath.Join(s.dir, "collab", sessionID+".json")
}

// validateSessionID 防止会话 ID 被用作路径穿越。
func validateSessionID(id string) error {
	if id == "" {
		return errors.New("collab: empty session id")
	}
	if id == "." || id == ".." || strings.ContainsAny(id, `/\:*?"<>|`) {
		return fmt.Errorf("collab: invalid session id %q", id)
	}
	return nil
}
