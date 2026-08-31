// Package orchestrator 提供多 Agent 协作与任务编排。
//
// 它把一个复杂任务拆成多个子任务，子任务可以声明依赖关系；
// Orchestrator 负责计算可并行就绪的子任务、记录完成/失败状态，
// 并汇总整个任务的执行情况。该包不依赖 internal/ui。
package orchestrator

import (
	"errors"
	"fmt"
	"sync"

	"github.com/chronosvx/agentforge/internal/agent"
)

// Status 表示单个子任务的编排状态。
type Status string

const (
	// StatusPending 表示子任务尚未满足就绪条件。
	StatusPending Status = "pending"
	// StatusReady 表示子任务已满足依赖条件，等待被取出执行。
	StatusReady Status = "ready"
	// StatusRunning 表示子任务已被 NextReady 取出，正在执行中。
	StatusRunning Status = "running"
	// StatusDone 表示子任务已完成。
	StatusDone Status = "done"
	// StatusFailed 表示子任务失败，或其依赖失败导致无法继续执行。
	StatusFailed Status = "failed"
)

// TaskStatus 表示整个任务的编排状态。
//
// 它是 Status 的别名，便于调用方用同一组状态常量表达任务级状态。
type TaskStatus = Status

const (
	// TaskStatusPending 表示任务尚未开始执行。
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning 表示任务有子任务正在执行或已就绪。
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusDone 表示任务所有子任务均已完成。
	TaskStatusDone TaskStatus = "done"
	// TaskStatusFailed 表示任务中存在失败子任务。
	TaskStatusFailed TaskStatus = "failed"
)

// SubTask 是任务中的一个可执行子任务。
//
// DependsOn 声明该子任务依赖的其他子任务 ID；只有所有依赖都处于
// StatusDone 时，该子任务才会被 NextReady 返回。
type SubTask struct {
	ID        string
	AgentID   string
	Action    agent.Action
	Status    Status
	DependsOn []string
}

// Task 是一个可被 Orchestrator 管理的复杂任务。
type Task struct {
	ID          string
	Name        string
	Description string
	SubTasks    []SubTask
}

// Summary 是一次任务/全部任务的汇总快照。
type Summary struct {
	TaskID  string
	Total   int
	Pending int
	Ready   int
	Running int
	Done    int
	Failed  int
	Status  TaskStatus
}

// Orchestrator 管理多个 Task，并负责子任务的依赖就绪与状态流转。
//
// 零值可用；所有方法均加锁，可安全并发调用。
type Orchestrator struct {
	mu    sync.Mutex
	tasks map[string]*Task
	order []string
}

// New 创建一个空的 Orchestrator。
func New() *Orchestrator {
	return &Orchestrator{tasks: make(map[string]*Task)}
}

// NewOrchestrator 是 New 的别名，便于调用方按名称实例化。
func NewOrchestrator() *Orchestrator {
	return New()
}

// AddTask 添加一个任务。任务 ID 不能为空且不能重复；子任务 ID 必须在
// 任务内唯一，DependsOn 只能引用任务内已存在的子任务，且不能形成依赖环。
// 添加时会把所有子任务重置为 StatusPending。
func (o *Orchestrator) AddTask(task Task) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.tasks == nil {
		o.tasks = make(map[string]*Task)
	}
	if task.ID == "" {
		return errors.New("orchestrator: task id is empty")
	}
	if _, ok := o.tasks[task.ID]; ok {
		return fmt.Errorf("orchestrator: task %q already exists", task.ID)
	}

	ids := make(map[string]struct{}, len(task.SubTasks))
	for _, st := range task.SubTasks {
		if st.ID == "" {
			return fmt.Errorf("orchestrator: task %q has a subtask with empty id", task.ID)
		}
		if _, dup := ids[st.ID]; dup {
			return fmt.Errorf("orchestrator: task %q has duplicate subtask id %q", task.ID, st.ID)
		}
		ids[st.ID] = struct{}{}
	}

	for i := range task.SubTasks {
		st := &task.SubTasks[i]
		for _, dep := range st.DependsOn {
			if dep == st.ID {
				return fmt.Errorf("orchestrator: subtask %q cannot depend on itself", st.ID)
			}
			if _, ok := ids[dep]; !ok {
				return fmt.Errorf("orchestrator: subtask %q depends on unknown subtask %q", st.ID, dep)
			}
		}
	}

	if err := detectCycle(task.SubTasks); err != nil {
		return err
	}

	cp := Task{
		ID:          task.ID,
		Name:        task.Name,
		Description: task.Description,
		SubTasks:    make([]SubTask, len(task.SubTasks)),
	}
	for i, st := range task.SubTasks {
		st.Status = StatusPending
		st.DependsOn = append([]string(nil), st.DependsOn...)
		cp.SubTasks[i] = st
	}

	o.tasks[task.ID] = &cp
	o.order = append(o.order, task.ID)
	return nil
}

