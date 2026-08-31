package enterprise

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// User 表示通过身份提供商认证后的企业用户。
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Roles []Role `json:"roles"`
}

// IdentityProvider 是 SSO/身份认证的抽象接口。
//
// 后续可接入 OIDC、SAML、LDAP 或云身份提供商，只需实现 Authenticate。
type IdentityProvider interface {
	// Authenticate 使用认证 token 换取用户信息。
	Authenticate(ctx context.Context, token string) (*User, error)
}

// StaticProvider 是面向本地开发与测试的静态身份提供商。
//
// 它将 token 映射到本地用户表，便于在无外部 SSO 时验证 RBAC/审计链路。
type StaticProvider struct {
	mu    sync.RWMutex
	users map[string]*User // key 为 token
}

// NewStaticProvider 创建空的静态身份提供商。
//
// 也可传入初始用户表（token -> *User），便于在构造时直接加载本地配置。
func NewStaticProvider(initial ...map[string]*User) *StaticProvider {
	p := &StaticProvider{users: make(map[string]*User)}
	if len(initial) > 0 && initial[0] != nil {
		for token, user := range initial[0] {
			if user != nil {
				p.users[token] = cloneUser(user)
			}
		}
	}
	return p
}

// AddUser 注册一个 token 到用户的映射。
//
// token 不能为空；user 不能为 nil，且必须包含 ID。
func (p *StaticProvider) AddUser(token string, user *User) error {
	if p == nil {
		return errors.New("enterprise: static provider is nil")
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("enterprise: static provider token is empty")
	}
	if user == nil {
		return errors.New("enterprise: static provider user is nil")
	}
	if strings.TrimSpace(user.ID) == "" {
		return errors.New("enterprise: static provider user id is empty")
	}

	clone := cloneUser(user)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.users[token] = clone
	return nil
}

// RemoveUser 删除指定 token 对应的用户映射。
func (p *StaticProvider) RemoveUser(token string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.users, token)
}

// Authenticate 根据 token 返回用户副本。
//
// 未找到 token 时返回 ErrIdentityNotFound。
func (p *StaticProvider) Authenticate(ctx context.Context, token string) (*User, error) {
	if p == nil {
		return nil, errors.New("enterprise: static provider is nil")
	}
	if ctx == nil {
		return nil, errors.New("enterprise: context is nil")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	user, ok := p.users[token]
	if !ok {
		return nil, ErrIdentityNotFound
	}
	return cloneUser(user), nil
}

// ErrIdentityNotFound 表示身份提供商未找到对应 token 的用户。
var ErrIdentityNotFound = errors.New("enterprise: identity not found")

// HasRole 判断用户是否拥有指定角色。
func (u *User) HasRole(role Role) bool {
	if u == nil {
		return false
	}
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// AddRole 为角色列表追加角色（若尚未存在）。
func (u *User) AddRole(role Role) {
	if u == nil {
		return
	}
	if u.HasRole(role) {
		return
	}
	u.Roles = append(u.Roles, role)
}

func cloneUser(u *User) *User {
	if u == nil {
		return nil
	}
	roles := make([]Role, len(u.Roles))
	copy(roles, u.Roles)
	return &User{
		ID:    u.ID,
		Name:  u.Name,
		Email: u.Email,
		Roles: roles,
	}
}

// String 返回用户的可读描述，便于日志中标识用户。
func (u *User) String() string {
	if u == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s(%s)", u.ID, u.Email)
}
