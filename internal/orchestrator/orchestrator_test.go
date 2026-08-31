package orchestrator

import (
	"testing"

	"github.com/chronosvx/agentforge/internal/agent"
)

func TestTaskSplitAndBasicExecution(t *testing.T) {
	o := New()
	task := Task{
		ID:   "task-1",
		Name: "实现多 Agent 协作",
		SubTasks: []SubTask{
			{ID: "s1", AgentID: "agent-a", Action: agent.Action{Type: agent.ActionTypeEditFile, Path: "a.go"}},
			{ID: "s2", AgentID: "agent-b", Action: agent.Action{Type: agent.ActionTypeRunCommand, Command: "go test ./..."}},
		},
	}
	if err := o.AddTask(task); err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	st := o.NextReady("task-1")
	if st == nil {
		t.Fatal("NextReady() = nil, want first subtask")
	}
	if st.ID != "s1" {
		t.Fatalf("NextReady() ID = %q, want %q", st.ID, "s1")
	}
	if st.Status != StatusRunning {
		t.Fatalf("returned subtask Status = %q, want %q", st.Status, StatusRunning)
	}
	if st.AgentID != "agent-a" {
		t.Fatalf("AgentID = %q, want agent-a", st.AgentID)
	}

	if err := o.MarkDone("task-1", "s1"); err != nil {
		t.Fatalf("MarkDone() error: %v", err)
	}

	st = o.NextReady("task-1")
	if st == nil || st.ID != "s2" {
		t.Fatalf("second NextReady() = %+v, want s2", st)
	}
	if err := o.MarkDone("task-1", "s2"); err != nil {
		t.Fatalf("MarkDone(s2) error: %v", err)
	}

	if got := o.Status("task-1"); got != TaskStatusDone {
		t.Fatalf("Status() = %q, want %q", got, TaskStatusDone)
	}
}

func TestDependencyOrdering(t *testing.T) {
	o := New()
	err := o.AddTask(Task{
		ID: "chain",
		SubTasks: []SubTask{
			{ID: "a", DependsOn: nil},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: []string{"b"}},
		},
	})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	got := nextIDs(t, o, "chain", 3)
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestParallelReady(t *testing.T) {
	o := New()
	err := o.AddTask(Task{
		ID: "parallel",
		SubTasks: []SubTask{
			{ID: "p1"},
			{ID: "p2"},
			{ID: "p3", DependsOn: []string{"p1"}},
		},
	})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	first := o.NextReady("parallel")
	if first == nil || first.ID != "p1" {
		t.Fatalf("first NextReady() = %+v, want p1", first)
	}
	second := o.NextReady("parallel")
	if second == nil || second.ID != "p2" {
		t.Fatalf("second NextReady() = %+v, want p2 (parallel ready before p1 done)", second)
	}
	if third := o.NextReady("parallel"); third != nil {
		t.Fatalf("third NextReady() = %+v, want nil because p3 depends on p1 and p1 is not done", third)
	}

	if err := o.MarkDone("parallel", "p1"); err != nil {
		t.Fatalf("MarkDone(p1) error: %v", err)
	}
	third := o.NextReady("parallel")
	if third == nil || third.ID != "p3" {
		t.Fatalf("third NextReady() after p1 done = %+v, want p3", third)
	}
}

