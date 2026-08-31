package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"unicode/utf8"
)

// 断点续传相关常量。
const (
	// ResumeOffsetHeader 是用于向服务端传递续传偏移量的 HTTP header。
	ResumeOffsetHeader = "X-Resume-Offset"
	// ResumeOffsetQuery 是部分服务端支持的查询参数名。
	ResumeOffsetQuery = "resume_offset"
)

// ErrResumeUnsupported 表示当前 Provider 或服务端不支持断点续传。
var ErrResumeUnsupported = errors.New("provider: resume unsupported")

// streamResumeProvider 是支持断点续传的 Provider 可选接口。
type streamResumeProvider interface {
	// ResumeStream 从 lastOffset 处继续流式请求。
	ResumeStream(ctx context.Context, req ChatRequest, lastOffset int) (<-chan StreamEvent, error)
}

// StreamResumer 记录流式响应中已收到的 SSE 事件索引与文本字符偏移。
//
// 它本身不直接发起网络请求；调用方在消费 StreamChat/ResumeStream 返回的
// 事件时调用 Record，断线后可把 Offset/EventIndex 作为 lastOffset 传给 Resume。
type StreamResumer struct {
	provider Provider

	mu         sync.Mutex
	eventIndex int
	charOffset int
}

// NewStreamResumer 创建一个绑定 Provider 的 StreamResumer。
func NewStreamResumer(p Provider) *StreamResumer {
	return &StreamResumer{provider: p}
}

// Record 记录一个已收到的流式事件。
// eventIndex 每次调用 +1；charOffset 仅累加文本事件的字符数（Unicode 码点）。
func (r *StreamResumer) Record(ev StreamEvent) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventIndex++
	if ev.Type == StreamEventText {
		r.charOffset += utf8.RuneCountInString(ev.Text)
	}
}

// EventIndex 返回已记录的事件数量。
func (r *StreamResumer) EventIndex() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eventIndex
}

// CharOffset 返回已记录的文本字符偏移量。
func (r *StreamResumer) CharOffset() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.charOffset
}

// Offset 是 CharOffset 的别名，便于调用方统一使用 Offset 作为断点。
func (r *StreamResumer) Offset() int {
	return r.CharOffset()
}

// Reset 清空已记录的事件索引与字符偏移。
func (r *StreamResumer) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventIndex = 0
	r.charOffset = 0
}

// Resume 从 lastOffset 处继续流式请求。
//
// 只有底层 Provider 实现了 streamResumeProvider 时才继续；否则返回
// ErrResumeUnsupported。OpenAI 兼容客户端通过 X-Resume-Offset header
// 向服务端传递偏移量，服务端不支持时由上层按实际错误处理。
func (r *StreamResumer) Resume(ctx context.Context, req ChatRequest, lastOffset int) (<-chan StreamEvent, error) {
	if r == nil || r.provider == nil {
		return nil, ErrResumeUnsupported
	}
	rp, ok := r.provider.(streamResumeProvider)
	if !ok {
		return nil, fmt.Errorf("%w: provider %T does not implement stream resume", ErrResumeUnsupported, r.provider)
	}
	if lastOffset < 0 {
		lastOffset = 0
	}
	req = WithResumeOffset(req, lastOffset)
	return rp.ResumeStream(ctx, req, lastOffset)
}

// WithResumeOffset 把续传偏移量写入 ChatRequest 的 Headers。
func WithResumeOffset(req ChatRequest, offset int) ChatRequest {
	headers := make(map[string]string, len(req.Headers)+1)
	for k, v := range req.Headers {
		headers[k] = v
	}
	headers[ResumeOffsetHeader] = strconv.Itoa(offset)
	req.Headers = headers
	return req
}
