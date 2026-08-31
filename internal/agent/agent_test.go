package agent

import "testing"

func TestPlanEmptyGoesDone(t *testing.T) {
	s := NewAgentSession(AllowAll)
	s.Plan(nil)
	if s.Phase != PhaseDone {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseDone)
	}
}

func TestStateMachineWithApproval(t *testing.T) {
	s := NewAgentSession(RequireApproval)
	s.Plan([]Action{
		{Type: ActionTypeEditFile, Path: "a.txt", Content: "hello"},
	})

	if s.Phase != PhasePlanning {
		t.Fatalf("after Plan Phase = %q, want %q", s.Phase, PhasePlanning)
	}

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want first action")
	}
	if s.Phase != PhaseAwaitingApproval {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseAwaitingApproval)
	}
	if len(s.PendingApprovals()) != 1 || s.PendingApprovals()[0].ID != a.ID {
		t.Fatalf("PendingApprovals() = %+v, want one action %q", s.PendingApprovals(), a.ID)
	}

	if err := s.Approve(a.ID); err != nil {
		t.Fatalf("Approve() error: %v", err)
	}
	if s.Phase != PhaseExecuting {
		t.Fatalf("after Approve Phase = %q, want %q", s.Phase, PhaseExecuting)
	}
	if len(s.PendingApprovals()) != 0 {
		t.Fatalf("PendingApprovals() = %+v, want empty after approve", s.PendingApprovals())
	}

	a = s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() after approve = nil, want approved action")
	}
	if a.Status != ActionStatusApproved {
		t.Fatalf("action status = %q, want %q", a.Status, ActionStatusApproved)
	}
	if s.Phase != PhaseExecuting {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseExecuting)
	}

	if err := s.MarkExecuted(a.ID, "changed a.txt"); err != nil {
		t.Fatalf("MarkExecuted() error: %v", err)
	}
	if s.Phase != PhaseTesting {
		t.Fatalf("after MarkExecuted Phase = %q, want %q", s.Phase, PhaseTesting)
	}

	if err := s.MarkTested(a.ID, true); err != nil {
		t.Fatalf("MarkTested() error: %v", err)
	}
	if s.Phase != PhaseReviewingDiff {
		t.Fatalf("after MarkTested Phase = %q, want %q", s.Phase, PhaseReviewingDiff)
	}

	if s.Actions[0].Output != "changed a.txt" {
		t.Fatalf("Output = %q, want %q", s.Actions[0].Output, "changed a.txt")
	}
	if s.Actions[0].Status != ActionStatusTested {
		t.Fatalf("Status = %q, want %q", s.Actions[0].Status, ActionStatusTested)
	}
	if !s.Actions[0].TestOK {
		t.Fatal("TestOK = false, want true")
	}
}

func TestAllowAllSkipsApproval(t *testing.T) {
	s := NewAgentSession(AllowAll)
	s.Plan([]Action{{Type: ActionTypeRunCommand, Command: "go test ./..."}})

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if s.Phase != PhaseExecuting {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseExecuting)
	}
	if len(s.PendingApprovals()) != 0 {
		t.Fatalf("PendingApprovals() = %+v, want empty under AllowAll", s.PendingApprovals())
	}
}

func TestRejectMarksFailed(t *testing.T) {
	s := NewAgentSession(RequireApproval)
	s.Plan([]Action{{Type: ActionTypeEditFile, Path: "x.go"}})

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if err := s.Reject(a.ID); err != nil {
		t.Fatalf("Reject() error: %v", err)
	}
	if s.Phase != PhaseFailed {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseFailed)
	}
	if len(s.PendingApprovals()) != 0 {
		t.Fatalf("PendingApprovals() = %+v, want empty after reject", s.PendingApprovals())
	}
	if s.Actions[0].Status != ActionStatusRejected {
		t.Fatalf("Status = %q, want %q", s.Actions[0].Status, ActionStatusRejected)
	}
}

func TestDenyAllRejectsOnNextPending(t *testing.T) {
	s := NewAgentSession(DenyAll)
	s.Plan([]Action{{Type: ActionTypeRunCommand, Command: "rm -rf /tmp/x"}})

	if a := s.NextPendingAction(); a != nil {
		t.Fatalf("NextPendingAction() = %+v, want nil under DenyAll", a)
	}
	if s.Phase != PhaseFailed {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseFailed)
	}
	if s.Actions[0].Status != ActionStatusRejected {
		t.Fatalf("Status = %q, want %q", s.Actions[0].Status, ActionStatusRejected)
	}
}

