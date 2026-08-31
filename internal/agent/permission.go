package agent

// PermissionRule 描述一类动作的权限规则。
//
// 默认拒绝：只有当存在匹配的规则且 Allow 为 true 时，动作才被允许。
type PermissionRule struct {
	ActionType      ActionType
	Allow           bool
	RequireApproval bool
}

// PermissionSet 是一组动作权限规则。
//
// Check 按规则加入顺序匹配第一个 ActionType 相同的规则；没有匹配规则时
// 返回默认拒绝（allowed=false, requireApproval=false）。
type PermissionSet struct {
	Rules []PermissionRule
}

// NewPermissionSet 创建一个权限集，所有动作默认拒绝。
// 可传入初始规则；不传时为空权限集。
func NewPermissionSet(rules ...PermissionRule) *PermissionSet {
	return &PermissionSet{Rules: rules}
}

// AddRule 添加一条权限规则。相同 ActionType 的规则按添加顺序匹配，
// 后添加的规则不会覆盖先添加的规则（首个匹配生效）。
func (s *PermissionSet) AddRule(rule PermissionRule) {
	if s == nil {
		return
	}
	s.Rules = append(s.Rules, rule)
}

// Check 检查一个动作是否被允许，以及是否需要审批。
func (s PermissionSet) Check(action Action) (allowed bool, requireApproval bool) {
	for _, rule := range s.Rules {
		if rule.ActionType == action.Type {
			if !rule.Allow {
				return false, false
			}
			return true, rule.RequireApproval
		}
	}
	return false, false
}
