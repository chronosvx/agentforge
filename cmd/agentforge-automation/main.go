// Command agentforge-automation 从 JSON 配置加载 Job，并调度执行 agentforge
// Headless 任务或其他命令。
//
// 用法示例：
//
//	agentforge-automation --config jobs.json
//	agentforge-automation --config jobs.json --once
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/chronosvx/agentforge/internal/automation"
	"github.com/chronosvx/agentforge/internal/config"
)

// jobFile 是 jobs.json 中单个任务的声明格式。
type jobFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Cron     string `json:"cron,omitempty"`
	Interval string `json:"interval,omitempty"` // Go duration 字符串，例如 "5s"、"1h"
	Enabled  *bool  `json:"enabled,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agentforge-automation:", err)
		os.Exit(1)
	}
}

func run() error {
	var configPath string
	var once, dryRun bool
	var dataDir string

	flag.StringVar(&configPath, "config", "", "Job 配置文件路径（JSON）")
	flag.BoolVar(&once, "once", false, "只立即执行一次所有启用的 Job，然后退出")
	flag.BoolVar(&dryRun, "dry-run", false, "只打印将执行的命令，不真正执行")
	flag.StringVar(&dataDir, "data-dir", config.DefaultDataDir, "数据目录（透传给子进程环境）")
	flag.Parse()

	if configPath == "" {
		return errors.New("--config is required")
	}

	jobs, err := loadJobs(configPath)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return errors.New("no enabled jobs found in config")
	}

	if once {
		for _, job := range jobs {
			if !job.Enabled {
				continue
			}
			if err := runJob(job, dataDir, dryRun); err != nil {
				fmt.Fprintf(os.Stderr, "agentforge-automation: job %s failed: %v\n", job.ID, err)
			}
		}
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	scheduler := automation.NewScheduler(func(job automation.Job) {
		if err := runJob(job, dataDir, dryRun); err != nil {
			fmt.Fprintf(os.Stderr, "agentforge-automation: job %s failed: %v\n", job.ID, err)
		}
	})
	for _, job := range jobs {
		if err := scheduler.AddJob(job); err != nil {
			return fmt.Errorf("add job %s: %w", job.ID, err)
		}
	}

	fmt.Printf("agentforge-automation: loaded %d job(s), waiting for schedule (Ctrl+C to stop)\n", len(jobs))
	if err := scheduler.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	scheduler.Stop()
	fmt.Println("agentforge-automation: stopped")
	return nil
}

// loadJobs 读取 JSON 文件，支持顶层数组或 {"jobs":[...]} 对象。
func loadJobs(path string) ([]automation.Job, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var list []jobFile
	if err := json.Unmarshal(data, &list); err != nil {
		var wrapper struct {
			Jobs []jobFile `json:"jobs"`
		}
		if err2 := json.Unmarshal(data, &wrapper); err2 != nil {
			return nil, fmt.Errorf("parse jobs config %s: %v", path, err)
		}
		list = wrapper.Jobs
	}

	jobs := make([]automation.Job, 0, len(list))
	for _, jf := range list {
		enabled := true
		if jf.Enabled != nil {
			enabled = *jf.Enabled
		}
		if !enabled {
			continue
		}

		var interval time.Duration
		if jf.Interval != "" {
			interval, err = time.ParseDuration(jf.Interval)
			if err != nil {
				return nil, fmt.Errorf("job %s: invalid interval %q: %w", jf.ID, jf.Interval, err)
			}
		}
		if jf.Cron == "" && interval <= 0 {
			return nil, fmt.Errorf("job %s: must set cron or interval", jf.ID)
		}
		name := jf.Name
		if name == "" {
			name = jf.ID
		}

		jobs = append(jobs, automation.Job{
			ID:       jf.ID,
			Name:     name,
			Command:  jf.Command,
			Cron:     jf.Cron,
			Interval: interval,
			Enabled:  true,
		})
	}
	return jobs, nil
}

// runJob 在子进程中执行 Job.Command。
//
// 命令通常形如 `agentforge --headless --prompt "..."`；也会透传
// DSH_AUTOMATION_JOB_ID / DSH_AUTOMATION_JOB_NAME 环境变量供审计使用。
func runJob(job automation.Job, dataDir string, dryRun bool) error {
	if dryRun {
		fmt.Printf("[dry-run] job=%s command=%q\n", job.ID, job.Command)
		return nil
	}

	cmd := exec.Command("sh", "-c", job.Command)
	cmd.Env = append(os.Environ(),
		"DSH_AUTOMATION_JOB_ID="+job.ID,
		"DSH_AUTOMATION_JOB_NAME="+job.Name,
		"DSH_DATA_DIR="+dataDir,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}