// NextReady 返回下一个满足依赖条件的子任务，并将其标记为 StatusRunning。
//
// 不传 taskID 时按任务添加顺序在所有任务中查找；传一个 taskID 时只在
// 指定任务中查找。没有可执行子任务时返回 nil。
func (o *Orchestrator) NextReady(taskIDs ...string) *SubTask {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.tasks == nil {
		return nil
	}
	if len(taskIDs) > 1 {
		return nil
	}

	if len(taskIDs) == 1 {
		t, err := o.taskLocked(taskIDs[0])
		if err != nil {
			return nil
		}
		return nextReadyLocked(t)
	}

	for _, id := range o.order {
		t := o.tasks[id]
		if st := nextReadyLocked(t); st != nil {
			return st
		}
	}
	return nil
}

// MarkDone 将子任务标记为完成。
//
// 支持两种调用方式：
//   - MarkDone("subtask-id")：在所有任务中按子任务 ID 查找；
//   - MarkDone("task-id", "subtask-id")：在指定任务中查找。
func (o *Orchestrator) MarkDone(args ...string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	t, st, err := o.locateSubTask(args)
	if err != nil {
		return err
	}
	if st.Status == StatusDone {
		return nil
	}
	if st.Status == StatusFailed {
		return fmt.Errorf("orchestrator: subtask %q already failed, cannot mark done", st.ID)
	}
	st.Status = StatusDone
	_ = t
	return nil
}

// MarkFailed 将子任务标记为失败，并级联失败所有直接或间接依赖它的子任务。
//
// 支持与 MarkDone 相同的两种调用方式。
func (o *Orchestrator) MarkFailed(args ...string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	t, st, err := o.locateSubTask(args)
	if err != nil {
		return err
	}
	if st.Status == StatusFailed {
		return nil
	}
	if st.Status == StatusDone {
		return fmt.Errorf("orchestrator: subtask %q already done, cannot mark failed", st.ID)
	}
	st.Status = StatusFailed
	failDependentsLocked(t, st.ID)
	return nil
}

// Status 返回任务状态。
//
// 不传参数时返回所有任务的总体状态；传一个 taskID 时返回该任务状态。
func (o *Orchestrator) Status(taskIDs ...string) TaskStatus {
	o.mu.Lock()
	defer o.mu.Unlock()

	if len(taskIDs) == 0 {
		return overallStatusLocked(o)
	}
	if len(taskIDs) != 1 {
		return TaskStatusFailed
	}
	t, err := o.taskLocked(taskIDs[0])
	if err != nil {
		return TaskStatusFailed
	}
	return taskStatusLocked(t)
}

// Summary 返回任务汇总。
//
// 不传参数时汇总所有任务；传一个 taskID 时只汇总指定任务。
func (o *Orchestrator) Summary(taskIDs ...string) Summary {
	o.mu.Lock()
	defer o.mu.Unlock()

	if len(taskIDs) == 0 {
		var s Summary
		for _, id := range o.order {
			t := o.tasks[id]
			ts := summarizeTaskLocked(t)
			s.Total += ts.Total
			s.Pending += ts.Pending
			s.Ready += ts.Ready
			s.Running += ts.Running
			s.Done += ts.Done
			s.Failed += ts.Failed
		}
		s.Status = overallStatusLocked(o)
		return s
	}
	if len(taskIDs) != 1 {
		return Summary{}
	}
	t, err := o.taskLocked(taskIDs[0])
	if err != nil {
		return Summary{TaskID: taskIDs[0]}
	}
	return summarizeTaskLocked(t)
}

func (o *Orchestrator) taskLocked(id string) (*Task, error) {
	if id == "" {
		return nil, errors.New("orchestrator: task id is empty")
	}
	t, ok := o.tasks[id]
	if !ok {
		return nil, fmt.Errorf("orchestrator: task %q not found", id)
	}
	return t, nil
}

