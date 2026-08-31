package agentflow

import (
	"context"
	"strings"

	"github.com/chronosvx/agentforge/internal/agent"
)

// StaticPlanner 是一个面向 Demo/测试的确定性规划器。
//
// 它支持三种自然语言形态：
//
//	run <command>           -> 执行命令
//	edit <path> <content>   -> 写入文件
//	test                     -> 运行测试
//
// 后续可被 LLMPlanner 替换，但接口保持不变。
type StaticPlanner struct {
	TestCmd string
}

// NewStaticPlanner 创建一个静态规划器。TestCmd 为空时默认 "go test ./..."。
func NewStaticPlanner() *StaticPlanner {
	return &StaticPlanner{TestCmd: "go test ./..."}
}

// Plan 根据 prompt 生成动作列表。
func (p *StaticPlanner) Plan(_ context.Context, prompt string, _ *Repo) ([]agent.Action, error) {
	text := strings.TrimSpace(prompt)
	lower := strings.ToLower(text)

	var actions []agent.Action

	switch {
	case strings.HasPrefix(lower, "run "):
		actions = append(actions, agent.Action{
			Type:        agent.ActionTypeRunCommand,
			Command:     strings.TrimSpace(text[4:]),
			Description: "执行命令: " + strings.TrimSpace(text[4:]),
		})
	case strings.HasPrefix(lower, "/run "):
		actions = append(actions, agent.Action{
			Type:        agent.ActionTypeRunCommand,
			Command:     strings.TrimSpace(text[5:]),
			Description: "执行命令: " + strings.TrimSpace(text[5:]),
		})
	case strings.HasPrefix(lower, "edit "):
		rest := strings.TrimSpace(text[5:])
		path, content, ok := strings.Cut(rest, " ")
		if !ok || strings.TrimSpace(path) == "" {
			return nil, errStaticUsage("edit <path> <content>")
		}
		actions = append(actions, agent.Action{
			Type:        agent.ActionTypeEditFile,
			Path:        strings.TrimSpace(path),
			Content:     strings.TrimSpace(content),
			Description: "编辑文件: " + strings.TrimSpace(path),
		})
	case lower == "test" || strings.Contains(lower, "test") || strings.Contains(lower, "测试"):
		actions = append(actions, agent.Action{
			Type:        agent.ActionTypeRunCommand,
			Command:     p.TestCmd,
			Description: "运行测试: " + p.TestCmd,
		})
	default:
		// 未识别时给一个保守的只读/测试计划，避免 Demo 误改文件。
		actions = append(actions, agent.Action{
			Type:        agent.ActionTypeRunCommand,
			Command:     p.TestCmd,
			Description: "运行测试: " + p.TestCmd,
		})
	}
	return actions, nil
}

type staticPlanError string

func (e staticPlanError) Error() string { return string(e) }

func errStaticUsage(msg string) error { return staticPlanError("agentflow: usage: " + msg) }
