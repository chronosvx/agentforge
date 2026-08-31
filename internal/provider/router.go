package provider

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
)

// ErrNoProvider 表示当前没有可用厂商。
var ErrNoProvider = errors.New("provider: no provider available")

// ProviderWithPriority 描述一个厂商及其路由元数据。
type ProviderWithPriority struct {
	Provider Provider
	Priority int
	Weight   int
}

type providerState struct {
	ProviderWithPriority
	failed      bool
	failCount   int
	failedUntil time.Time
}

// Router 按优先级和健康状态选择厂商。
//
// 策略：
//   - 默认按 Priority 从高到低选择；
//   - 同一优先级内按轮询（round-robin）选择；
//   - MarkFailed 标记的厂商会被跳过，经过 retryAfter 后自动恢复；
//   - Reset 可手动恢复厂商。
type Router struct {
	mu          sync.Mutex
	providers   []*providerState
	rr          map[int]int
	retryAfter  time.Duration
	now         func() time.Time
	initialized bool
}

// NewRouter 创建一个默认 30 秒冷却期的 Router。
func NewRouter() *Router {
	r := &Router{}
	r.initLocked()
	return r
}

func (r *Router) initLocked() {
	if r.rr == nil {
		r.rr = make(map[int]int)
	}
	if r.now == nil {
		r.now = time.Now
	}
	if !r.initialized {
		r.retryAfter = 30 * time.Second
		r.initialized = true
	}
}

// Add 添加一个厂商，优先级越高越先被选择，权重默认为 1。
func (r *Router) Add(p Provider, priority int) {
	r.AddWeighted(p, priority, 1)
}

// AddWeighted 添加一个厂商并指定权重。权重当前作为元数据保留，同优先级仍按轮询选择。
func (r *Router) AddWeighted(p Provider, priority, weight int) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	if weight <= 0 {
		weight = 1
	}
	r.providers = append(r.providers, &providerState{
		ProviderWithPriority: ProviderWithPriority{
			Provider: p,
			Priority: priority,
			Weight:   weight,
		},
	})
}

// SetRetryAfter 设置失败后的冷却时间。<=0 表示失败后立即恢复（通常仅用于测试）。
func (r *Router) SetRetryAfter(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	r.retryAfter = d
}

// Providers 返回当前已注册厂商的快照。
func (r *Router) Providers() []ProviderWithPriority {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	out := make([]ProviderWithPriority, 0, len(r.providers))
	for _, s := range r.providers {
		out = append(out, s.ProviderWithPriority)
	}
	return out
}

// Next 返回下一个可用厂商。没有可用厂商时返回 ErrNoProvider。
func (r *Router) Next(ctx context.Context) (Provider, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()

	now := r.now()
	r.expireLocked(now)

	if len(r.providers) == 0 {
		return nil, ErrNoProvider
	}

	var best []*providerState
	for _, s := range r.providers {
		if s.failed {
			continue
		}
		if len(best) == 0 || s.Priority > best[0].Priority {
			best = []*providerState{s}
		} else if s.Priority == best[0].Priority {
			best = append(best, s)
		}
	}
	if len(best) == 0 {
		return nil, ErrNoProvider
	}

	priority := best[0].Priority
	idx := r.rr[priority] % len(best)
	r.rr[priority] = idx + 1
	return best[idx].Provider, nil
}

// MarkFailed 标记厂商失败。之后 Next 会跳过它，直到冷却结束或被 Reset。
func (r *Router) MarkFailed(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()

	s := r.findLocked(p)
	if s == nil {
		return
	}
	s.failed = true
	s.failCount++
	s.failedUntil = r.now().Add(r.retryAfter)
}

// Reset 手动恢复一个厂商，并清空失败计数。
func (r *Router) Reset(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()

	s := r.findLocked(p)
	if s == nil {
		return
	}
	s.failed = false
	s.failCount = 0
	s.failedUntil = time.Time{}
}

func (r *Router) expireLocked(now time.Time) {
	for _, s := range r.providers {
		if s.failed && !now.Before(s.failedUntil) {
			s.failed = false
			s.failCount = 0
			s.failedUntil = time.Time{}
		}
	}
}

func (r *Router) findLocked(p Provider) *providerState {
	if p == nil {
		return nil
	}
	for _, s := range r.providers {
		if reflect.DeepEqual(s.Provider, p) {
			return s
		}
	}
	return nil
}
