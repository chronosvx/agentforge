// Package agent 提供 Agent 执行闭环的后端状态机。
//
// 该包把“计划 -> 审批 -> 执行 -> 测试 -> Diff -> 回滚”建模为可编程的
// AgentSession，不依赖 UI，后续可由 TUI、CLI 或服务端复用。
package agent

import (
	"fmt"
	"sync"
)

// Phase 表示 Agent 会话的顶层状态。
type Phase string

const (
	PhasePlanning         Phase = "Planning"
	PhaseAwaitingApproval Phase = "AwaitingApproval"
	PhaseExecuting        Phase = "Executing"
	PhaseTesting          Phase = "Testing"
	PhaseReviewingDiff    Phase = "ReviewingDiff"
	PhaseRollingBack      Phase = "RollingBack"
	PhaseDone             Phase = "Done"
	PhaseFailed           Phase = "Failed"
)

// 简短别名，便于按领域名称引用。
const (
	Planning         Phase = PhasePlanning
	AwaitingApproval Phase = PhaseAwaitingApproval
	Executing        Phase = PhaseExecuting
	Testing          Phase = PhaseTesting
	ReviewingDiff    Phase = PhaseReviewingDiff
	RollingBack      Phase = PhaseRollingBack
	Done             Phase = PhaseDone
	Failed           Phase = PhaseFailed
)

// ActionType 表示 Agent 可以执行的动作类型。
type ActionType string

const (
	ActionTypeEditFile   ActionType = "edit_file"
	ActionTypeRunCommand ActionType = "run_command"
	ActionTypeApplyDiff  ActionType = "apply_diff"
	ActionTypeRollback   ActionType = "rollback"
)

// 简短别名，便于按领域名称引用。
const (
	EditFile   ActionType = ActionTypeEditFile
	RunCommand ActionType = ActionTypeRunCommand
	ApplyDiff  ActionType = ActionTypeApplyDiff
	Rollback   ActionType = ActionTypeRollback
)

// ApprovalPolicy 控制动作是否需要人工审批。
type ApprovalPolicy int

const (
	// AllowAll 表示允许的动作直接执行，不需要审批。
	AllowAll ApprovalPolicy = iota
	// RequireApproval 表示允许的动作都需要先经过审批。
	RequireApproval
	// DenyAll 表示拒绝所有动作。
	DenyAll
)

// ActionStatus 表示单个动作在会话中的流转状态。
type ActionStatus string

const (
	ActionStatusPending    ActionStatus = "pending"
	ActionStatusApproved   ActionStatus = "approved"
	ActionStatusRejected   ActionStatus = "rejected"
	ActionStatusExecuted   ActionStatus = "executed"
	ActionStatusTested     ActionStatus = "tested"
	ActionStatusRolledBack ActionStatus = "rolled_back"
)

// RollbackState 表示会话级回滚状态。
type RollbackState string

const (
	RollbackNone      RollbackState = "none"
	RollbackRequested RollbackState = "requested"
	RollbackCompleted RollbackState = "completed"
)

// Action 是 Agent 计划中的一个可执行动作。
//
// 除 Type/Description/Command/Path/Content 等计划信息外，Action 也携带
// 运行时状态（Status、Output、TestOK、回滚标记），便于上层直接读取。
type Action struct {
	ID          string
	Type        ActionType
	Description string
	Command     string
	Path        string
	Content     string

	Status            ActionStatus
	Output            string
	TestOK            bool
	RollbackRequested bool
	RolledBack        bool
}

// AgentSession 管理一个 Agent 执行闭环的全部状态。
//
// 它持有动作列表、审批队列、执行/测试结果、Diff 与回滚状态。所有公开方法
// 均加锁，可安全地被 UI 或后台执行器并发调用。
type AgentSession struct {
	mu sync.Mutex

	Phase         Phase
	Actions       []Action
	ApprovalQueue []string
	Policy        ApprovalPolicy
	Permissions   *PermissionSet
	Diff          string
	RollbackState RollbackState
}

// NewAgentSession 创建一个新的 Agent 会话，初始阶段为 Planning。
// 未指定策略时默认 AllowAll。
func NewAgentSession(policies ...ApprovalPolicy) *AgentSession {
	policy := AllowAll
	if len(policies) > 0 {
		policy = policies[0]
	}
	return &AgentSession{
		Phase:         PhasePlanning,
		Policy:        policy,
		RollbackState: RollbackNone,
	}
}

// SetPermissionSet 设置该会话使用的权限集。nil 表示不附加权限规则。
func (s *AgentSession) SetPermissionSet(ps *PermissionSet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Permissions = ps
}

