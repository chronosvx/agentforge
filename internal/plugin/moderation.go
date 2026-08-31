// 本文件实现插件市场治理：举报、黑名单与审核状态流转。
package plugin

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ReviewStatus 是插件市场审核状态。
type ReviewStatus string

const (
	// StatusPending 表示插件等待审核。
	StatusPending ReviewStatus = "pending"
	// StatusApproved 表示插件已通过审核。
	StatusApproved ReviewStatus = "approved"
	// StatusRejected 表示插件被拒绝。
	StatusRejected ReviewStatus = "rejected"
	// StatusBlacklisted 表示插件被列入黑名单。
	StatusBlacklisted ReviewStatus = "blacklisted"
)

// Report 是用户对某个插件的一次举报记录。
type Report struct {
	Plugin    string    `json:"plugin"`
	Reason    string    `json:"reason"`
	Reporter  string    `json:"reporter"`
	CreatedAt time.Time `json:"createdAt"`
}

// ModerationStore 维护插件举报记录、黑名单与审核状态。
// 所有方法均为线程安全。
type ModerationStore struct {
	mu       sync.RWMutex
	reports  []Report
	statuses map[string]ReviewStatus
}

// NewModerationStore 返回一个空的插件治理存储。
func NewModerationStore() *ModerationStore {
	return &ModerationStore{
		statuses: make(map[string]ReviewStatus),
	}
}

// AddReport 保存一条举报记录。
// 插件名、举报原因和举报人不能为空；CreatedAt 为零值时自动设置为当前 UTC 时间。
// 首次收到举报时，插件状态自动置为 pending。
func (s *ModerationStore) AddReport(r Report) error {
	if s == nil {
		return errors.New("plugin: moderation store is nil")
	}
	r.Plugin = strings.TrimSpace(r.Plugin)
	r.Reason = strings.TrimSpace(r.Reason)
	r.Reporter = strings.TrimSpace(r.Reporter)
	if r.Plugin == "" {
		return errors.New("plugin: report plugin name is empty")
	}
	if r.Reason == "" {
		return errors.New("plugin: report reason is empty")
	}
	if r.Reporter == "" {
		return errors.New("plugin: report reporter is empty")
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = make(map[string]ReviewStatus)
	}
	s.reports = append(s.reports, r)
	if s.statuses[r.Plugin] == "" {
		s.statuses[r.Plugin] = StatusPending
	}
	return nil
}

// Reports 返回全部举报，或仅返回指定插件的举报。
// plugin 为空时返回所有举报；返回切片为副本，调用方可安全修改。
func (s *ModerationStore) Reports(plugin string) []Report {
	if s == nil {
		return nil
	}
	plugin = strings.TrimSpace(plugin)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Report
	for _, r := range s.reports {
		if plugin == "" || r.Plugin == plugin {
			out = append(out, r)
		}
	}
	return out
}

// Status 返回插件当前审核状态。
// 未显式设置过状态的插件按 pending 处理。
func (s *ModerationStore) Status(name string) ReviewStatus {
	if s == nil {
		return StatusPending
	}
	name = strings.TrimSpace(name)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.statuses[name]; ok {
		return st
	}
	return StatusPending
}

// SetStatus 设置插件的审核状态。
func (s *ModerationStore) SetStatus(name string, status ReviewStatus) error {
	if s == nil {
		return errors.New("plugin: moderation store is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("plugin: plugin name is empty")
	}
	switch status {
	case StatusPending, StatusApproved, StatusRejected, StatusBlacklisted:
	default:
		return fmt.Errorf("plugin: unknown review status %q", status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = make(map[string]ReviewStatus)
	}
	s.statuses[name] = status
	return nil
}

// Blacklist 将插件加入黑名单。
func (s *ModerationStore) Blacklist(name string) error {
	return s.SetStatus(name, StatusBlacklisted)
}

// Unblacklist 将插件移出黑名单并标记为已通过审核。
func (s *ModerationStore) Unblacklist(name string) error {
	if s == nil {
		return errors.New("plugin: moderation store is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("plugin: plugin name is empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = make(map[string]ReviewStatus)
	}
	if s.statuses[name] == StatusBlacklisted {
		s.statuses[name] = StatusApproved
	}
	return nil
}

// IsBlacklisted 报告插件是否在黑名单中。
func (s *ModerationStore) IsBlacklisted(name string) bool {
	return s.Status(name) == StatusBlacklisted
}
