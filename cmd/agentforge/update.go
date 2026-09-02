package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/update"
)

const (
	publicInstallOwner = "chronosvx"
	publicInstallRepo  = "agentforge-install"
)

var (
	currentExecutablePath = os.Executable
	downloadReleaseAsset  = update.DownloadReleaseAsset
	latestFromPublic      = update.LatestFromPublic
)

// maybeCheckUpdate 检查公开安装仓库上的新版本，返回可提示的新版本号；
// 没有新版本或检查失败时返回空字符串。
func maybeCheckUpdate() string {
	if os.Getenv("AGENTFORGE_NO_UPDATE_CHECK") != "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	latest, err := latestFromPublic(ctx)
	if err != nil {
		return ""
	}

	newer, err := update.Check(version, latest)
	if err != nil || !newer {
		return ""
	}
	return latest
}

// runUpdate 实现 `agentforge update`：从公开安装仓库下载并校验新版本，
// 然后备份并替换当前可执行文件。
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	versionOpt := fs.String("version", "", "要升级到的目标版本（默认最新版）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("update: unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	exe, err := currentExecutablePath()
	if err != nil {
		return fmt.Errorf("update: locate current executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return fmt.Errorf("update: resolve executable path: %w", err)
	}

	target := strings.TrimSpace(*versionOpt)
	if target == "" || strings.EqualFold(target, "latest") {
		target, err = latestFromPublic(ctx)
		if err != nil {
			return fmt.Errorf("update: resolve latest version: %w", err)
		}
	}
	tag := normalizeReleaseTag(target)
	asset := update.AssetName(runtime.GOOS, runtime.GOARCH)
	fmt.Printf("agentforge: updating to %s (%s)\n", tag, asset)

	// 在可执行文件同目录创建临时文件，保证后续 rename 在同一文件系统内。
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".agentforge-update-*")
	if err != nil {
		return fmt.Errorf("update: create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("update: close temporary file: %w", err)
	}
	defer os.Remove(tmpPath)

	checksumsTmp, err := os.CreateTemp("", "agentforge-checksums-*.txt")
	if err != nil {
		return fmt.Errorf("update: create checksums temporary file: %w", err)
	}
	checksumsPath := checksumsTmp.Name()
	if err := checksumsTmp.Close(); err != nil {
		_ = os.Remove(checksumsPath)
		return fmt.Errorf("update: close checksums temporary file: %w", err)
	}
	defer os.Remove(checksumsPath)

	fmt.Printf("agentforge: downloading %s\n", asset)
	if err := downloadReleaseAsset(ctx, publicInstallOwner, publicInstallRepo, tag, asset, tmpPath); err != nil {
		return fmt.Errorf("update: download %s: %w", asset, err)
	}
	fmt.Println("agentforge: downloading checksums.txt")
	if err := downloadReleaseAsset(ctx, publicInstallOwner, publicInstallRepo, tag, "checksums.txt", checksumsPath); err != nil {
		return fmt.Errorf("update: download checksums.txt: %w", err)
	}
	if err := update.VerifyChecksum(tmpPath, checksumsPath, asset); err != nil {
		return fmt.Errorf("update: verify checksum: %w", err)
	}

	backupPath := exe + ".bak"
	if err := replaceExecutable(exe, tmpPath, backupPath); err != nil {
		return err
	}

	fmt.Printf("agentforge: updated to %s\n", tag)
	return nil
}

// normalizeReleaseTag 确保版本字符串以小写 v 开头，作为 GitHub Release tag 使用。
func normalizeReleaseTag(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return version
	}
	if version[0] == 'v' || version[0] == 'V' {
		return "v" + version[1:]
	}
	return "v" + version
}

// replaceExecutable 将旧可执行文件备份为 backupPath，再用 newPath 原子替换 exePath。
func replaceExecutable(exePath, newPath, backupPath string) error {
	// 清理旧的备份文件；Unix 上 os.Rename 可直接覆盖，Windows 上先删除更稳妥。
	if _, err := os.Lstat(backupPath); err == nil {
		if err := os.Remove(backupPath); err != nil {
			return fmt.Errorf("update: remove old backup %s: %w", backupPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("update: inspect backup %s: %w", backupPath, err)
	}

	if err := os.Rename(exePath, backupPath); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("update: backup current executable failed: %w\nhint: on Windows, close other agentforge processes or run from a non-running copy, then retry", err)
		}
		return fmt.Errorf("update: backup current executable: %w", err)
	}

	if err := os.Rename(newPath, exePath); err != nil {
		// 尽力恢复旧文件，避免升级失败后留下缺失的可执行文件。
		_ = os.Rename(backupPath, exePath)
		if runtime.GOOS == "windows" {
			return fmt.Errorf("update: replace executable failed: %w\nhint: on Windows, close other agentforge processes or run from a non-running copy, then retry", err)
		}
		return fmt.Errorf("update: replace executable: %w", err)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(exePath, 0o755); err != nil {
			// 文件已替换，权限失败只提示，不阻断升级结果。
			fmt.Fprintf(os.Stderr, "agentforge: warning: chmod new executable: %v\n", err)
		}
	}
	return nil
}