// Plan 载入一批计划动作，并重置会话的审批/执行/测试/回滚状态。
//
// 动作 ID 为空时会按顺序生成 action-0、action-1……。阶段会被重置为 Planning；
// 空计划直接进入 Done。
func (s *AgentSession) Plan(actions []Action) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Actions = make([]Action, 0, len(actions))
	s.ApprovalQueue = nil
	s.Diff = ""
	s.RollbackState = RollbackNone

	for i := range actions {
		a := actions[i]
		if a.ID == "" {
			a.ID = fmt.Sprintf("action-%d", i)
		}
		a.Status = ActionStatusPending
		a.Output = ""
		a.TestOK = false
		a.RollbackRequested = false
		a.RolledBack = false
		s.Actions = append(s.Actions, a)
	}

	if len(s.Actions) == 0 {
		s.Phase = PhaseDone
		return
	}
	s.Phase = PhasePlanning
}

// NextPendingAction 返回下一个待处理动作。
//
// 如果下一个动作需要审批，会话进入 AwaitingApproval 并把动作加入审批队列；
// 否则进入 Executing。没有待处理动作时返回 nil，并自动收敛到
// Testing/ReviewingDiff/Done/Failed 等阶段。
func (s *AgentSession) NextPendingAction() *Action {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == PhaseRollingBack || s.Phase == PhaseDone || s.Phase == PhaseFailed {
		return nil
	}

	for i := range s.Actions {
		a := &s.Actions[i]
		if a.Status == ActionStatusApproved {
			s.Phase = PhaseExecuting
			return cloneAction(a)
		}
		if a.Status != ActionStatusPending {
			continue
		}

		allowed, requireApproval := s.checkActionLocked(a)
		if !allowed {
			a.Status = ActionStatusRejected
			s.Phase = PhaseFailed
			return nil
		}

		if requireApproval {
			s.Phase = PhaseAwaitingApproval
			s.enqueueApprovalLocked(a.ID)
			return cloneAction(a)
		}

		s.Phase = PhaseExecuting
		return cloneAction(a)
	}

	s.refreshNoPendingLocked()
	return nil
}

// Approve 批准一个等待审批的动作。批准后该动作可被 NextPendingAction 取出执行。
func (s *AgentSession) Approve(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == PhaseDone || s.Phase == PhaseFailed || s.Phase == PhaseRollingBack {
		return fmt.Errorf("agent: cannot approve in phase %s", s.Phase)
	}

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.Status != ActionStatusPending {
		return fmt.Errorf("agent: action %q cannot be approved in status %s", id, a.Status)
	}
	allowed, _ := s.checkActionLocked(a)
	if !allowed {
		return fmt.Errorf("agent: action %q is not allowed by policy or permissions", id)
	}

	a.Status = ActionStatusApproved
	s.removeFromApprovalQueueLocked(id)
	s.Phase = PhaseExecuting
	return nil
}

// Reject 拒绝一个等待审批的动作，并将会话置为 Failed。
func (s *AgentSession) Reject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == PhaseDone || s.Phase == PhaseFailed || s.Phase == PhaseRollingBack {
		return fmt.Errorf("agent: cannot reject in phase %s", s.Phase)
	}

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.Status != ActionStatusPending {
		return fmt.Errorf("agent: action %q cannot be rejected in status %s", id, a.Status)
	}

	a.Status = ActionStatusRejected
	s.removeFromApprovalQueueLocked(id)
	s.Phase = PhaseFailed
	return nil
}

// MarkExecuted 标记一个动作已执行，并记录执行输出。
func (s *AgentSession) MarkExecuted(id, output string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == PhaseDone || s.Phase == PhaseFailed || s.Phase == PhaseRollingBack {
		return fmt.Errorf("agent: cannot mark executed in phase %s", s.Phase)
	}

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.Status != ActionStatusPending && a.Status != ActionStatusApproved {
		return fmt.Errorf("agent: action %q cannot be marked executed in status %s", id, a.Status)
	}

	a.Status = ActionStatusExecuted
	a.Output = output

	s.refreshAfterActionLocked()
	return nil
}

// MarkTested 标记一个已执行动作的测试结果。
//
// ok 为 true 时继续闭环；ok 为 false 时会话进入 Failed，可由上层调用
// RequestRollback 进入回滚流程。
func (s *AgentSession) MarkTested(id string, ok bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == PhaseDone || s.Phase == PhaseFailed || s.Phase == PhaseRollingBack {
		return fmt.Errorf("agent: cannot mark tested in phase %s", s.Phase)
	}

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.Status != ActionStatusExecuted {
		return fmt.Errorf("agent: action %q cannot be marked tested in status %s", id, a.Status)
	}

	a.Status = ActionStatusTested
	a.TestOK = ok

	if !ok {
		s.Phase = PhaseFailed
		return nil
	}

	s.refreshAfterActionLocked()
	return nil
}

// RequestRollback 请求对某个动作执行回滚，会话进入 RollingBack。
func (s *AgentSession) RequestRollback(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.RolledBack {
		return fmt.Errorf("agent: action %q has already been rolled back", id)
	}

	a.RollbackRequested = true
	s.RollbackState = RollbackRequested
	s.Phase = PhaseRollingBack
	return nil
}

