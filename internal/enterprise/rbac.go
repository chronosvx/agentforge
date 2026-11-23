// Package enterprise 提供企业部署的最小基础能力：RBAC、SSO、KMS 与 SIEM。
//
// 当前实现以可扩展接口和本地简单实现为主，后续可替换为云 SSO、云 KMS
// 或集中式 SIEM，而不影响调用方接口。
package enterprise

import (
	"fmt"
	"sort"
	"strings"
)

// Role 表示企业环境中的用户角色。
type Role string

const (
	// RoleAdmin 拥有全部权限。
	RoleAdmin Role = "admin"
	// RoleMember 是普通成员，可执行日常业务操作。
	RoleMember Role = "member"
	// RoleAuditor 面向审计人员，以只读与审计操作为主。
	RoleAuditor Role = "auditor"
	// RoleGuest 是最小权限访客角色。
	RoleGuest Role = "guest"
)

// KnownRoles 返回当前已知的角色列表，便于配置校验或界面展示。
func KnownRoles() []Role {
	return []Role{RoleAdmin, RoleMember, RoleAuditor, RoleGuest}
}

// Policy 是基于角色的权限策略表。
//
// 每个角色可允许一组 action。角色为 admin 时默认允许全部 action，
// 也可通过 Allow(RoleAdmin, "*") 显式表达。
type Policy struct {
	permissions map[Role]map[string]struct{}
}

// NewPolicy 创建空策略；默认不授予任何权限。
func NewPolicy() *Policy {
	return &Policy{permissions: make(map[Role]map[string]struct{})}
}

// DefaultPolicy 返回适合企业最小基础能力的默认策略。
//
// 默认规则：
//   - admin：全部动作（*）
//   - member：会话读写、协作评论/审批、Agent/命令执行、成本查看
//   - auditor：审计日志、SIEM、会话与协作只读
//   - guest：会话只读
func DefaultPolicy() *Policy {
	p := NewPolicy()
	p.Allow(RoleAdmin, "*")
	p.Allow(RoleMember,
		"session.read",
		"session.write",
		"collab.read",
		"collab.comment",
		"collab.approve",
		"agent.run",
		"exec.run",
		"cost.view",
	)
	p.Allow(RoleAuditor,
		"session.read",
		"collab.read",
		"audit.read",
		"siem.read",
	)
	p.Allow(RoleGuest, "session.read")
	return p
}

// Allow 为角色授予一个或多个 action 权限。
//
// action 使用 "*" 表示该角色的全部权限。
func (p *Policy) Allow(role Role, actions ...string) {
	if p == nil {
		return
	}
	if p.permissions == nil {
		p.permissions = make(map[Role]map[string]struct{})
	}

	perms := p.permissions[role]
	if perms == nil {
		perms = make(map[string]struct{})
		p.permissions[role] = perms
	}
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action != "" {
			perms[action] = struct{}{}
		}
	}
}

// Can 判断指定角色是否可执行 action。
func (p *Policy) Can(role Role, action string) bool {
	if p == nil || p.permissions == nil {
		return false
	}
	perms, ok := p.permissions[role]
	if !ok {
		return false
	}
	if _, ok := perms["*"]; ok {
		return true
	}
	_, ok = perms[action]
	return ok
}

// Actions 返回指定角色当前被授予的 action 列表（不含 "*"）。
func (p *Policy) Actions(role Role) []string {
	if p == nil || p.permissions == nil {
		return nil
	}
	perms := p.permissions[role]
	out := make([]string, 0, len(perms))
	for action := range perms {
		if action != "*" {
			out = append(out, action)
		}
	}
	sort.Strings(out)
	return out
}

// RBAC 将 Policy 与用户角色结合，提供面向用户的权限检查。
type RBAC struct {
	Policy *Policy
}

// NewRBAC 创建 RBAC 检查器；policy 为空时使用 DefaultPolicy。
func NewRBAC(policy *Policy) *RBAC {
	if policy == nil {
		policy = DefaultPolicy()
	}
	return &RBAC{Policy: policy}
}

// Check 判断用户是否可执行 action。
//
// 用户拥有多个角色时，任一角色允许即返回 true。
func (r *RBAC) Check(user *User, action string) bool {
	if r == nil || r.Policy == nil || user == nil {
		return false
	}
	for _, role := range user.Roles {
		if r.Policy.Can(role, action) {
			return true
		}
	}
	return false
}

// CheckRole 直接按角色判断是否可执行 action，便于服务端内部校验。
func (r *RBAC) CheckRole(role Role, action string) bool {
	if r == nil || r.Policy == nil {
		return false
	}
	return r.Policy.Can(role, action)
}

// String 返回角色的可读名称。
func (r Role) String() string {
	return string(r)
}

// ParseRole 将字符串解析为 Role，未知角色返回错误。
func ParseRole(s string) (Role, error) {
	role := Role(strings.TrimSpace(s))
	switch role {
	case RoleAdmin, RoleMember, RoleAuditor, RoleGuest:
		return role, nil
	default:
		return "", fmt.Errorf("enterprise: unknown role %q", s)
	}
}
