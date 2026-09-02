package agentflow

import (
	"fmt"
	"regexp"
	"strings"
)

// systemWriteDirs 是默认拒绝写入的系统目录。注意这里只用于“写入”类操作，
// 单纯的读取（cat/ls /etc 等）不会命中。
var systemWriteDirs = []string{
	"/etc", "/usr", "/bin", "/boot", "/dev",
	"/lib", "/sbin", "/sys", "/proc",
}

// safeDevPaths 是允许写入的 /dev 伪设备；它们不是磁盘/块设备写入。
var safeDevPaths = []string{
	"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom",
	"/dev/tty", "/dev/stdin", "/dev/stdout", "/dev/stderr",
}

var (
	rePrivilege        = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(?:sudo|su)(?:\s|$)`)
	reNestedSh         = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(?:bash|sh|zsh|dash|ksh|fish)\s+(?:-[a-z]*c\b|--command\b)`)
	reRemotePipe       = regexp.MustCompile(`(?i)\b(?:curl|wget)\b[^;\n]*\|\s*(?:sudo\s+)?(?:ba|z|da|k|fi)?sh\b`)
	reRm               = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*rm\b`)
	reRmRoot           = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*rm\s+(?:-[a-z]*[rf][a-z]*\s+)+(?:--\s+)?-?/(?:\*)?(?:\s|$|;)`)
	reMkfs             = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(?:mkfs(?:\.[a-z0-9]+)?|mkswap)(?:\s|$)`)
	reDDInvoke         = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*dd\b`)
	reDDOFDev          = regexp.MustCompile(`(?i)\bof\s*=\s*/dev/[^\s;|&]+`)
	reChmod            = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*chmod\b`)
	reChown            = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*chown\b`)
	reRecursive        = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])-{1,2}[a-z]*r[a-z]*\b|--recursive\b`)
	reTargetRootSystem = regexp.MustCompile(`(?i)(?:^|[\s=])/(?:\*)?(?:\s|$)|(?:^|[\s=])/(?:etc|usr|bin|boot|dev|lib|sbin|sys|proc)(?:/|(?:\s|$))`)
	reOutputOpt        = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_-])(?:-[oO]\s*|--output(?:=|\s+)|--output-document(?:=|\s+)|-P\s+|--directory-prefix(?:=|\s+))("?[^\s;|&]+"?)`)
	reTee              = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*tee\b`)
	reCopyMove         = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(?:cp|mv|install|ln|scp)\b`)
	reCreate           = regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(?:touch|mkdir|truncate)\b`)
)

// checkCommandSafety 对 Agent 准备执行的命令做默认安全策略检查。
// 它只拦截高风险的系统破坏/提权/远程脚本执行/系统路径写入模式；
// 普通构建、测试、只读查看和下载到非系统路径的命令可以正常通过。
func checkCommandSafety(command string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}

	lower := strings.ToLower(command)

	if rePrivilege.MatchString(lower) {
		return unsafeCommandError("sudo/su 提权命令被默认禁止")
	}
	if reNestedSh.MatchString(lower) {
		return unsafeCommandError("bash -c/sh -c 等嵌套 shell 执行被默认禁止")
	}
	if remotePipeToShell(lower) {
		return unsafeCommandError("curl/wget 管道到 shell 的远程脚本执行被默认禁止")
	}
	if reRmRoot.MatchString(lower) {
		return unsafeCommandError("删除根目录的危险 rm 命令被默认禁止")
	}
	if reRm.MatchString(lower) && containsSystemPathArg(lower, "rm") {
		return unsafeCommandError("删除系统路径的危险 rm 命令被默认禁止")
	}
	if reMkfs.MatchString(lower) {
		return unsafeCommandError("mkfs/mkswap 磁盘格式化命令被默认禁止")
	}
	if reDDInvoke.MatchString(lower) {
		if m := reDDOFDev.FindString(lower); m != "" && !isSafeDevWrite(extractPathFromAssignment(m)) {
			return unsafeCommandError("dd 向磁盘设备写入被默认禁止")
		}
	}
	if reChmod.MatchString(lower) && reRecursive.MatchString(lower) && reTargetRootSystem.MatchString(lower) {
		return unsafeCommandError("chmod -R 修改根目录或系统路径权限被默认禁止")
	}
	if reChown.MatchString(lower) && reRecursive.MatchString(lower) && reTargetRootSystem.MatchString(lower) {
		return unsafeCommandError("chown -R 修改根目录或系统路径属主被默认禁止")
	}
	if err := checkSystemPathWrites(lower); err != nil {
		return err
	}

	return nil
}

type unsafeCommandError string

func (e unsafeCommandError) Error() string {
	return "agentflow: unsafe command: " + string(e)
}

func unsafeCommandErrorf(format string, args ...any) error {
	return unsafeCommandError(fmt.Sprintf(format, args...))
}

// checkSystemPathWrites 检测命令是否在向 /etc、/usr、/bin、/boot、/dev
// 等系统路径写入。普通读取不会被拦截。
func checkSystemPathWrites(lower string) error {
	// 1) shell 重定向：> /etc/...、>> /usr/...、2>/bin/... 等
	// 只匹配引号外的重定向，避免把 echo/git commit 等参数里的 "> /etc" 当成写入。
	for _, target := range redirectionTargets(lower) {
		if isSystemWritePath(target) {
			return unsafeCommandErrorf("向系统路径 %s 重定向写入被默认禁止", target)
		}
	}

	// 2) 下载/编译类 -o/-O/--output 到系统路径
	if outputOptWritesToSystem(lower) {
		return unsafeCommandError("下载/编译输出到系统路径被默认禁止")
	}

	// 3) tee 到系统路径
	if reTee.MatchString(lower) && containsSystemPathArg(lower, "tee") {
		return unsafeCommandError("tee 向系统路径写入被默认禁止")
	}

	// 4) cp/mv/install/ln/scp 复制或移动到系统路径
	if reCopyMove.MatchString(lower) && copyMoveWritesToSystem(lower) {
		return unsafeCommandError("复制/移动/链接到系统路径被默认禁止")
	}

	// 5) touch/mkdir/truncate 在系统路径创建或修改
	if reCreate.MatchString(lower) && containsSystemPathArg(lower, "touch|mkdir|truncate") {
		return unsafeCommandError("在系统路径创建/修改文件被默认禁止")
	}

	// 6) chmod/chown 修改系统路径的权限或属主
	if (reChmod.MatchString(lower) || reChown.MatchString(lower)) && containsSystemPathArg(lower, "chmod|chown") {
		return unsafeCommandError("chmod/chown 修改系统路径权限/属主被默认禁止")
	}

	return nil
}

// redirectionTargets 返回命令中位于引号外的 shell 重定向目标。
func redirectionTargets(s string) []string {
	var targets []string
	inSingle, inDouble := false, false
	for i := 0; i < len(s); {
		ch := s[i]
		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			i++
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
			}
			i++
			continue
		}
		if ch == '\'' {
			inSingle = true
			i++
			continue
		}
		if ch == '"' {
			inDouble = true
			i++
			continue
		}
		if ch == '\\' && i+1 < len(s) {
			i += 2
			continue
		}

		opStart := -1
		if ch == '>' {
			opStart = i
		} else if (ch == '&' || ch == '1' || ch == '2') && i+1 < len(s) && s[i+1] == '>' {
			opStart = i
		}
		if opStart >= 0 {
			j := opStart
			if j < len(s) && (s[j] == '&' || s[j] == '1' || s[j] == '2') {
				j++
			}
			for j < len(s) && s[j] == '>' {
				j++
			}
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			start := j
			for j < len(s) && !strings.ContainsRune(" \t\n;|&", rune(s[j])) {
				j++
			}
			if start < j {
				targets = append(targets, s[start:j])
				i = j
				continue
			}
		}
		i++
	}
	return targets
}

func isSystemWritePath(p string) bool {
	p = strings.Trim(strings.TrimSpace(p), `"'`)
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	for _, dir := range systemWriteDirs {
		if lower == dir || strings.HasPrefix(lower, dir+"/") {
			// /dev 下的普通伪设备通常允许写入；真正的块设备由 dd/mkfs 等规则拦截。
			if isSafeDevWrite(lower) {
				return false
			}
			return true
		}
	}
	return false
}

