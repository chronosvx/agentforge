package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/mcp"
)

// runMCP 实现 `agentforge mcp --url <url> [--call <tool> --args <json>]`。
func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	url := fs.String("url", "", "MCP Server URL 或 stdio 命令（必填）")
	call := fs.String("call", "", "要调用的工具名（可选）")
	rawArgs := fs.String("args", "{}", "调用参数 JSON（可选）")
	timeout := fs.Duration("timeout", 30*time.Second, "超时")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*url) == "" {
		return errors.New("mcp: --url is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	client := mcp.New(*url)
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("mcp: list tools: %w", err)
	}

	fmt.Printf("MCP connected: %s\n", *url)
	fmt.Printf("Tools (%d):\n", len(tools))
	for _, t := range tools {
		fmt.Printf("  - %s: %s\n", t.Name, t.Description)
	}

	if strings.TrimSpace(*call) != "" {
		var params map[string]any
		if err := json.Unmarshal([]byte(*rawArgs), &params); err != nil {
			return fmt.Errorf("mcp: invalid --args JSON: %w", err)
		}
		result, err := client.CallTool(ctx, *call, params)
		if err != nil {
			return fmt.Errorf("mcp: call %s: %w", *call, err)
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Printf("\nCall %s result:\n%s\n", *call, data)
	}
	return nil
}
