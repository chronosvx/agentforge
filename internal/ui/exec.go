package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosvx/agentforge/internal/executor"
)

// execResultMsg 承载 /exec 执行结果。
type execResultMsg struct {
	result string
	err    error
}

// execRunCmd 异步执行 /exec 命令。
func (m *Model) execRunCmd(raw string) tea.Cmd {
	return func() tea.Msg {
		typ := "local"
		rest := strings.TrimSpace(raw)
		if strings.HasPrefix(rest, "docker ") {
			typ = "docker"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "docker "))
		} else if strings.HasPrefix(rest, "ssh ") {
			typ = "ssh"
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "ssh "))
		}
		m.executorType = typ

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var ex executor.Executor
		switch typ {
		case "docker":
			ex = &executor.DockerExecutor{Image: "alpine"}
		case "ssh":
			ex = &executor.SSHExecutor{Host: "localhost"}
		default:
			ex = &executor.LocalExecutor{}
		}

		res, err := ex.Run(ctx, executor.Spec{Command: "sh", Args: []string{"-c", rest}})
		if err != nil {
			return execResultMsg{err: err}
		}
		return execResultMsg{result: strings.TrimSpace(res.Stdout)}
	}
}

// renderExecPanel 渲染执行面板。
func (m *Model) renderExecPanel() string {
	var b strings.Builder
	b.WriteString("执行面板\n")
	b.WriteString(fmt.Sprintf("执行器: %s\n", m.executorType))
	b.WriteString(fmt.Sprintf("命令: %s\n", m.execCmd))
	if m.execRunning {
		b.WriteString("运行中...\n")
	}
	if m.execErr != nil {
		b.WriteString("错误: " + sanitizeTerminalText(m.execErr.Error()) + "\n")
	}
	if m.execResult != "" {
		b.WriteString("结果:\n" + sanitizeTerminalText(m.execResult) + "\n")
	}
	return b.String()
}