func isSafeDevWrite(p string) bool {
	lower := strings.ToLower(strings.TrimSpace(p))
	for _, dev := range safeDevPaths {
		if lower == dev || strings.HasPrefix(lower, dev+"/") {
			return true
		}
	}
	// /dev/fd/*、/dev/std* 也是进程伪设备，不按磁盘写入处理。
	return strings.HasPrefix(lower, "/dev/fd/") || strings.HasPrefix(lower, "/dev/std")
}

func outputOptWritesToSystem(lower string) bool {
	for _, name := range []string{"curl", "wget", "go", "gcc", "cc", "clang", "g++", "clang++", "rustc", "ld"} {
		idx := commandInvocationIndex(lower, name)
		if idx < 0 {
			continue
		}
		segment := shellSegment(lower[idx:])
		if i := strings.Index(segment, "|"); i >= 0 {
			segment = segment[:i]
		}
		for _, m := range reOutputOpt.FindAllStringSubmatch(segment, -1) {
			if len(m) >= 2 && isSystemWritePath(m[1]) {
				return true
			}
		}
	}
	return false
}

func extractPathFromAssignment(s string) string {
	_, after, ok := strings.Cut(s, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.Trim(after, `"'`))
}

// containsSystemPathArg 粗略判断命令参数里是否出现了绝对系统路径。
// 主要用于 cp/mv/install/ln/tee/touch/mkdir 等“目标路径”型命令。
func containsSystemPathArg(lower, commandPattern string) bool {
	// 截取第一个命令出现之后的内容，避免把命令名本身当成路径。
	idx := -1
	for _, name := range strings.Split(commandPattern, "|") {
		if i := commandInvocationIndex(lower, name); i >= 0 {
			if idx < 0 || i < idx {
				idx = i
			}
		}
	}
	if idx < 0 {
		return false
	}
	rest := lower[idx:]
	if i := strings.Index(rest, "|"); i >= 0 {
		rest = rest[:i]
	}
	reAbs := regexp.MustCompile(`(?i)(?:^|[\s="'])(/[^\s;|&]+)`)
	for _, m := range reAbs.FindAllStringSubmatch(rest, -1) {
		if isSystemWritePath(m[1]) {
			return true
		}
	}
	return false
}

