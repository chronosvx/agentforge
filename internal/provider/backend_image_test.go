package provider

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/cost"
)

type imageRecordingProvider struct {
	mu   sync.Mutex
	reqs []ChatRequest
}

func (p *imageRecordingProvider) Name() string                   { return "fake-image" }
func (p *imageRecordingProvider) Model() string                  { return "fake-model" }
func (p *imageRecordingProvider) CostEstimator() *cost.Estimator { return cost.NewEstimator() }

func (p *imageRecordingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return &ChatResponse{}, nil
}

func (p *imageRecordingProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan StreamEvent, error) {
	p.mu.Lock()
	p.reqs = append(p.reqs, req)
	p.mu.Unlock()

	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: StreamEventDone}
	close(ch)
	return ch, nil
}

func (p *imageRecordingProvider) lastRequest() (ChatRequest, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.reqs) == 0 {
		return ChatRequest{}, false
	}
	return p.reqs[len(p.reqs)-1], true
}

func writeTestImage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.png")
	if err := os.WriteFile(path, []byte("fake-image-bytes"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return path
}

func waitForImageRequest(t *testing.T, p *imageRecordingProvider) ChatRequest {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if req, ok := p.lastRequest(); ok {
			return req
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for StreamChat request")
	return ChatRequest{}
}

func TestBackendInvokeImagePathsAttachToLastUserMessage(t *testing.T) {
	path := writeTestImage(t)
	p := &imageRecordingProvider{}
	b := NewBackend(p, nil)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"describe this", []string{path}}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	req := waitForImageRequest(t, p)
	if len(req.Messages) != 1 {
		t.Fatalf("len(req.Messages) = %d, want 1", len(req.Messages))
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != "user" || last.Content != "describe this" {
		t.Fatalf("last message = %#v, want user describe this", last)
	}
	if len(last.Images) != 1 {
		t.Fatalf("last.Images = %#v, want one image", last.Images)
	}
	if last.Images[0].DataURL == "" || last.Images[0].MediaType != "image/png" {
		t.Fatalf("last.Images[0] = %#v, want png data URL", last.Images[0])
	}

	// 请求构造后 pendingImages 应被清空，避免下一条普通消息重复携带图片。
	b.mu.Lock()
	st := b.sessions["sess"]
	var pending int
	if st != nil {
		pending = len(st.pendingImages)
	}
	b.mu.Unlock()
	if pending != 0 {
		t.Fatalf("len(pendingImages) = %d, want 0 after stream request", pending)
	}
}

func TestBackendInvokeImageMapFormAttachToLastUserMessage(t *testing.T) {
	path := writeTestImage(t)
	p := &imageRecordingProvider{}
	b := NewBackend(p, nil)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"what is in the image?", map[string]any{"images": []string{path}}}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}

	req := waitForImageRequest(t, p)
	if len(req.Messages) != 1 {
		t.Fatalf("len(req.Messages) = %d, want 1", len(req.Messages))
	}
	last := req.Messages[0]
	if len(last.Images) != 1 {
		t.Fatalf("last.Images = %#v, want one image", last.Images)
	}
	if last.Images[0].MediaType != "image/png" {
		t.Fatalf("last.Images[0].MediaType = %q, want image/png", last.Images[0].MediaType)
	}
}

func TestBackendInvokeImageFileMissingReturnsErrorWithoutMessage(t *testing.T) {
	p := &imageRecordingProvider{}
	b := NewBackend(p, nil)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	_, err := b.Invoke(ctx, "sess", "chat", []any{"describe", []string{filepath.Join(t.TempDir(), "missing.png")}})
	if err == nil {
		t.Fatal("Invoke() error = nil, want missing file error")
	}

	b.mu.Lock()
	st := b.sessions["sess"]
	var msgCount int
	if st != nil {
		msgCount = len(st.messages)
	}
	b.mu.Unlock()
	if msgCount != 0 {
		t.Fatalf("len(st.messages) = %d, want 0 after failed image load", msgCount)
	}
}
