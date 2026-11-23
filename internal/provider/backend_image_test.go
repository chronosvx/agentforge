package provider

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/session"
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
	return waitForRequestCount(t, p, 1)
}

func waitForRequestCount(t *testing.T, p *imageRecordingProvider, want int) ChatRequest {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		count := len(p.reqs)
		var req ChatRequest
		if count > 0 {
			req = p.reqs[count-1]
		}
		p.mu.Unlock()
		if count >= want {
			return req
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d StreamChat request(s)", want)
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

	// 请求构造后图片仍绑定在该条用户消息上，供后续追问继续携带。
	b.mu.Lock()
	st := b.sessions["sess"]
	var attached int
	if st != nil && len(st.messageImages) > 0 {
		for _, images := range st.messageImages {
			attached += len(images)
		}
	}
	b.mu.Unlock()
	if attached != 1 {
		t.Fatalf("attached images = %d, want 1 retained on message", attached)
	}
}

func TestBackendImagePersistsAcrossFollowUpTurns(t *testing.T) {
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
	waitForImageRequest(t, p)
	waitForStreamIdle(t, b, "sess")

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"what about the top-left corner?"}); err != nil {
		t.Fatalf("follow-up Invoke() error: %v", err)
	}
	req := waitForRequestCount(t, p, 2)

	var imageMessages int
	for _, msg := range req.Messages {
		if len(msg.Images) > 0 {
			imageMessages++
		}
	}
	if imageMessages != 1 {
		t.Fatalf("messages with images = %d, want 1 (the original user turn)", imageMessages)
	}
	if req.Messages[0].Role != "user" || len(req.Messages[0].Images) != 1 {
		t.Fatalf("first user message lost its image: %#v", req.Messages[0])
	}
	if req.Messages[len(req.Messages)-1].Content != "what about the top-left corner?" {
		t.Fatalf("last message = %#v, want follow-up text", req.Messages[len(req.Messages)-1])
	}
}

func TestBackendSyncHistoryReplacesNextRequestContext(t *testing.T) {
	p := &imageRecordingProvider{}
	b := NewBackend(p, nil)

	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if _, err := b.Invoke(ctx, "sess", "chat", []any{"old question"}); err != nil {
		t.Fatalf("Invoke() error: %v", err)
	}
	waitForImageRequest(t, p)
	waitForStreamIdle(t, b, "sess")

	compacted := []client.Message{
		{Role: "system", Content: "summary of old question"},
		{Role: "user", Content: "new question"},
	}
	if err := b.SyncHistory(ctx, "sess", compacted); err != nil {
		t.Fatalf("SyncHistory() error: %v", err)
	}

	if _, err := b.Invoke(ctx, "sess", "chat", []any{"follow up"}); err != nil {
		t.Fatalf("Invoke() after sync error: %v", err)
	}
	req := waitForRequestCount(t, p, 2)
	if len(req.Messages) != 3 {
		t.Fatalf("len(req.Messages) = %d, want 3: %#v", len(req.Messages), req.Messages)
	}
	if req.Messages[0].Content != "summary of old question" || req.Messages[1].Content != "new question" {
		t.Fatalf("request did not use synced history: %#v", req.Messages)
	}
	for _, msg := range req.Messages {
		if msg.Content == "old question" {
			t.Fatalf("request still contains cleared history: %#v", req.Messages)
		}
	}
}

func TestBackendSyncHistoryPersistsAcrossReopen(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore() error: %v", err)
	}
	p := &imageRecordingProvider{}
	b := NewBackend(p, store)
	ctx := context.Background()
	if _, err := b.Open(ctx, "workspace", "sess", nil); err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	synced := []client.Message{
		{Role: "system", Content: "summary"},
		{Role: "user", Content: "kept question"},
	}
	if err := b.SyncHistory(ctx, "sess", synced); err != nil {
		t.Fatalf("SyncHistory() error: %v", err)
	}

	reopened := NewBackend(p, store)
	snap, err := reopened.Open(ctx, "workspace", "sess", nil)
	if err != nil {
		t.Fatalf("reopen Open() error: %v", err)
	}
	msgs, ok := snap.State["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("reopened messages = %#v, want 2 synced messages", snap.State["messages"])
	}
	first, _ := msgs[0].(map[string]any)
	if first["content"] != "summary" {
		t.Fatalf("reopened first message = %#v, want summary", first)
	}
}

func waitForStreamIdle(t *testing.T, b *Backend, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		st := b.sessions[sessionID]
		streaming := st != nil && st.streaming
		b.mu.Unlock()
		if !streaming {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for stream to finish")
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
