package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chronosvx/agentforge/internal/client"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/ui"
)

// extractHeadlessOptions 从参数中提取 --headless、--prompt 与 --format，返回剩余参数。
// 它避免修改 internal/config 的 flag 集合，同时让 headless 参数可被主流程识别。
func extractHeadlessOptions(args []string) (headless bool, prompt string, format string, rest []string, err error) {
	format = "text"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--headless":
			headless = true
		case strings.HasPrefix(arg, "--headless="):
			switch strings.TrimPrefix(arg, "--headless=") {
			case "true":
				headless = true
			case "false":
				headless = false
			default:
				return false, "", "", nil, fmt.Errorf("invalid --headless value %q", strings.TrimPrefix(arg, "--headless="))
			}
		case arg == "--prompt":
			if i+1 >= len(args) {
				return false, "", "", nil, errors.New("--prompt requires a value")
			}
			prompt = args[i+1]
			i++
		case strings.HasPrefix(arg, "--prompt="):
			prompt = strings.TrimPrefix(arg, "--prompt=")
		case arg == "--format":
			if i+1 >= len(args) {
				return false, "", "", nil, errors.New("--format requires a value")
			}
			format = args[i+1]
			i++
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		default:
			rest = append(rest, arg)
		}
	}
	if format != "text" && format != "json" && format != "stream-json" {
		return false, "", "", nil, fmt.Errorf("invalid --format value %q (want text, json or stream-json)", format)
	}
	return headless, prompt, format, rest, nil
}

// runHeadless 在非交互模式下通过现有 ChatBackend 发送 prompt，等待流式完成后打印最终结果。
func runHeadless(cl ui.ChatBackend, cfg *config.Config, prompt string, format string) error {
	if strings.TrimSpace(prompt) == "" {
		return errors.New("headless: --prompt is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	snap, err := cl.Open(ctx, cfg.Workspace, cfg.SessionID, nil)
	if err != nil {
		return fmt.Errorf("headless: open session: %w", err)
	}
	events, err := cl.Subscribe(ctx, snap.ChannelID, snap.Version)
	if err != nil {
		return fmt.Errorf("headless: subscribe: %w", err)
	}
	if _, err := cl.Invoke(ctx, snap.ChannelID, "chat", []any{prompt}); err != nil {
		return fmt.Errorf("headless: invoke: %w", err)
	}

	if format == "stream-json" {
		var streamErr error
		content, err := waitForHeadlessResultStream(ctx, events, snap.ChannelID, func(latest string) {
			if streamErr != nil {
				return
			}
			if err := writeHeadlessStreamEvent(headlessStreamEvent{
				Type:    "delta",
				Session: snap.ChannelID,
				Content: latest,
			}); err != nil {
				streamErr = fmt.Errorf("headless: encode delta: %w", err)
			}
		})
		if err != nil {
			return err
		}
		if streamErr != nil {
			return streamErr
		}
		if err := writeHeadlessStreamEvent(headlessStreamEvent{
			Type:    "done",
			Session: snap.ChannelID,
			Channel: snap.ChannelID,
			Content: content,
			OK:      true,
		}); err != nil {
			return fmt.Errorf("headless: encode stream result: %w", err)
		}
		return nil
	}

	content, err := waitForHeadlessResult(ctx, events, snap.ChannelID)
	if err != nil {
		return err
	}
	if format == "json" {
		type headlessResult struct {
			Session string `json:"session"`
			Channel string `json:"channel"`
			Content string `json:"content"`
			OK      bool   `json:"ok"`
		}
		if err := json.NewEncoder(os.Stdout).Encode(headlessResult{
			Session: snap.ChannelID,
			Channel: snap.ChannelID,
			Content: content,
			OK:      true,
		}); err != nil {
			return fmt.Errorf("headless: encode result: %w", err)
		}
		return nil
	}
	fmt.Println(content)
	return nil
}

// waitForHeadlessResult 消费事件流直到完成。Provider 后端会发送 done 事件；
// 其他后端（如 DSH API）没有 done 事件时，在最后一条增量后短暂静默即视为完成。
func waitForHeadlessResult(ctx context.Context, events <-chan client.Event, channelID string) (string, error) {
	return waitForHeadlessResultStream(ctx, events, channelID, nil)
}

// waitForHeadlessResultStream 消费事件流直到完成，并在 assistant 内容每次变化时
// 调用 onContent 回调。
func waitForHeadlessResultStream(ctx context.Context, events <-chan client.Event, channelID string, onContent func(string)) (string, error) {
	const idleTimeout = 3 * time.Second

	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()

	content := ""
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case ev, ok := <-events:
			if !ok {
				if content != "" {
					return content, nil
				}
				return "", fmt.Errorf("headless: event stream for %s closed before completion", channelID)
			}
			if ev.Operation == "error" {
				return "", fmt.Errorf("headless: backend error: %s", headlessErrorFromState(ev.State))
			}
			if latest := headlessContentFromState(ev.State); latest != content {
				content = latest
				if onContent != nil {
					onContent(content)
				}
			}
			if ev.Operation == "done" {
				return content, nil
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTimeout)
		case <-idle.C:
			if content != "" {
				return content, nil
			}
			return "", fmt.Errorf("headless: timed out waiting for result on %s", channelID)
		}
	}
}

// headlessStreamEvent 是 --format stream-json 输出的单行 JSON 结构。
type headlessStreamEvent struct {
	Type    string `json:"type"`
	Session string `json:"session"`
	Channel string `json:"channel,omitempty"`
	Content string `json:"content"`
	OK      bool   `json:"ok,omitempty"`
}

// writeHeadlessStreamEvent 向 stdout 写一行 JSON Lines。
func writeHeadlessStreamEvent(ev headlessStreamEvent) error {
	return json.NewEncoder(os.Stdout).Encode(ev)
}

// headlessContentFromState 从后端事件状态中提取最后一条 assistant 消息。
func headlessContentFromState(state map[string]any) string {
	switch msgs := state["messages"].(type) {
	case []any:
		for i := len(msgs) - 1; i >= 0; i-- {
			m, ok := msgs[i].(map[string]any)
			if !ok {
				continue
			}
			role, _ := m["role"].(string)
			if role != "assistant" {
				continue
			}
			if s, ok := m["content"].(string); ok {
				return s
			}
		}
	case []map[string]any:
		for i := len(msgs) - 1; i >= 0; i-- {
			role, _ := msgs[i]["role"].(string)
			if role != "assistant" {
				continue
			}
			if s, ok := msgs[i]["content"].(string); ok {
				return s
			}
		}
	}
	return ""
}

// headlessErrorFromState 从后端事件状态中提取错误信息。
func headlessErrorFromState(state map[string]any) string {
	if state == nil {
		return "unknown error"
	}
	if cost, ok := state["cost"].(map[string]any); ok {
		if s, ok := cost["lastError"].(string); ok && s != "" {
			return s
		}
	}
	if s, ok := state["error"].(string); ok && s != "" {
		return s
	}
	return "unknown error"
}
