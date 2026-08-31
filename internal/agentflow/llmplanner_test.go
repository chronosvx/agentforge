package agentflow

import (
	"context"
	"testing"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/cost"
	"github.com/chronosvx/agentforge/internal/provider"
)

type fakeLLMProvider struct {
	chatReq  provider.ChatRequest
	response *provider.ChatResponse
}

func (f *fakeLLMProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	f.chatReq = req
	return f.response, nil
}

func (f *fakeLLMProvider) StreamChat(context.Context, provider.ChatRequest) (<-chan provider.StreamEvent, error) {
	return nil, nil
}

func (f *fakeLLMProvider) Name() string {
	return "fake"
}

func (f *fakeLLMProvider) Model() string {
	return "fake-model"
}

func (f *fakeLLMProvider) CostEstimator() *cost.Estimator {
	return nil
}

func TestLLMPlannerPlanParsesTwoActions(t *testing.T) {
	fp := &fakeLLMProvider{
		response: &provider.ChatResponse{
			Content: `[
				{"type": "run_command", "command": "go test ./...", "path": "", "content": "", "description": "Run tests"},
				{"type": "edit_file", "command": "", "path": "README.md", "content": "# Updated\n", "description": "Update README"}
			]`,
		},
	}

	p := NewLLMPlanner(fp, "test-model")
	actions, err := p.Plan(context.Background(), "run tests and update readme", &Repo{WorkDir: "/tmp"})
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}

	if len(actions) != 2 {
		t.Fatalf("len(actions) = %d, want 2", len(actions))
	}

	first := actions[0]
	if first.Type != agent.ActionTypeRunCommand || first.Command != "go test ./..." || first.Description != "Run tests" {
		t.Fatalf("unexpected first action: %+v", first)
	}

	second := actions[1]
	if second.Type != agent.ActionTypeEditFile || second.Path != "README.md" || second.Content != "# Updated\n" || second.Description != "Update README" {
		t.Fatalf("unexpected second action: %+v", second)
	}

	if fp.chatReq.Model != "test-model" {
		t.Fatalf("chat request model = %q, want %q", fp.chatReq.Model, "test-model")
	}
	if fp.chatReq.Stream {
		t.Fatal("chat request Stream = true, want false")
	}
	if len(fp.chatReq.Messages) != 2 {
		t.Fatalf("len(chat request messages) = %d, want 2", len(fp.chatReq.Messages))
	}
	if fp.chatReq.Messages[0].Role != "system" || fp.chatReq.Messages[1].Role != "user" {
		t.Fatalf("unexpected message roles: %+v", fp.chatReq.Messages)
	}
	if fp.chatReq.Messages[1].Content != "run tests and update readme" {
		t.Fatalf("user message = %q, want original prompt", fp.chatReq.Messages[1].Content)
	}
}