func TestFailurePropagation(t *testing.T) {
	o := New()
	err := o.AddTask(Task{
		ID: "fail-chain",
		SubTasks: []SubTask{
			{ID: "a"},
			{ID: "b", DependsOn: []string{"a"}},
			{ID: "c", DependsOn: []string{"b"}},
			{ID: "independent"},
		},
	})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	// a 与 independent 可并行就绪；先执行 a 并让其失败。
	first := o.NextReady("fail-chain")
	if first == nil || first.ID != "a" {
		t.Fatalf("NextReady() = %+v, want a", first)
	}
	if err := o.MarkFailed("fail-chain", "a"); err != nil {
		t.Fatalf("MarkFailed(a) error: %v", err)
	}

	// b 依赖 a，a 失败后 b 不应就绪；c 也随 b 级联失败。
	// 与失败链无关的 independent 仍可继续并行执行。
	independent := o.NextReady("fail-chain")
	if independent == nil || independent.ID != "independent" {
		t.Fatalf("NextReady() after a failed = %+v, want independent", independent)
	}
	if err := o.MarkDone("fail-chain", "independent"); err != nil {
		t.Fatalf("MarkDone(independent) error: %v", err)
	}
	if got := o.NextReady("fail-chain"); got != nil {
		t.Fatalf("NextReady() after all available work = %+v, want nil", got)
	}
	if got := o.Status("fail-chain"); got != TaskStatusFailed {
		t.Fatalf("Status() = %q, want %q", got, TaskStatusFailed)
	}

	sum := o.Summary("fail-chain")
	if sum.Failed != 3 {
		t.Fatalf("Summary.Failed = %d, want 3 (a, b, c)", sum.Failed)
	}
	if sum.Done != 1 {
		t.Fatalf("Summary.Done = %d, want 1 (independent)", sum.Done)
	}
	if sum.Pending != 0 {
		t.Fatalf("Summary.Pending = %d, want 0", sum.Pending)
	}
}

func TestSummary(t *testing.T) {
	o := New()
	err := o.AddTask(Task{
		ID: "summary",
		SubTasks: []SubTask{
			{ID: "done1"},
			{ID: "running"},
			{ID: "pending"},
			{ID: "failed"},
		},
	})
	if err != nil {
		t.Fatalf("AddTask() error: %v", err)
	}

	if err := o.MarkDone("summary", "done1"); err != nil {
		t.Fatalf("MarkDone(done1) error: %v", err)
	}
	if st := o.NextReady("summary"); st == nil || st.ID != "running" {
		t.Fatalf("NextReady() = %+v, want running", st)
	}
	if err := o.MarkFailed("summary", "failed"); err != nil {
		t.Fatalf("MarkFailed(failed) error: %v", err)
	}

	sum := o.Summary("summary")
	if sum.Total != 4 || sum.Done != 1 || sum.Running != 1 || sum.Pending != 1 || sum.Failed != 1 {
		t.Fatalf("Summary = %+v, want total=4 done=1 running=1 pending=1 failed=1", sum)
	}
	if sum.Status != TaskStatusFailed {
		t.Fatalf("Summary.Status = %q, want %q", sum.Status, TaskStatusFailed)
	}
}

func TestAddTaskValidation(t *testing.T) {
	o := New()
	if err := o.AddTask(Task{ID: "dup", SubTasks: []SubTask{{ID: "x"}}}); err != nil {
		t.Fatalf("AddTask(first) error: %v", err)
	}
	if err := o.AddTask(Task{ID: "dup", SubTasks: []SubTask{{ID: "y"}}}); err == nil {
		t.Fatal("AddTask(duplicate) error = nil, want error")
	}
	if err := o.AddTask(Task{ID: "bad-dep", SubTasks: []SubTask{{ID: "x", DependsOn: []string{"missing"}}}}); err == nil {
		t.Fatal("AddTask(unknown dep) error = nil, want error")
	}
	if err := o.AddTask(Task{ID: "cycle", SubTasks: []SubTask{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}}); err == nil {
		t.Fatal("AddTask(cycle) error = nil, want error")
	}
}

func nextIDs(t *testing.T, o *Orchestrator, taskID string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		st := o.NextReady(taskID)
		if st == nil {
			t.Fatalf("NextReady() = nil at step %d, want %d subtasks", i, n)
		}
		ids = append(ids, st.ID)
		if err := o.MarkDone(taskID, st.ID); err != nil {
			t.Fatalf("MarkDone(%s) error: %v", st.ID, err)
		}
	}
	if extra := o.NextReady(taskID); extra != nil {
		t.Fatalf("NextReady() after %d subtasks = %+v, want nil", n, extra)
	}
	return ids
}