func TestMultipleActionsContinueAfterTest(t *testing.T) {
	s := NewAgentSession(AllowAll)
	s.Plan([]Action{
		{ID: "a1", Type: ActionTypeEditFile, Path: "one.txt"},
		{ID: "a2", Type: ActionTypeRunCommand, Command: "go test ./..."},
	})

	a1 := s.NextPendingAction()
	if a1 == nil || a1.ID != "a1" {
		t.Fatalf("first NextPendingAction() = %+v, want a1", a1)
	}
	if err := s.MarkExecuted(a1.ID, "ok"); err != nil {
		t.Fatalf("MarkExecuted(a1) error: %v", err)
	}
	if err := s.MarkTested(a1.ID, true); err != nil {
		t.Fatalf("MarkTested(a1) error: %v", err)
	}
	if s.Phase != PhaseExecuting {
		t.Fatalf("after testing a1 Phase = %q, want %q", s.Phase, PhaseExecuting)
	}

	a2 := s.NextPendingAction()
	if a2 == nil || a2.ID != "a2" {
		t.Fatalf("second NextPendingAction() = %+v, want a2", a2)
	}
	if err := s.MarkExecuted(a2.ID, "all pass"); err != nil {
		t.Fatalf("MarkExecuted(a2) error: %v", err)
	}
	if err := s.MarkTested(a2.ID, true); err != nil {
		t.Fatalf("MarkTested(a2) error: %v", err)
	}
	if s.Phase != PhaseReviewingDiff {
		t.Fatalf("final Phase = %q, want %q", s.Phase, PhaseReviewingDiff)
	}
}

func TestRollbackState(t *testing.T) {
	s := NewAgentSession(AllowAll)
	s.Plan([]Action{{Type: ActionTypeApplyDiff, Path: "change.patch"}})

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if err := s.MarkExecuted(a.ID, "applied"); err != nil {
		t.Fatalf("MarkExecuted() error: %v", err)
	}
	if err := s.MarkTested(a.ID, false); err != nil {
		t.Fatalf("MarkTested(false) error: %v", err)
	}
	if s.Phase != PhaseFailed {
		t.Fatalf("after failed test Phase = %q, want %q", s.Phase, PhaseFailed)
	}

	if err := s.RequestRollback(a.ID); err != nil {
		t.Fatalf("RequestRollback() error: %v", err)
	}
	if s.Phase != PhaseRollingBack {
		t.Fatalf("after RequestRollback Phase = %q, want %q", s.Phase, PhaseRollingBack)
	}
	if s.RollbackState != RollbackRequested {
		t.Fatalf("RollbackState = %q, want %q", s.RollbackState, RollbackRequested)
	}
	if !s.Actions[0].RollbackRequested {
		t.Fatal("RollbackRequested = false, want true")
	}
	if next := s.NextPendingAction(); next != nil {
		t.Fatalf("NextPendingAction() during rollback = %+v, want nil", next)
	}

	if err := s.Rollback(a.ID); err != nil {
		t.Fatalf("Rollback() error: %v", err)
	}
	if s.Phase != PhaseDone {
		t.Fatalf("after Rollback Phase = %q, want %q", s.Phase, PhaseDone)
	}
	if s.RollbackState != RollbackCompleted {
		t.Fatalf("RollbackState = %q, want %q", s.RollbackState, RollbackCompleted)
	}
	if !s.Actions[0].RolledBack || s.Actions[0].Status != ActionStatusRolledBack {
		t.Fatalf("action rollback state = %+v, want rolled back", s.Actions[0])
	}
}

func TestInvalidTransitionsReturnError(t *testing.T) {
	s := NewAgentSession(AllowAll)
	s.Plan([]Action{{ID: "only", Type: ActionTypeEditFile}})

	if err := s.Approve("missing"); err == nil {
		t.Fatal("Approve(missing) error = nil, want error")
	}
	if err := s.MarkExecuted("missing", ""); err == nil {
		t.Fatal("MarkExecuted(missing) error = nil, want error")
	}
	if err := s.MarkTested("only", true); err == nil {
		t.Fatal("MarkTested before execution error = nil, want error")
	}

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	if err := s.MarkExecuted(a.ID, "done"); err != nil {
		t.Fatalf("MarkExecuted() error: %v", err)
	}
	if err := s.MarkExecuted(a.ID, "again"); err == nil {
		t.Fatal("MarkExecuted twice error = nil, want error")
	}
}
