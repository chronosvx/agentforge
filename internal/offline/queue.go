// Package offline 提供基于本地 JSON 文件的离线消息队列。
//
// 当直连模型厂商暂时不可用时，上层可以把用户消息暂存到队列中，
// 网络恢复后再逐条取出并发送。
package offline

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Item 是队列中的一条待发送消息。
type Item struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Message   string    `json:"message"`
	Replay    bool      `json:"replay"`
	CreatedAt time.Time `json:"created_at"`
}

// queueFile 是队列持久化文件的根结构。
type queueFile struct {
	Items []Item `json:"items"`
}

// Queue 是基于单个 JSON 文件的线程安全消息队列。
type Queue struct {
	path string
	mu   sync.Mutex
}

// NewQueue 创建一个写入 path 的队列。path 为 JSON 文件路径。
func NewQueue(path string) *Queue {
	return &Queue{path: path}
}

// Enqueue 追加一条消息到队列末尾。Replay 默认为 false。
func (q *Queue) Enqueue(sessionID, message string) error {
	return q.enqueue(sessionID, message, false)
}

// EnqueueReplay 追加一条已存在于内存会话中的重放消息。
// Replay 为 true 时，FlushOffline 不会再次把该消息追加到会话。
func (q *Queue) EnqueueReplay(sessionID, message string) error {
	return q.enqueue(sessionID, message, true)
}

func (q *Queue) enqueue(sessionID, message string, replay bool) error {
	if q == nil {
		return fmt.Errorf("offline: nil queue")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.readLocked()
	if err != nil {
		return err
	}
	items = append(items, Item{
		ID:        newID(),
		SessionID: sessionID,
		Message:   message,
		Replay:    replay,
		CreatedAt: time.Now().UTC(),
	})
	return q.writeLocked(items)
}

// Pending 返回当前队列中全部待发送消息，按入队顺序排列。
func (q *Queue) Pending() ([]Item, error) {
	if q == nil {
		return nil, fmt.Errorf("offline: nil queue")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.readLocked()
	if err != nil {
		return nil, err
	}
	if items == nil {
		return []Item{}, nil
	}
	return items, nil
}

// Remove 从队列中移除指定 ID 的消息。ID 不存在时视为已移除，返回 nil。
func (q *Queue) Remove(id string) error {
	if q == nil {
		return fmt.Errorf("offline: nil queue")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	items, err := q.readLocked()
	if err != nil {
		return err
	}

	out := make([]Item, 0, len(items))
	removed := false
	for _, item := range items {
		if item.ID == id {
			removed = true
			continue
		}
		out = append(out, item)
	}
	if !removed {
		return nil
	}
	return q.writeLocked(out)
}

// readLocked 读取队列文件；文件不存在时返回空列表。
func (q *Queue) readLocked() ([]Item, error) {
	data, err := os.ReadFile(q.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("offline: read queue: %w", err)
	}

	var file queueFile
	if err := json.Unmarshal(data, &file); err != nil {
		// 兼容直接以 JSON 数组作为队列文件的旧格式/手写格式。
		var items []Item
		if arrayErr := json.Unmarshal(data, &items); arrayErr != nil {
			return nil, fmt.Errorf("offline: decode queue: %w", err)
		}
		if items == nil {
			return []Item{}, nil
		}
		return items, nil
	}
	if file.Items == nil {
		return []Item{}, nil
	}
	return file.Items, nil
}

// writeLocked 原子写入队列文件。
func (q *Queue) writeLocked(items []Item) error {
	if items == nil {
		items = []Item{}
	}

	data, err := json.MarshalIndent(queueFile{Items: items}, "", "  ")
	if err != nil {
		return fmt.Errorf("offline: encode queue: %w", err)
	}

	dir := filepath.Dir(q.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("offline: create queue dir: %w", err)
		}
	}

	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("offline: write queue temp file: %w", err)
	}
	if err := os.Rename(tmp, q.path); err != nil {
		return fmt.Errorf("offline: replace queue file: %w", err)
	}
	return nil
}

// newID 生成一个随机十六进制 ID。
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源不可用时退化为时间戳，保证队列仍可用。
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