func (o *Orchestrator) locateSubTask(args []string) (*Task, *SubTask, error) {
	if o.tasks == nil {
		return nil, nil, errors.New("orchestrator: no tasks added")
	}

	switch len(args) {
	case 1:
		for _, id := range o.order {
			t := o.tasks[id]
			for i := range t.SubTasks {
				if t.SubTasks[i].ID == args[0] {
					return t, &t.SubTasks[i], nil
				}
			}
		}
		return nil, nil, fmt.Errorf("orchestrator: subtask %q not found", args[0])
	case 2:
		// 优先按 (taskID, subTaskID) 解释，也兼容 (subTaskID, taskID)。
		if t, err := o.taskLocked(args[0]); err == nil {
			for i := range t.SubTasks {
				if t.SubTasks[i].ID == args[1] {
					return t, &t.SubTasks[i], nil
				}
			}
		}
		if t, err := o.taskLocked(args[1]); err == nil {
			for i := range t.SubTasks {
				if t.SubTasks[i].ID == args[0] {
					return t, &t.SubTasks[i], nil
				}
			}
		}
		return nil, nil, fmt.Errorf("orchestrator: subtask %q not found in task %q", args[1], args[0])
	default:
		return nil, nil, errors.New("orchestrator: expected subtask id or task id + subtask id")
	}
}

func nextReadyLocked(t *Task) *SubTask {
	for i := range t.SubTasks {
		st := &t.SubTasks[i]
		if st.Status == StatusRunning || st.Status == StatusDone || st.Status == StatusFailed {
			continue
		}
		if !dependenciesDoneLocked(t, st) {
			continue
		}
		st.Status = StatusRunning
		cp := *st
		cp.DependsOn = append([]string(nil), st.DependsOn...)
		return &cp
	}
	return nil
}

func dependenciesDoneLocked(t *Task, st *SubTask) bool {
	statusByID := make(map[string]Status, len(t.SubTasks))
	for i := range t.SubTasks {
		statusByID[t.SubTasks[i].ID] = t.SubTasks[i].Status
	}
	for _, dep := range st.DependsOn {
		if statusByID[dep] != StatusDone {
			return false
		}
	}
	return true
}

func failDependentsLocked(t *Task, failedID string) {
	for i := range t.SubTasks {
		st := &t.SubTasks[i]
		if st.Status == StatusDone || st.Status == StatusFailed {
			continue
		}
		for _, dep := range st.DependsOn {
			if dep == failedID {
				st.Status = StatusFailed
				failDependentsLocked(t, st.ID)
				break
			}
		}
	}
}

func taskStatusLocked(t *Task) TaskStatus {
	hasRunningOrReady := false
	hasPending := false
	for i := range t.SubTasks {
		switch t.SubTasks[i].Status {
		case StatusFailed:
			return TaskStatusFailed
		case StatusRunning, StatusReady:
			hasRunningOrReady = true
		case StatusPending:
			hasPending = true
		}
	}
	if hasRunningOrReady {
		return TaskStatusRunning
	}
	if hasPending {
		return TaskStatusPending
	}
	return TaskStatusDone
}

func overallStatusLocked(o *Orchestrator) TaskStatus {
	if o.tasks == nil || len(o.tasks) == 0 {
		return TaskStatusPending
	}
	hasRunningOrReady := false
	hasPending := false
	for _, id := range o.order {
		t := o.tasks[id]
		for i := range t.SubTasks {
			switch t.SubTasks[i].Status {
			case StatusFailed:
				return TaskStatusFailed
			case StatusRunning, StatusReady:
				hasRunningOrReady = true
			case StatusPending:
				hasPending = true
			}
		}
	}
	if hasRunningOrReady {
		return TaskStatusRunning
	}
	if hasPending {
		return TaskStatusPending
	}
	return TaskStatusDone
}

func summarizeTaskLocked(t *Task) Summary {
	s := Summary{TaskID: t.ID}
	for i := range t.SubTasks {
		st := &t.SubTasks[i]
		s.Total++
		switch st.Status {
		case StatusPending:
			s.Pending++
		case StatusReady:
			s.Ready++
		case StatusRunning:
			s.Running++
		case StatusDone:
			s.Done++
		case StatusFailed:
			s.Failed++
		}
	}
	s.Status = taskStatusLocked(t)
	return s
}

func detectCycle(subtasks []SubTask) error {
	byID := make(map[string]SubTask, len(subtasks))
	for _, st := range subtasks {
		byID[st.ID] = st
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(subtasks))

	var visit func(id string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			return fmt.Errorf("orchestrator: dependency cycle detected at %q", id)
		case done:
			return nil
		}
		state[id] = visiting
		for _, dep := range byID[id].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = done
		return nil
	}

	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
