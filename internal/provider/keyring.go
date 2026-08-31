package provider

import (
	"errors"
	"sync"
)

// KeyRing 错误。
var (
	// ErrNoKeys 表示 KeyRing 中还没有任何 Key。
	ErrNoKeys = errors.New("provider: no API keys in key ring")
	// ErrAllKeysFailed 表示 KeyRing 中所有 Key 都已被标记为失败，暂时没有可用 Key。
	ErrAllKeysFailed = errors.New("provider: all API keys are marked failed")
)

// KeyRing 管理同一厂商的多个 API Key，并在线程安全的前提下轮询可用 Key。
//
// 被 MarkFailed 标记的 Key 在 Reset 之前不会被 Next 返回；当所有 Key
// 都不可用时，Next 返回 ErrAllKeysFailed。
type KeyRing struct {
	mu     sync.Mutex
	keys   []string
	failed map[string]bool
	next   int
}

// NewKeyRing 创建一个 KeyRing，可选地传入初始 Key。
func NewKeyRing(keys ...string) *KeyRing {
	r := &KeyRing{failed: make(map[string]bool)}
	for _, key := range keys {
		r.Add(key)
	}
	return r
}

// Add 向 KeyRing 添加一个 Key。重复添加同一 Key 不会产生重复项；
// 空字符串会被忽略。
func (r *KeyRing) Add(key string) {
	if key == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.failed == nil {
		r.failed = make(map[string]bool)
	}
	if r.containsLocked(key) {
		return
	}
	r.keys = append(r.keys, key)
}

// Next 按轮询顺序返回下一个可用 Key。它会跳过已失败的 Key；
// 如果没有可用 Key，则返回对应的错误。
func (r *KeyRing) Next() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.keys) == 0 {
		return "", ErrNoKeys
	}
	if r.failed == nil {
		r.failed = make(map[string]bool)
	}

	for i := 0; i < len(r.keys); i++ {
		idx := (r.next + i) % len(r.keys)
		key := r.keys[idx]
		if !r.failed[key] {
			r.next = (idx + 1) % len(r.keys)
			return key, nil
		}
	}
	return "", ErrAllKeysFailed
}

// MarkFailed 将指定 Key 标记为失败，后续 Next 会跳过它。
// 如果 Key 不在 KeyRing 中，该方法为 no-op。
func (r *KeyRing) MarkFailed(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.containsLocked(key) {
		return
	}
	if r.failed == nil {
		r.failed = make(map[string]bool)
	}
	r.failed[key] = true
}

// Reset 清除指定 Key 的失败标记，使其重新参与轮询。
// 如果 Key 不在 KeyRing 中，该方法为 no-op。
func (r *KeyRing) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.containsLocked(key) {
		return
	}
	delete(r.failed, key)
}

func (r *KeyRing) containsLocked(key string) bool {
	for _, k := range r.keys {
		if k == key {
			return true
		}
	}
	return false
}
