package agent

import "testing"

func TestPermissionSetDefaultDeny(t *testing.T) {
	ps := NewPermissionSet()
	allowed, requireApproval := ps.Check(Action{Type: ActionTypeEditFile})
	if allowed {
		t.Fatal("allowed = true, want default deny")
	}
	if requireApproval {
		t.Fatal("requireApproval = true, want false for default deny")
	}
}

func TestPermissionSetAllowsWithApproval(t *testing.T) {
	ps := NewPermissionSet()
	ps.AddRule(PermissionRule{
		ActionType:      ActionTypeEditFile,
		Allow:           true,
		RequireApproval: true,
	})

	allowed, requireApproval := ps.Check(Action{Type: ActionTypeEditFile})
	if !allowed {
		t.Fatal("allowed = false, want true")
	}
	if !requireApproval {
		t.Fatal("requireApproval = false, want true")
	}

	// 未配置的动作仍然默认拒绝。
	allowed, _ = ps.Check(Action{Type: ActionTypeRunCommand})
	if allowed {
		t.Fatal("run_command allowed = true, want default deny")
	}
}

func TestPermissionSetDenyRule(t *testing.T) {
	ps := NewPermissionSet()
	ps.AddRule(PermissionRule{
		ActionType: ActionTypeRunCommand,
		Allow:      false,
	})

	allowed, requireApproval := ps.Check(Action{Type: ActionTypeRunCommand})
	if allowed {
		t.Fatal("allowed = true, want false")
	}
	if requireApproval {
		t.Fatal("requireApproval = true, want false for deny rule")
	}
}

func TestPermissionSetDenyIgnoresRequireApproval(t *testing.T) {
	ps := NewPermissionSet()
	ps.AddRule(PermissionRule{
		ActionType:      ActionTypeRunCommand,
		Allow:           false,
		RequireApproval: true,
	})

	allowed, requireApproval := ps.Check(Action{Type: ActionTypeRunCommand})
	if allowed || requireApproval {
		t.Fatalf("Check() = (%v, %v), want (false, false) for deny rule", allowed, requireApproval)
	}
}

func TestPermissionSetFirstMatchWins(t *testing.T) {
	ps := NewPermissionSet()
	ps.AddRule(PermissionRule{ActionType: ActionTypeEditFile, Allow: true, RequireApproval: false})
	ps.AddRule(PermissionRule{ActionType: ActionTypeEditFile, Allow: false})

	allowed, requireApproval := ps.Check(Action{Type: ActionTypeEditFile})
	if !allowed || requireApproval {
		t.Fatalf("Check() = (%v, %v), want (true, false) from first match", allowed, requireApproval)
	}
}

func TestPermissionSetWithAgentSession(t *testing.T) {
	ps := NewPermissionSet()
	ps.AddRule(PermissionRule{
		ActionType:      ActionTypeEditFile,
		Allow:           true,
		RequireApproval: true,
	})

	s := NewAgentSession(AllowAll)
	s.SetPermissionSet(ps)
	s.Plan([]Action{{Type: ActionTypeEditFile, Path: "a.txt"}})

	a := s.NextPendingAction()
	if a == nil {
		t.Fatal("NextPendingAction() = nil, want action")
	}
	// 规则要求审批，即使 ApprovalPolicy 是 AllowAll 也进入审批队列。
	if s.Phase != PhaseAwaitingApproval {
		t.Fatalf("Phase = %q, want %q", s.Phase, PhaseAwaitingApproval)
	}
	if len(s.PendingApprovals()) != 1 {
		t.Fatalf("PendingApprovals() = %+v, want one", s.PendingApprovals())
	}
}