// copyMoveWritesToSystem 判断 cp/mv/install/ln/scp 是否把目标放到系统路径。
// 这类命令只有最后一个路径参数（或 --target-directory）是写入目标，
// 因此不能因为源文件来自 /etc、/usr 等就误判为写入系统路径。
func copyMoveWritesToSystem(lower string) bool {
	for _, name := range []string{"cp", "mv", "install", "ln", "scp"} {
		idx := commandInvocationIndex(lower, name)
		if idx < 0 {
			continue
		}
		segment := shellSegment(lower[idx:])
		if i := strings.Index(segment, "|"); i >= 0 {
			segment = segment[:i]
		}
		if target := lastAbsolutePath(segment); target != "" && isSystemWritePath(target) {
			return true
		}
	}
	return false
}

// remotePipeToShell 检测 curl/wget 作为实际命令出现时是否通过管道交给 shell。
func remotePipeToShell(lower string) bool {
	for _, name := range []string{"curl", "wget"} {
		idx := commandInvocationIndex(lower, name)
		if idx < 0 {
			continue
		}
		segment := shellSegment(lower[idx:])
		if reRemotePipe.MatchString(segment) {
			return true
		}
	}
	return false
}

// commandInvocationIndex 返回命令名作为独立命令出现的位置，-1 表示未命中。
func commandInvocationIndex(lower, name string) int {
	re := regexp.MustCompile(`(?i)(?:^|[;&|({])\s*(?:sudo\s+)*(` + regexp.QuoteMeta(name) + `)(?:\s|$|[;&|])`)
	m := re.FindStringSubmatchIndex(lower)
	if m == nil {
		return -1
	}
	return m[2]
}

// shellSegment 截取从当前位置到下一个命令分隔符之前的内容。
func shellSegment(s string) string {
	cut := len(s)
	for _, sep := range []string{"\n", ";", "&&", "||"} {
		if i := strings.Index(s, sep); i >= 0 && i < cut {
			cut = i
		}
	}
	return s[:cut]
}

// lastAbsolutePath 返回片段中最后一个绝对路径参数。
func lastAbsolutePath(segment string) string {
	re := regexp.MustCompile(`(?i)(?:^|[\s="'])(/[^\s;|&]+)`)
	ms := re.FindAllStringSubmatch(segment, -1)
	if len(ms) == 0 {
		return ""
	}
	return strings.Trim(ms[len(ms)-1][1], `"'`)
}
