// Package automation 提供轻量级任务调度能力，用于定时/CI/Webhook 触发
// agentforge Headless 任务或其他命令。
package automation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Job 表示一个可被 Scheduler 调度的任务。
//
// Cron 与 Interval 至少配置一个；同时配置时优先使用 Interval。
type Job struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Command  string        `json:"command"`
	Cron     string        `json:"cron,omitempty"`
	Interval time.Duration `json:"-"`
	Enabled  bool          `json:"enabled"`

	lastRun time.Time
	nextRun time.Time
}

// Scheduler 是一个简单的轮询式调度器。
//
// 它不依赖外部 cron 库，使用短 ticker 检查 Job 是否到期；适合单机自动化，
// 后续可替换为分布式队列或完整 cron 引擎。
type Scheduler struct {
	mu      sync.Mutex
	jobs    []*Job
	handler func(Job)

	cancel context.CancelFunc
	done   chan struct{}
}

// NewScheduler 创建调度器。handler 会在任务到期时被调用，不能为 nil。
func NewScheduler(handler func(Job)) *Scheduler {
	return &Scheduler{handler: handler}
}

// AddJob 添加一个任务并计算首次触发时间。
func (s *Scheduler) AddJob(job Job) error {
	if s == nil {
		return errors.New("automation: scheduler is nil")
	}
	if strings.TrimSpace(job.ID) == "" {
		return errors.New("automation: job id is required")
	}
	if strings.TrimSpace(job.Command) == "" {
		return errors.New("automation: job command is required")
	}
	if strings.TrimSpace(job.Cron) == "" && job.Interval <= 0 {
		return errors.New("automation: job must set cron or interval")
	}

	copyJob := job
	next, err := nextTime(&copyJob, time.Now())
	if err != nil {
		return err
	}
	copyJob.nextRun = next

	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, &copyJob)
	return nil
}

// Jobs 返回当前已添加任务的快照。
func (s *Scheduler) Jobs() []Job {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		if j != nil {
			out = append(out, *j)
		}
	}
	return out
}

// Start 在后台启动调度循环。ctx 取消或调用 Stop 后停止。
func (s *Scheduler) Start(ctx context.Context) error {
	if s == nil {
		return errors.New("automation: scheduler is nil")
	}
	if s.handler == nil {
		return errors.New("automation: scheduler handler is nil")
	}
	if ctx == nil {
		return errors.New("automation: context is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("automation: scheduler already started")
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(runCtx)
	return nil
}

// Stop 停止调度循环并等待退出。未启动时是 no-op。
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if s.cancel == nil {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.mu.Unlock()

	cancel()
	<-done
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkDue(ctx)
		}
	}
}

func (s *Scheduler) checkDue(ctx context.Context) {
	if ctx == nil || ctx.Err() != nil {
		return
	}

	now := time.Now()
	s.mu.Lock()
	var due []Job
	for _, job := range s.jobs {
		if job == nil || !job.Enabled {
			continue
		}
		if job.nextRun.IsZero() {
			next, err := nextTime(job, now)
			if err != nil {
				continue
			}
			job.nextRun = next
		}
		if !now.Before(job.nextRun) {
			due = append(due, *job)
			job.lastRun = now
			next, err := nextTime(job, now)
			if err == nil {
				job.nextRun = next
			} else {
				job.nextRun = time.Time{}
			}
		}
	}
	s.mu.Unlock()

	for _, job := range due {
		if ctx.Err() != nil {
			return
		}
		if s.handler != nil {
			s.handler(job)
		}
	}
}

// nextTime 计算任务的下一次触发时间。
func nextTime(job *Job, after time.Time) (time.Time, error) {
	if job == nil {
		return time.Time{}, errors.New("automation: job is nil")
	}
	if job.Interval > 0 {
		return after.Add(job.Interval), nil
	}
	if strings.TrimSpace(job.Cron) != "" {
		return nextCron(job.Cron, after)
	}
	return time.Time{}, errors.New("automation: job must set cron or interval")
}

// nextCron 计算给定 5 段 cron 表达式在 after 之后的下一次触发时间（UTC）。
//
// 支持字段：分 时 日 月 周；每段支持 `*`、`*/n`、`a-b`、`a,b,c` 和单值。
// 周字段中 0 和 7 都表示周日。
func nextCron(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("automation: cron %q must have 5 fields", expr)
	}

	minuteSet, _, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: cron minute: %w", err)
	}
	hourSet, _, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: cron hour: %w", err)
	}
	daySet, dayWild, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: cron day: %w", err)
	}
	monthSet, _, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: cron month: %w", err)
	}
	weekSet, weekWild, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return time.Time{}, fmt.Errorf("automation: cron week: %w", err)
	}

	t := after.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := 5 * 366 * 24 * 60 // 最多向后找 5 年
	for i := 0; i < limit; i++ {
		if minuteSet[t.Minute()] &&
			hourSet[t.Hour()] &&
			monthSet[int(t.Month())] &&
			cronDayMatches(daySet, dayWild, weekSet, weekWild, t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("automation: no matching time for cron %q within 5 years", expr)
}

func cronDayMatches(daySet map[int]bool, dayWild bool, weekSet map[int]bool, weekWild bool, t time.Time) bool {
	dom := t.Day()
	dow := int(t.Weekday())
	if !dayWild && !weekWild {
		return daySet[dom] || weekSet[dow]
	}
	if !dayWild {
		return daySet[dom]
	}
	if !weekWild {
		return weekSet[dow]
	}
	return true
}

// parseCronField 解析 cron 字段，返回允许值集合与是否通配。
func parseCronField(field string, min, max int) (map[int]bool, bool, error) {
	field = strings.TrimSpace(field)
	allowed := make(map[int]bool)
	if field == "" {
		return nil, false, errors.New("empty field")
	}
	if field == "*" || field == "?" {
		for v := min; v <= max; v++ {
			allowed[v] = true
		}
		return allowed, true, nil
	}

	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false, errors.New("empty field part")
		}

		step := 1
		rangePart := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			rangePart = part[:idx]
			stepStr := part[idx+1:]
			step, err := strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return nil, false, fmt.Errorf("invalid step %q", stepStr)
			}
		}

		start, end := min, max
		if rangePart != "*" {
			if idx := strings.Index(rangePart, "-"); idx >= 0 {
				var err error
				start, err = strconv.Atoi(rangePart[:idx])
				if err != nil {
					return nil, false, fmt.Errorf("invalid range start %q", rangePart[:idx])
				}
				end, err = strconv.Atoi(rangePart[idx+1:])
				if err != nil {
					return nil, false, fmt.Errorf("invalid range end %q", rangePart[idx+1:])
				}
			} else {
				v, err := strconv.Atoi(rangePart)
				if err != nil {
					return nil, false, fmt.Errorf("invalid value %q", rangePart)
				}
				start, end = v, v
			}
		}

		if start < min || end > max || start > end {
			return nil, false, fmt.Errorf("value out of range %q", part)
		}
		for v := start; v <= end; v += step {
			allowed[v] = true
		}
	}
	if max == 7 && allowed[7] {
		// 周字段的 7 等同 0（周日）。
		allowed[0] = true
	}
	return allowed, false, nil
}
