package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"github.com/chronosvx/agentforge/internal/config"
	"github.com/chronosvx/agentforge/internal/enterprise"
	"github.com/chronosvx/agentforge/internal/executor"
	"os"
	"strings"
	"time"
)

// execOptions 保存 `exec` 子命令的解析结果。
type execOptions struct {
	local   bool
	docker  bool
	ssh     bool
	cmd     string
	image   string
	host    string
	user    string
	port    int
	workDir string
	env     []string
	timeout time.Duration
	dataDir string
}

// stringListValue 支持重复的 --env KEY=VALUE 参数。
type stringListValue []string

func (s *stringListValue) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListValue) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// parseExecArgs 解析 `agentforge exec` 子命令参数。
func parseExecArgs(args []string) (execOptions, error) {
	fs := flag.NewFlagSet("agentforge exec", flag.ContinueOnError)
	var opts execOptions
	fs.BoolVar(&opts.local, "local", false, "run locally")
	fs.BoolVar(&opts.docker, "docker", false, "run in a Docker container")
	fs.BoolVar(&opts.ssh, "ssh", false, "run over SSH")
	fs.StringVar(&opts.cmd, "cmd", "", "command line to execute")
	fs.StringVar(&opts.image, "image", "", "Docker image (required with --docker)")
	fs.StringVar(&opts.host, "host", "", "SSH host or user@host (required with --ssh)")
	fs.StringVar(&opts.user, "user", "", "SSH user (optional)")
	fs.IntVar(&opts.port, "port", 0, "SSH port (optional)")
	fs.StringVar(&opts.workDir, "workdir", "", "working directory")
	fs.Var((*stringListValue)(&opts.env), "env", "environment variable KEY=VALUE (repeatable)")
	fs.DurationVar(&opts.timeout, "timeout", 0, "execution timeout (e.g. 5s, 1m)")
	fs.StringVar(&opts.dataDir, "data-dir", config.DefaultDataDir, "本地数据目录（用于 SIEM/审计日志）")
	if err := fs.Parse(args); err != nil {
		return execOptions{}, err
	}
	if fs.NArg() > 0 {
		return execOptions{}, fmt.Errorf("exec: unexpected arguments: %v", fs.Args())
	}

	modes := 0
	if opts.local {
		modes++
	}
	if opts.docker {
		modes++
	}
	if opts.ssh {
		modes++
	}
	if modes != 1 {
		return execOptions{}, errors.New("exec: exactly one of --local, --docker, --ssh is required")
	}
	if strings.TrimSpace(opts.cmd) == "" {
		return execOptions{}, errors.New("exec: --cmd is required")
	}
	if opts.docker && strings.TrimSpace(opts.image) == "" {
		return execOptions{}, errors.New("exec: --image is required with --docker")
	}
	if opts.ssh && strings.TrimSpace(opts.host) == "" {
		return execOptions{}, errors.New("exec: --host is required with --ssh")
	}
	return opts, nil
}

// splitCommandLine 把命令行字符串拆成程序名与参数，支持单引号、双引号和反斜杠转义。
func splitCommandLine(s string) ([]string, error) {
	var fields []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	started := false
	flush := func() {
		if started {
			fields = append(fields, cur.String())
			cur.Reset()
			started = false
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
			started = true
		case inDouble:
			if r == '"' {
				inDouble = false
			} else if r == '\\' && i+1 < len(runes) && (runes[i+1] == '"' || runes[i+1] == '\\' || runes[i+1] == '$' || runes[i+1] == '`') {
				i++
				cur.WriteRune(runes[i])
			} else {
				cur.WriteRune(r)
			}
			started = true
		case r == '\'':
			inSingle = true
			started = true
		case r == '"':
			inDouble = true
			started = true
		case r == '\\':
			if i+1 < len(runes) {
				i++
				cur.WriteRune(runes[i])
			} else {
				cur.WriteRune(r)
			}
			started = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inSingle || inDouble {
		return nil, errors.New("exec: unterminated quote in command")
	}
	flush()
	if len(fields) == 0 {
		return nil, errors.New("exec: empty command")
	}
	return fields, nil
}

// runExec 实现 `agentforge exec` 子命令（不指定企业身份时保持原有行为）。
func runExec(args []string) error {
	return runExecWithIdentity(args, "", "")
}

// runExecWithIdentity 在 exec 子命令前执行 RBAC 校验，并记录 SIEM 事件。
func runExecWithIdentity(args []string, identityUser, identityRoles string) (retErr error) {
	opts, err := parseExecArgs(args)
	if err != nil {
		return err
	}

	if err := checkExecRBAC(identityUser, identityRoles); err != nil {
		dir, _ := expandHome(opts.dataDir)
		emitSIEM(newSIEMSink(dir), identityUser, "exec.run", "exec", "denied")
		return err
	}

	dir, err := expandHome(opts.dataDir)
	if err != nil {
		return err
	}
	siemLog := newSIEMSink(dir)
	defer func() {
		result := "ok"
		if retErr != nil {
			result = "error"
		}
		emitSIEM(siemLog, identityUser, "exec.run", "exec", result)
	}()

	parts, err := splitCommandLine(opts.cmd)
	if err != nil {
		return err
	}

	spec := executor.Spec{
		Command: parts[0],
		Args:    parts[1:],
		WorkDir: opts.workDir,
		Env:     opts.env,
		Timeout: opts.timeout,
	}

	var ex executor.Executor
	switch {
	case opts.local:
		ex = executor.NewLocalExecutor()
	case opts.docker:
		ex = executor.NewDockerExecutor(opts.image)
	case opts.ssh:
		ex = executor.NewSSHExecutor(opts.host, opts.user, opts.port)
	}

	res, err := ex.Run(context.Background(), spec)
	if res.Stdout != "" {
		fmt.Print(res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprint(os.Stderr, res.Stderr)
	}
	if err != nil {
		if res.ExitCode != 0 {
			return fmt.Errorf("exec: command failed with exit code %d", res.ExitCode)
		}
		return fmt.Errorf("exec: %w", err)
	}
	return nil
}

// checkExecRBAC 校验模拟企业用户是否有权执行 exec。
//
// 未指定 --user 时默认允许；指定了 --user 但未指定 --roles 时也默认允许；
// 一旦提供角色，就使用 DefaultPolicy（可视为已配置策略）进行校验。
func checkExecRBAC(user, roles string) error {
	if strings.TrimSpace(user) == "" {
		return nil
	}
	roleNames := splitRoles(roles)
	if len(roleNames) == 0 {
		return nil
	}

	parsed := make([]enterprise.Role, 0, len(roleNames))
	for _, name := range roleNames {
		role, err := enterprise.ParseRole(name)
		if err != nil {
			return fmt.Errorf("exec: %w", err)
		}
		parsed = append(parsed, role)
	}

	rbac := enterprise.NewRBAC(enterprise.DefaultPolicy())
	u := &enterprise.User{ID: user, Name: user, Roles: parsed}
	if !rbac.Check(u, "exec.run") {
		return fmt.Errorf("exec: RBAC denied user %q (roles %s) from running commands", user, strings.Join(roleNames, ","))
	}
	return nil
}

// splitRoles 将逗号分隔的角色字符串拆分为去空白的角色名列表。
func splitRoles(roles string) []string {
	var out []string
	for _, part := range strings.Split(roles, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, name)
		}
	}
	return out
}
