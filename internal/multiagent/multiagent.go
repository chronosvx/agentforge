// Package multiagent provides a small task runner that schedules independent
// and dependent tasks through the orchestrator package.
package multiagent

import (
	"context"
	"errors"
	"fmt"

	"github.com/chronosvx/agentforge/internal/orchestrator"
)

// maxConcurrent is the maximum number of tasks executed at the same time.
const maxConcurrent = 4

// Task is a single unit of work for the multi-agent runner.
type Task struct {
	ID           string
	Prompt       string
	WorkDir      string
	TestCmd      string
	Dependencies []string
}

// Result is the outcome of one task execution.
type Result struct {
	TaskID string
	Output string
	Err    error
}

// EventType describes a multi-agent scheduling event.
type EventType string

const (
	EventTaskStarted EventType = "task_started"
	EventTaskDone    EventType = "task_done"
	EventTaskFailed  EventType = "task_failed"
)

// Event is emitted while a multi-agent run is in progress.
type Event struct {
	Type   EventType
	TaskID string
	Output string
	Err    error
}

// Runner executes tasks while respecting their dependencies.
type Runner struct {
	// Execute runs one task and returns its output.
	Execute func(ctx context.Context, task Task) (string, error)
	// OnEvent, when non-nil, is called synchronously for scheduling events.
	OnEvent func(Event)
}

// NewRunner returns a Runner using execute as the task executor.
func NewRunner(execute func(ctx context.Context, task Task) (string, error)) *Runner {
	return &Runner{Execute: execute}
}

// Run schedules tasks with orchestrator and executes ready tasks concurrently.
//
// The returned slice is always in the same order as tasks. Task execution
// errors are stored in Result.Err; Run also returns a non-nil error when the
// orchestrator reports that the overall task set failed.
func (r *Runner) Run(ctx context.Context, tasks []Task, deps map[string][]string) ([]Result, error) {
	if r == nil || r.Execute == nil {
		return nil, errors.New("multiagent: runner Execute is nil")
	}

	results := make([]Result, len(tasks))
	indexByID := make(map[string]int, len(tasks))
	for i, task := range tasks {
		if task.ID == "" {
			return nil, fmt.Errorf("multiagent: task at index %d has empty ID", i)
		}
		if _, ok := indexByID[task.ID]; ok {
			return nil, fmt.Errorf("multiagent: duplicate task ID %q", task.ID)
		}
		indexByID[task.ID] = i
		results[i].TaskID = task.ID
	}

	// The orchestrator models dependencies between subtasks inside a single
	// task, so all multiagent tasks are registered as one orchestrator task.
	// This lets deps reference other task IDs while still using NextReady and
	// MarkDone/MarkFailed for scheduling.
	o := orchestrator.New()
	subtasks := make([]orchestrator.SubTask, 0, len(tasks))
	for _, task := range tasks {
		depends := append([]string(nil), task.Dependencies...)
		depends = append(depends, deps[task.ID]...)
		subtasks = append(subtasks, orchestrator.SubTask{
			ID:        task.ID,
			DependsOn: depends,
		})
	}
	if err := o.AddTask(orchestrator.Task{ID: "multiagent", SubTasks: subtasks}); err != nil {
		return nil, fmt.Errorf("multiagent: add tasks: %w", err)
	}

	sem := make(chan struct{}, maxConcurrent)
	done := make(chan struct{})
	executed := make(map[string]bool, len(tasks))
	var running int

	for {
		st := o.NextReady()
		if st != nil {
			running++
			idx := indexByID[st.ID]
			task := tasks[idx]
			executed[task.ID] = true
			go func(st *orchestrator.SubTask, task Task, idx int) {
				sem <- struct{}{}
				defer func() {
					<-sem
					done <- struct{}{}
				}()

				r.emit(Event{Type: EventTaskStarted, TaskID: task.ID})
				output, err := r.Execute(ctx, task)
				results[idx].Output = output
				results[idx].Err = err
				if err != nil {
					r.emit(Event{Type: EventTaskFailed, TaskID: task.ID, Output: output, Err: err})
					_ = o.MarkFailed(st.ID)
				} else {
					r.emit(Event{Type: EventTaskDone, TaskID: task.ID, Output: output})
					_ = o.MarkDone(st.ID)
				}
			}(st, task, idx)
			continue
		}

		if running == 0 {
			break
		}
		<-done
		running--
	}

	if o.Status() == orchestrator.TaskStatusFailed {
		for i, task := range tasks {
			if !executed[task.ID] && results[i].Err == nil {
				results[i].Err = errors.New("multiagent: task skipped because a dependency failed")
			}
		}
		return results, errors.New("multiagent: one or more tasks failed")
	}
	return results, nil
}

func (r *Runner) emit(ev Event) {
	if r != nil && r.OnEvent != nil {
		r.OnEvent(ev)
	}
}
