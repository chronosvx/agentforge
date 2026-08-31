package session

import (
	"context"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/persist"
)

// AsyncStore 包装 Store，把 Save 变为异步落盘，同时保持 Load/Delete 同步语义。
//
// Save 提交后立即返回（除 ID 校验等即时错误外），实际写盘由 internal/persist
// 的后台 worker 串行完成。需要确认数据已落盘时可调用 Flush；后台写盘错误通过
// Errors 通道收集。
type AsyncStore struct {
	store  *Store
	writer *persist.Writer
}

// NewAsyncStore 创建 AsyncStore。
//
// buffer 用于预分配异步队列容量；队列本身不会因提交过快而阻塞 Save。
func NewAsyncStore(store *Store, buffer int) *AsyncStore {
	return &AsyncStore{
		store:  store,
		writer: persist.NewWriter(buffer),
	}
}

// Save 异步保存一个会话。
//
// 与 Store.Save 相同，ID 为空或包含路径分隔符时立即返回错误；其余错误由后台
// worker 产生，可通过 Errors 读取。
func (a *AsyncStore) Save(entry Entry) error {
	if err := validateID(entry.ID); err != nil {
		return err
	}
	entry = cloneEntry(entry)
	return a.writer.Submit(func() error {
		return a.store.Save(entry)
	})
}

// Load 同步读取指定 ID 的会话。
//
// 注意：异步 Save 尚未 Flush 时，Load 可能读不到最新提交的数据。
func (a *AsyncStore) Load(id string) (*Entry, error) {
	return a.store.Load(id)
}

// Delete 同步删除指定 ID 的会话。
//
// 为保证“先 Save 后 Delete”的调用顺序不被异步队列打乱，Delete 会先等待此前
// 已提交的 Save 全部完成，再执行删除。
func (a *AsyncStore) Delete(id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := a.writer.Flush(context.Background()); err != nil {
		return err
	}
	return a.store.Delete(id)
}

// List 同步返回全部已保存的会话。
func (a *AsyncStore) List() ([]Entry, error) {
	return a.store.List()
}

// Flush 等待当前已提交的异步 Save 全部完成。
func (a *AsyncStore) Flush(ctx context.Context) error {
	return a.writer.Flush(ctx)
}

// Close 停止接收新任务，等待队列清空并释放后台资源。
func (a *AsyncStore) Close() error {
	return a.writer.Close()
}

// Errors 返回后台写盘错误的只读通道。
func (a *AsyncStore) Errors() <-chan error {
	return a.writer.Errors()
}

// cloneEntry 对 Entry 做一次深拷贝，避免调用方在 Save 返回后继续修改
// 底层 slice/map 时与后台 worker 产生数据竞争。
func cloneEntry(entry Entry) Entry {
	out := entry
	out.Messages = append([]client.Message(nil), entry.Messages...)
	out.Context = cloneAnyMap(entry.Context)
	out.Cost = cloneAnyMap(entry.Cost)
	out.Diffs = cloneAnySlice(entry.Diffs)
	out.Todos = cloneAnySlice(entry.Todos)
	out.Tools = cloneAnySlice(entry.Tools)
	return out
}

func cloneAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAnySlice(in []any) []any {
	if in == nil {
		return nil
	}
	out := make([]any, len(in))
	for i, v := range in {
		out[i] = cloneAny(v)
	}
	return out
}

func cloneAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return cloneAnyMap(x)
	case []any:
		return cloneAnySlice(x)
	default:
		return v
	}
}