// Rollback 标记指定动作已回滚，并将会话置为 Done。
func (s *AgentSession) Rollback(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	a := s.findActionLocked(id)
	if a == nil {
		return fmt.Errorf("agent: action %q not found", id)
	}
	if a.RolledBack {
		return fmt.Errorf("agent: action %q has already been rolled back", id)
	}

	a.RollbackRequested = true
	a.RolledBack = true
	a.Status = ActionStatusRolledBack
	s.RollbackState = RollbackCompleted
	s.Phase = PhaseDone
	return nil
}

// SetDiff 设置当前待评审的 Diff 内容。
func (s *AgentSession) SetDiff(diff string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Diff = diff
}

// PendingApprovals 返回当前等待审批的动作快照（按加入顺序）。
func (s *AgentSession) PendingApprovals() []Action {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Action, 0, len(s.ApprovalQueue))
	for _, id := range s.ApprovalQueue {
		if a := s.findActionLocked(id); a != nil {
			out = append(out, *a)
		}
	}
	return out
}

// checkActionLocked 综合 ApprovalPolicy 与 PermissionSet 判断动作是否允许，
// 以及是否要求审批。
func (s *AgentSession) checkActionLocked(a *Action) (allowed bool, requireApproval bool) {
	if s.Policy == DenyAll {
		return false, false
	}

	if s.Permissions != nil {
		allowed, ruleRequireApproval := s.Permissions.Check(*a)
		if !allowed {
			return false, false
		}
		if s.Policy == RequireApproval || ruleRequireApproval {
			return true, true
		}
		return true, false
	}

	if s.Policy == RequireApproval {
		return true, true
	}
	return true, false
}

// refreshAfterActionLocked 在动作被执行/测试后重新计算会话阶段。
func (s *AgentSession) refreshAfterActionLocked() {
	// 仍有待处理动作时，按下一个动作决定进入审批还是执行。
	for i := range s.Actions {
		a := &s.Actions[i]
		if a.Status == ActionStatusPending || a.Status == ActionStatusApproved {
			allowed, requireApproval := s.checkActionLocked(a)
			if !allowed {
				s.Phase = PhaseFailed
				return
			}
			if requireApproval && a.Status == ActionStatusPending {
				s.Phase = PhaseAwaitingApproval
				s.enqueueApprovalLocked(a.ID)
				return
			}
			s.Phase = PhaseExecuting
			return
		}
	}

	// 没有待处理动作时，优先进入测试阶段。
	for i := range s.Actions {
		a := &s.Actions[i]
		if a.Status == ActionStatusExecuted {
			s.Phase = PhaseTesting
			return
		}
	}

	// 全部已测试且通过时进入 Diff 评审；否则按失败/完成处理。
	s.refreshNoPendingLocked()
}

// refreshNoPendingLocked 在没有待处理动作时收敛阶段。
func (s *AgentSession) refreshNoPendingLocked() {
	if s.Phase == PhaseRollingBack || s.Phase == PhaseDone || s.Phase == PhaseFailed {
		return
	}

	hasRejected := false
	hasExecuted := false
	hasFailedTest := false
	hasTested := false
	hasRolledBack := false

	for _, a := range s.Actions {
		switch a.Status {
		case ActionStatusRejected:
			hasRejected = true
		case ActionStatusExecuted:
			hasExecuted = true
		case ActionStatusTested:
			hasTested = true
			if !a.TestOK {
				hasFailedTest = true
			}
		case ActionStatusRolledBack:
			hasRolledBack = true
		}
	}

	if hasRejected || hasFailedTest {
		s.Phase = PhaseFailed
		return
	}
	if hasExecuted {
		s.Phase = PhaseTesting
		return
	}
	if hasTested || hasRolledBack || len(s.Actions) > 0 {
		s.Phase = PhaseReviewingDiff
		return
	}
	s.Phase = PhaseDone
}

func (s *AgentSession) findActionLocked(id string) *Action {
	for i := range s.Actions {
		if s.Actions[i].ID == id {
			return &s.Actions[i]
		}
	}
	return nil
}

func (s *AgentSession) enqueueApprovalLocked(id string) {
	for _, existing := range s.ApprovalQueue {
		if existing == id {
			return
		}
	}
	s.ApprovalQueue = append(s.ApprovalQueue, id)
}

func (s *AgentSession) removeFromApprovalQueueLocked(id string) {
	out := s.ApprovalQueue[:0]
	for _, existing := range s.ApprovalQueue {
		if existing != id {
			out = append(out, existing)
		}
	}
	s.ApprovalQueue = out
}

func cloneAction(a *Action) *Action {
	if a == nil {
		return nil
	}
	copy := *a
	return &copy
}
