// 本文件实现插件权限模型：外部插件默认按最小权限运行。
package plugin

import (
	"errors"
	"fmt"
	"strings"
)

// ErrPermissionDenied 表示插件请求了当前权限集中未授权的操作。
var ErrPermissionDenied = errors.New("plugin: permission denied")

// Permissions 描述一个插件进程允许执行的操作集合。
// 所有字段默认均为 false，即默认拒绝。
type Permissions struct {
	AllowNetwork     bool // 是否允许网络访问
	AllowExec        bool // 是否允许执行外部进程
	AllowFileWrite   bool // 是否允许写文件
	AllowReadAnyPath bool // 是否允许读取任意路径
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
