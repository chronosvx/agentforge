package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/agentflow"
	"github.com/chronosvx/agentforge/internal/multiagent"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// runMultiAgent 实现 `agentforge multiagent` 子命令：并行执行多个 Agent 任务。
func runMultiAgent(args []string) error {
	fs := flag.NewFlagSet("multiagent", flag.ContinueOnError)
	tasksFlag := fs.String("tasks", "", "逗号分隔的多个任务提示词")
	workdir := fs.String("workdir", ".", "工作目录")
	autoApprove := fs.Bool("yes", false, "自动批准所有动作")
	testCmd := fs.String("test", "", "测试命令；默认不测试")
	timeout := fs.Duration("timeout", 5*time.Minute, "单条命令超时")
	jsonOut := fs.Bool("json", false, "以 JSON 格式输出结果")
	if err := fs.Parse(args); err != nil {
		return err
	}

	prompts := splitTasks(*tasksFlag)
	if len(prompts) == 0 && fs.NArg() > 0 {
		prompts = fs.Args()
	}
	if len(prompts) == 0 {
		return errors.New("multiagent: --tasks is required")
	}

	tasks := make([]multiagent.Task, 0, len(prompts))
	for i, p := range prompts {
		tasks = append(tasks, multiagent.Task{
			ID:      fmt.Sprintf("task-%d", i),
			Prompt:  p,
			WorkDir: *workdir,
			TestCmd: *testCmd,
		})
	}

	runner := multiagent.NewRunner(func(ctx context.Context, task multiagent.Task) (string, error) {
		r := agentflow.NewRunner(agentflow.NewStaticPlanner(), nil)
		res, err := r.Run(ctx, agentflow.Request{
			Prompt:      task.Prompt,
			WorkDir:     task.WorkDir,
			AutoApprove: *autoApprove,
			Timeout:     *timeout,
			TestCmd:     task.TestCmd,
		})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("phase=%s diff=%d", res.Session.Phase, len(res.Diff)), nil
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	results, err := runner.Run(ctx, tasks, nil)

	if *jsonOut {
		type jsonResult struct {
			TaskID string `json:"task_id"`
			Output string `json:"output,omitempty"`
			Error  string `json:"error,omitempty"`
		}
		out := make([]jsonResult, 0, len(results))
		for _, r := range results {
			item := jsonResult{TaskID: r.TaskID, Output: r.Output}
			if r.Err != nil {
				item.Error = r.Err.Error()
			}
			out = append(out, item)
		}
		data, marshalErr := json.MarshalIndent(map[string]any{"results": out, "failed": err != nil}, "", "  ")
		if marshalErr != nil {
			return marshalErr
		}
		fmt.Println(string(data))
	} else {
		for _, r := range results {
			if r.Err != nil {
				fmt.Printf("%s: ERROR %v\n", r.TaskID, r.Err)
			} else {
				fmt.Printf("%s: %s\n", r.TaskID, r.Output)
			}
		}
	}
	if err != nil {
		return err
	}
	return nil
}

// splitTasks 将逗号分隔的任务提示词拆分为去空白的列表。
func splitTasks(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
