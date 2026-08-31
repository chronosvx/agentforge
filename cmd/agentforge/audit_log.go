package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runAudit 实现 `agentforge audit [--tail N]`：查看本地审计日志。
func runAudit(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	tail := fs.Int("tail", 50, "显示最后 N 条审计记录")
	dataDir := fs.String("data-dir", "~/.agentforge", "数据目录")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir, err := expandHome(*dataDir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "audit.log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no audit log found at", path)
			return nil
		}
		return err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	start := 0
	if len(lines) > *tail {
		start = len(lines) - *tail
	}
	for _, line := range lines[start:] {
		fmt.Println(line)
	}
	fmt.Printf("\n(%d audit entries shown, %d total)\n", len(lines)-start, len(lines))
	return nil
}
