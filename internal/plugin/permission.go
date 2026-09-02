// 本文件实现插件权限模型：外部插件默认按最小权限运行。
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrPermissionDenied 表示插件请求了当前权限集中未授权的操作。
var ErrPermissionDenied = errors.New("plugin: permission denied")

// Permissions 描述一个插件进程允许执行的操作集合。
// 所有字段默认均为 false，即默认拒绝。
type Permissions struct {
	AllowNetwork     bool `json:"allowNetwork,omitempty"`
	AllowExec        bool `json:"allowExec,omitempty"`
	AllowFileWrite   bool `json:"allowFileWrite,omitempty"`
	AllowReadAnyPath bool `json:"allowReadAnyPath,omitempty"`
}

// UnmarshalJSON 兼容 lowerCamel 与 Go 大写字段名两种 manifest 写法。
func (p *Permissions) UnmarshalJSON(data []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key, value := range raw {
		enabled, ok := value.(bool)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "allownetwork", "allow_network", "network":
			p.AllowNetwork = enabled
		case "allowexec", "allow_exec", "exec", "execute":
			p.AllowExec = enabled
		case "allowfilewrite", "allow_file_write", "filewrite", "file_write":
			p.AllowFileWrite = enabled
		case "allowreadanypath", "allow_read_any_path", "readanypath", "read_any_path":
			p.AllowReadAnyPath = enabled
		}
	}
	return nil
}

// CheckPermission 对单个操作 op 做简单权限校验。
// 支持的操作名：network/net、exec/execute、file_write/write、read_any_path/read。
// 权限不足时返回包装了 ErrPermissionDenied 的错误；未知操作返回普通错误。
func CheckPermission(perm Permissions, op string) error {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "network", "net":
		if !perm.AllowNetwork {
			return fmt.Errorf("%w: network access", ErrPermissionDenied)
		}
	case "exec", "execute":
		if !perm.AllowExec {
			return fmt.Errorf("%w: process execution", ErrPermissionDenied)
		}
	case "file_write", "write":
		if !perm.AllowFileWrite {
			return fmt.Errorf("%w: file write", ErrPermissionDenied)
		}
	case "read_any_path", "read":
		if !perm.AllowReadAnyPath {
			return fmt.Errorf("%w: read any path", ErrPermissionDenied)
		}
	default:
		return fmt.Errorf("plugin: unknown permission operation %q", op)
	}
	return nil
}
