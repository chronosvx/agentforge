// Package persist 提供本地异步写入的通用队列。
//
// Writer 将耗时的持久化操作放到后台 goroutine 串行执行，调用方提交后
// 无需等待写盘完成；通过 Flush 可以显式等待队列清空，通过 Errors 可以
// 收集后台执行过程中产生的错误。
package persist

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Writer 是一个线程安全的异步任务队列。
//
// Submit 会立即返回（除非 Writer 已关闭），后台 worker 按提交顺序串行执行
// 任务。Flush 等待当前已提交任务全部完成，Close 停止接收新任务并等待队列
// 清空。
type Writer struct {
	mu    sync.Mutex
	cond  *sync.Cond
	queue []func() error
	// pending 是尚未完成的任务数（包括排队中和正在执行的）。
	pending int
	closed  bool

	workerDone chan struct{}

	errMu     sync.Mutex
	errCond   *sync.Cond
	errs      []error
	errClosed bool
	errCh     chan error
}

// NewWriter 创建一个异步 Writer。
//
// buffer 用于预分配任务队列容量，不影响 Submit 的即时返回语义；队列本身是
// 无界队列，提交不会被缓冲区占满所阻塞。
func NewWriter(buffer int) *Writer {
	if buffer < 0 {
		buffer = 0
	}
	w := &Writer{
		queue:      make([]func() error, 0, buffer),
		workerDone: make(chan struct{}),
		errCh:      make(chan error),
	}
	w.cond = sync.NewCond(&w.mu)
	w.errCond = sync.NewCond(&w.errMu)
	go w.loop()
	go w.forwardErrors()
	return w
}

// Submit 将 save 加入后台队列并立即返回。
//
// 如果 Writer 已经 Close，返回错误；任务执行过程中的错误不会在此返回，而是
// 通过 Errors 通道收集。
func (w *Writer) Submit(save func() error) error {
	if save == nil {
		return errors.New("persist: nil save function")
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errors.New("persist: writer is closed")
	}
	w.queue = append(w.queue, save)
	w.pending++
	w.mu.Unlock()
	w.cond.Signal()
	return nil
}

// Flush 阻塞直到当前已提交的任务全部执行完成。
//
// ctx 被取消时立即返回 ctx.Err()。
func (w *Writer) Flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// ctx 取消时唤醒 cond.Wait，使 Flush 能及时返回。
	stop := context.AfterFunc(ctx, func() {
		w.mu.Lock()
		w.cond.Broadcast()
		w.mu.Unlock()
	})
	defer stop()

	w.mu.Lock()
	defer w.mu.Unlock()
	for w.pending > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		w.cond.Wait()
	}
	return nil
}

// Close 停止接收新任务，等待队列清空后返回。
//
// Close 不会关闭 Errors 通道本身；错误转发 goroutine 会在所有已收集错误被
// 读取后关闭该通道。
func (w *Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		<-w.workerDone
		return nil
	}
	w.closed = true
	w.mu.Unlock()
	w.cond.Broadcast()

	<-w.workerDone

	w.errMu.Lock()
	w.errClosed = true
	w.errMu.Unlock()
	w.errCond.Broadcast()
	return nil
}

// Errors 返回后台任务执行错误的只读通道。
//
// 通道会在 Writer 关闭且已收集错误全部转发后关闭。调用方应持续读取或稍后
// range 该通道，避免错误转发 goroutine 因无人接收而阻塞。
func (w *Writer) Errors() <-chan error {
	return w.errCh
}

func (w *Writer) loop() {
	defer close(w.workerDone)

	for {
		w.mu.Lock()
		for len(w.queue) == 0 && !w.closed {
			w.cond.Wait()
		}
		if len(w.queue) == 0 && w.closed {
			w.mu.Unlock()
			return
		}

		save := w.queue[0]
		w.queue[0] = nil
		w.queue = w.queue[1:]
		w.mu.Unlock()

		if err := runSave(save); err != nil {
			w.addError(err)
		}

		w.mu.Lock()
		w.pending--
		w.mu.Unlock()
		w.cond.Broadcast()
	}
}

func (w *Writer) addError(err error) {
	w.errMu.Lock()
	w.errs = append(w.errs, err)
	w.errMu.Unlock()
	w.errCond.Signal()
}

func (w *Writer) forwardErrors() {
	for {
		w.errMu.Lock()
		for len(w.errs) == 0 && !w.errClosed {
			w.errCond.Wait()
		}
		if len(w.errs) == 0 && w.errClosed {
			w.errMu.Unlock()
			close(w.errCh)
			return
		}

		err := w.errs[0]
		w.errs[0] = nil
		w.errs = w.errs[1:]
		w.errMu.Unlock()

		// 如果没有读者，这个 goroutine 会阻塞在发送上；Close 不等待它，
		// 因此后台写盘不会因此卡住。调用方应消费 Errors。
		w.errCh <- err
	}
}

// runSave 执行一个持久化函数，并把 panic 转换为 error，避免 worker 因单个
// 任务崩溃而退出。
func runSave(save func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("persist: task panic: %v", r)
		}
	}()
	return save()
}
