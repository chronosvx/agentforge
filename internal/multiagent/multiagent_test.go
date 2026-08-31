package multiagent

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type recordingExecutor struct {
	mu    sync.Mutex
	calls []string
}

func (e *recordingExecutor) execute(_ context.Context, task Task) (string, error) {
	e.mu.Lock()
	e.calls = append(e.calls, task.ID)
	e.mu.Unlock()
	return "ok " + task.ID, nil
}

func (e *recordingExecutor) order() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.calls...)
}

func TestRunIndependentTasks(t *testing.T) {
	exec := &recordingExecutor{}
	r := NewRunner(exec.execute)

	tasks := []Task{
		{ID: "a", Prompt: "A"},
		{ID: "b", Prompt: "B"},
		{ID: "c", Prompt: "C"},
	}

	results, err := r.Run(context.Background(), tasks, nil)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(results) != len(tasks) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(tasks))
	}
	for i, want := range []string{"a", "b", "c"} {
		if results[i].TaskID != want {
			t.Errorf("results[%d].TaskID = %q, want %q", i, results[i].TaskID, want)
		}
		if results[i].Output != "ok "+want {
			t.Errorf("results[%d].Output = %q, want %q", i, results[i].Output, "ok "+want)
		}
		if results[i].Err != nil {
			t.Errorf("results[%d].Err = %v, want nil", i, results[i].Err)
		}
	}
	if got := exec.order(); len(got) != 3 {
		t.Fatalf("executed %d tasks, want 3: %v", len(got), got)
	}
}

func TestRunDependencyChain(t *testing.T) {
	exec := &recordingExecutor{}
	r := NewRunner(exec.execute)

	tasks := []Task{
		{ID: "a", Prompt: "A"},
		{ID: "b", Prompt: "B"},
	}
	deps := map[string][]string{
		"b": {"a"},
	}

	results, err := r.Run(context.Background(), tasks, deps)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	order := exec.order()
	if len(order) != 2 {
		t.Fatalf("executed %d tasks, want 2: %v", len(order), order)
	}
	if order[0] != "a" || order[1] != "b" {
		t.Fatalf("execution order = %v, want [a b]", order)
	}

	if results[0].TaskID != "a" || results[0].Output != "ok a" {
		t.Errorf("results[0] = %+v, want task a with output %q", results[0], "ok a")
	}
	if results[1].TaskID != "b" || results[1].Output != "ok b" {
		t.Errorf("results[1] = %+v, want task b with output %q", results[1], "ok b")
	}
}

func TestRunParallelDependencies(t *testing.T) {
	exec := &recordingExecutor{}
	r := NewRunner(exec.execute)

	tasks := []Task{
		{ID: "a"},
		{ID: "b"},
		{ID: "c", Prompt: "depends on a and b"},
	}
	deps := map[string][]string{
		"c": {"a", "b"},
	}

	results, err := r.Run(context.Background(), tasks, deps)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}

	order := exec.order()
	if len(order) != 3 {
		t.Fatalf("executed %d tasks, want 3: %v", len(order), order)
	}
	if order[0] == "c" || order[1] == "c" {
		t.Fatalf("task c ran before its dependencies: %v", order)
	}
	if results[2].TaskID != "c" || results[2].Output != "ok c" {
		t.Errorf("results[2] = %+v, want task c with output %q", results[2], "ok c")
	}
}

func TestRunFailureSetsResultErrAndReturnsError(t *testing.T) {
	execErr := errors.New("boom")
	r := NewRunner(func(_ context.Context, task Task) (string, error) {
		return "", execErr
	})

	tasks := []Task{{ID: "a"}, {ID: "b", Prompt: "depends on a"}}
	deps := map[string][]string{"b": {"a"}}

	results, err := r.Run(context.Background(), tasks, deps)
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil because a task failed")
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Err == nil {
		t.Errorf("results[0].Err = nil, want %v", execErr)
	}
	if results[1].Err == nil {
		t.Errorf("results[1].Err = nil, want non-nil because b was skipped after a failed")
	}
}
