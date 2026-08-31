package agentflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chronosvx/agentforge/internal/agent"
	"github.com/chronosvx/agentforge/internal/provider"
)

// LLMPlanner 使用 LLM 根据自然语言请求生成 Agent 计划。
//
// 它要求模型只输出 JSON 数组，数组元素对应 agent.Action，
// 并通过 MaxActions 限制单次计划的最大动作数量。
type LLMPlanner struct {
	Provider   provider.Provider
	Model      string
	MaxActions int
}

// NewLLMPlanner 创建一个 LLMPlanner。MaxActions 默认设置为 10。
func NewLLMPlanner(p provider.Provider, model string) *LLMPlanner {
	return &LLMPlanner{
		Provider:   p,
		Model:      model,
		MaxActions: 10,
	}
}

// llmAction 是模型返回 JSON 数组中一个动作的原始表示。
type llmAction struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	Path        string `json:"path"`
	Content     string `json:"content"`
	Description string `json:"description"`
}

// Plan 调用 Provider 生成 JSON 动作数组，并转换为 []agent.Action。
func (p *LLMPlanner) Plan(ctx context.Context, prompt string, _ *Repo) ([]agent.Action, error) {
	if p == nil || p.Provider == nil {
		return nil, fmt.Errorf("agentflow: llm planner provider is nil")
	}
	if strings.TrimSpace(p.Model) == "" {
		return nil, fmt.Errorf("agentflow: llm planner model is empty")
	}

	systemPrompt := `You are a software agent that plans concrete actions for a code repository.
Respond with ONLY a JSON array of actions. Do not include markdown, explanations, or extra text.
Each action must be an object with exactly these fields:
- type: one of "run_command", "edit_file", "apply_diff"
- command: the shell command to run (required for run_command)
- path: the file path (required for edit_file)
- content: the file content or diff content (required for apply_diff)
- description: a short human-readable description of the action
Example:
[
  {"type": "run_command", "command": "go test ./...", "path": "", "content": "", "description": "Run tests"},
  {"type": "edit_file", "command": "", "path": "README.md", "content": "# Updated", "description": "Update README"}
]`

	resp, err := p.Provider.Chat(ctx, provider.ChatRequest{
		Model: p.Model,
		Messages: []provider.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
	})
	if err != nil {
		return nil, fmt.Errorf("agentflow: llm plan chat: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("agentflow: llm plan chat returned nil response")
	}

	var parsed []llmAction
	if err := json.Unmarshal([]byte(resp.Content), &parsed); err != nil {
		return nil, fmt.Errorf("agentflow: parse llm plan JSON: %w", err)
	}

	actions := make([]agent.Action, 0, len(parsed))
	for i, item := range parsed {
		action, err := convertLLMAction(item)
		if err != nil {
			return nil, fmt.Errorf("agentflow: invalid action %d: %w", i, err)
		}
		actions = append(actions, action)
	}

	if p.MaxActions > 0 && len(actions) > p.MaxActions {
		actions = actions[:p.MaxActions]
	}

	return actions, nil
}

// convertLLMAction 校验并转换一个模型返回的动作对象。
func convertLLMAction(item llmAction) (agent.Action, error) {
	action := agent.Action{
		Type:        agent.ActionType(item.Type),
		Command:     item.Command,
		Path:        item.Path,
		Content:     item.Content,
		Description: item.Description,
	}

	switch action.Type {
	case agent.ActionTypeRunCommand:
		if strings.TrimSpace(action.Command) == "" {
			return agent.Action{}, fmt.Errorf("run_command requires non-empty command")
		}
	case agent.ActionTypeEditFile:
		if strings.TrimSpace(action.Path) == "" {
			return agent.Action{}, fmt.Errorf("edit_file requires non-empty path")
		}
	case agent.ActionTypeApplyDiff:
		if strings.TrimSpace(action.Content) == "" {
			return agent.Action{}, fmt.Errorf("apply_diff requires non-empty content")
		}
	default:
		return agent.Action{}, fmt.Errorf("unknown action type %q", item.Type)
	}

	return action, nil
}
