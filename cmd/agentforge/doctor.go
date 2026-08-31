package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runDoctor 实现 `agentforge doctor`：检查本机环境是否满足运行要求。
func runDoctor(args []string) error {
	fs := newFlagSet("doctor")
	dataDir := fs.String("data-dir", "", "自定义数据目录（默认 ~/.agentforge）")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := *dataDir
	if dir == "" {
		dir = "~/.agentforge"
	}
	expanded, err := expandHome(dir)
	if err != nil {
		return err
	}

	fmt.Println("agentforge doctor")
	fmt.Println("=================")
	ok := true

	ok = report("Go toolchain", checkGo(), ok)
	ok = report("DSH CLI (dsh)", checkExecutable("dsh"), ok)
	ok = report("Git", checkExecutable("git"), ok)
	ok = report("GitHub CLI (gh)", checkExecutable("gh"), ok)
	ok = report("Keychain backend", checkKeychainBackend(), ok)
	ok = report("Data directory", checkDataDir(expanded), ok)

	if ok {
		fmt.Println("\nAll checks passed.")
	} else {
		fmt.Println("\nSome checks failed. See messages above.")
	}
	return nil
}

func report(name string, err error, ok bool) bool {
	if err == nil {
		fmt.Printf("[PASS] %s\n", name)
		return ok
	}
	fmt.Printf("[WARN] %s: %v\n", name, err)
	return false
}

func checkGo() error {
	out, err := exec.Command("go", "version").Output()
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return fmt.Errorf("go version returned empty output")
	}
	fmt.Printf("       %s\n", version)
	return nil
}

func checkExecutable(name string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s not found in PATH", name)
	}
	fmt.Printf("       found at %s\n", path)
	return nil
}

func checkKeychainBackend() error {
	switch runtime.GOOS {
	case "darwin":
		return checkExecutable("security")
	case "windows":
		return checkExecutable("powershell")
	case "linux":
		return checkExecutable("secret-tool")
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}

func checkDataDir(dir string) error {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		fmt.Printf("       %s does not exist yet; it will be created on first run\n", dir)
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		fmt.Printf("       warning: %s permissions are %04o (expected 0700 or stricter)\n", dir, mode)
	}
	fmt.Printf("       %s\n", filepath.Clean(dir))
	return nil
}

// newFlagSet 创建一个不会把 -h 当错误的 flag.FlagSet，便于子命令统一处理。
func newFlagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}
