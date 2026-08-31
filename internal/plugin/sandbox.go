package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ErrNetworkSandboxUnsupported 表示当前版本未实现真正的网络沙箱。
// 为避免假装已隔离网络，只要插件请求 AllowNetwork 就直接拒绝执行。
var ErrNetworkSandboxUnsupported = errors.New("plugin: network sandbox is not implemented")

// RunSandboxed 在受限环境中直接执行一个可执行文件，不经过 shell。
//
// 当前沙箱边界：
//   - 禁止 shell：使用 os/exec 直接启动 executable，参数原样传递；
//   - 默认拒绝网络：不实现真实网络沙箱；若 perm.AllowNetwork 为 true，
//     会返回 ErrNetworkSandboxUnsupported，避免在无隔离保证时放行网络；
//   - 限制工作目录：cmd.Dir 被设置为 workDir，且 workDir 必须存在（若提供）；
//   - 超时：timeout > 0 时使用 context.WithTimeout 强制终止。
//
// 返回合并后的标准输出/标准错误文本（去除首尾空白）。
func RunSandboxed(ctx context.Context, executable string, args []string, workDir string, timeout time.Duration, perm Permissions) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := CheckPermission(perm, "exec"); err != nil {
		return "", err
	}
	if perm.AllowNetwork {
		return "", fmt.Errorf("%w: refusing to run with AllowNetwork until a real network sandbox is available", ErrNetworkSandboxUnsupported)
	}
	if strings.TrimSpace(executable) == "" {
		return "", fmt.Errorf("plugin: sandboxed executable is empty")
	}
	if workDir != "" {
		info, err := os.Stat(workDir)
		if err != nil {
			return "", fmt.Errorf("plugin: sandbox work directory: %w", err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("plugin: sandbox work directory is not a directory: %s", workDir)
		}
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		if timeout > 0 {
			return strings.TrimSpace(string(out)), fmt.Errorf("%w: command timed out after %s: %w", ErrTimeout, timeout, context.DeadlineExceeded)
		}
		return strings.TrimSpace(string(out)), fmt.Errorf("%w: command timed out: %w", ErrTimeout, context.DeadlineExceeded)
	}
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("plugin: sandboxed run %q: %w", executable, err)
	}
	return strings.TrimSpace(string(out)), nil
}
