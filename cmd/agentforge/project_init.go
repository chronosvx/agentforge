package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// runProjectInit 实现 `agentforge project-init`：为当前项目生成 AGENT.md。
//
// 默认在 --dir 目录下生成 AGENT.md，可通过 --output 指定其它输出文件；
// 已存在 AGENT.md 时需要 --force 才会覆盖。
func runProjectInit(args []string) error {
	fs := flag.NewFlagSet("project-init", flag.ContinueOnError)
	dir := fs.String("dir", ".", "项目目录（默认当前目录）")
	force := fs.Bool("force", false, "覆盖已存在的 AGENT.md")
	output := fs.String("output", "", "输出文件路径（默认 <dir>/AGENT.md）")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("project-init: unexpected arguments: %v", fs.Args())
	}

	projectDir, err := expandHome(*dir)
	if err != nil {
		return fmt.Errorf("project-init: resolve --dir: %w", err)
	}
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("project-init: resolve --dir: %w", err)
	}
	info, err := os.Stat(absDir)
	if err != nil {
		return fmt.Errorf("project-init: stat %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project-init: %s is not a directory", absDir)
	}

	outPath := *output
	if outPath == "" {
		outPath = filepath.Join(absDir, "AGENT.md")
	} else {
		outPath, err = expandHome(outPath)
		if err != nil {
			return fmt.Errorf("project-init: resolve --output: %w", err)
		}
	}

	if _, err := os.Stat(outPath); err == nil && !*force {
		return fmt.Errorf("project-init: %s already exists (use --force to overwrite)", outPath)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("project-init: stat %s: %w", outPath, err)
	}

	content := buildProjectContext(absDir)
	if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("project-init: write %s: %w", outPath, err)
	}
	if err := os.Chmod(outPath, 0o644); err != nil {
		return fmt.Errorf("project-init: chmod %s: %w", outPath, err)
	}
	fmt.Printf("project context written to %s\n", outPath)
	return nil
}

// buildProjectContext 生成 AGENT.md 的 Markdown 内容。
func buildProjectContext(dir string) string {
	name := filepath.Base(dir)
	techLines := detectTechStack(dir)
	entries := listTopLevel(dir)
	branch, remote := gitInfo(dir)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	fmt.Fprintf(&b, "> 本文件是 AGENT.md，由 `agentforge project-init` 自动生成，用于为 AI Agent 提供项目上下文。\n\n")

	fmt.Fprintf(&b, "## 项目信息\n\n")
	fmt.Fprintf(&b, "- 项目名：%s\n", name)
	if branch != "" {
		fmt.Fprintf(&b, "- Git 分支：%s\n", branch)
	}
	if remote != "" {
		fmt.Fprintf(&b, "- Remote Origin：%s\n", remote)
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "## 技术栈\n\n")
	if len(techLines) == 0 {
		fmt.Fprintln(&b, "- 未检测到常见技术栈")
	} else {
		for _, line := range techLines {
			fmt.Fprintln(&b, line)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "## 顶层结构\n\n")
	if len(entries) == 0 {
		fmt.Fprintln(&b, "- （空）")
	} else {
		for _, entry := range entries {
			fmt.Fprintf(&b, "- %s\n", entry)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "## 使用说明\n\n")
	fmt.Fprintf(&b, "- 开始工作前，请先阅读 README、README.* 或 docs/ 下的文档，理解项目目标、结构与现有约定。\n")
	fmt.Fprintf(&b, "- 修改代码前，请先查看相关实现和测试，遵循仓库已有的命名、风格和架构约定。\n")
	fmt.Fprintf(&b, "- 不要随意破坏现有命令、配置格式、对外接口或兼容性；如确需变更，请同步更新文档和测试。\n")
	fmt.Fprintf(&b, "- 尽量做小步、可验证的修改；提交前运行相关测试和格式化/静态检查工具。\n")
	fmt.Fprintf(&b, "- 如果对项目意图不确定，请先搜索代码或询问维护者，不要臆测。\n")

	return b.String()
}

// projectInitIgnoredNames 是生成项目上下文时需要忽略的常见目录/文件。
var projectInitIgnoredNames = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	".idea":        true,
	".vscode":      true,
	".next":        true,
	".cache":       true,
	".DS_Store":    true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"vendor":       true,
	"__pycache__":  true,
	"target":       true,
}

// listTopLevel 返回目录的顶层结构，深度限制为 1。
func listTopLevel(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if projectInitIgnoredNames[name] {
			continue
		}
		if entry.IsDir() {
			lines = append(lines, name+"/")
		} else {
			lines = append(lines, name)
		}
	}
	sort.Strings(lines)
	return lines
}

// detectTechStack 扫描目录并返回按字母排序的技术栈 Markdown 行。
func detectTechStack(dir string) []string {
	clues := map[string]map[string]bool{}
	add := func(tech, clue string) {
		if clues[tech] == nil {
			clues[tech] = map[string]bool{}
		}
		clues[tech][clue] = true
	}

	exactFiles := map[string]string{
		"go.mod":              "Go",
		"go.work":             "Go",
		"package.json":        "JavaScript/Node.js",
		"pnpm-lock.yaml":      "JavaScript/Node.js",
		"yarn.lock":           "JavaScript/Node.js",
		"package-lock.json":   "JavaScript/Node.js",
		"Cargo.toml":          "Rust",
		"pyproject.toml":      "Python",
		"requirements.txt":    "Python",
		"setup.py":            "Python",
		"Pipfile":             "Python",
		"poetry.lock":         "Python",
		"pom.xml":             "Java",
		"build.gradle":        "Java",
		"build.gradle.kts":    "Kotlin",
		"settings.gradle":     "Java",
		"settings.gradle.kts": "Kotlin",
		"Dockerfile":          "Docker",
		"Makefile":            "Make",
		"GNUmakefile":         "Make",
		"CMakeLists.txt":      "CMake",
		"compose.yaml":        "Docker",
		"compose.yml":         "Docker",
		"docker-compose.yml":  "Docker",
		"docker-compose.yaml": "Docker",
		"tsconfig.json":       "TypeScript",
		"jsconfig.json":       "JavaScript",
		"Gemfile":             "Ruby",
		"composer.json":       "PHP",
		"pubspec.yaml":        "Dart",
	}

	extensions := map[string]string{
		".go":     "Go",
		".js":     "JavaScript",
		".jsx":    "JavaScript/React",
		".ts":     "TypeScript",
		".tsx":    "TypeScript/React",
		".py":     "Python",
		".rs":     "Rust",
		".java":   "Java",
		".kt":     "Kotlin",
		".kts":    "Kotlin",
		".rb":     "Ruby",
		".php":    "PHP",
		".cs":     "C#",
		".cpp":    "C++",
		".cc":     "C++",
		".cxx":    "C++",
		".c":      "C",
		".h":      "C/C++",
		".hpp":    "C++",
		".swift":  "Swift",
		".vue":    "Vue",
		".svelte": "Svelte",
		".ex":     "Elixir",
		".exs":    "Elixir",
		".sql":    "SQL",
		".sh":     "Shell",
		".html":   "HTML",
		".css":    "CSS",
		".scala":  "Scala",
		".dart":   "Dart",
	}

	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != dir && projectInitIgnoredNames[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if tech, ok := exactFiles[name]; ok {
			add(tech, name)
		}
		if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
			if tech, ok := extensions[ext]; ok {
				add(tech, "*"+ext)
			}
		}
		return nil
	})

	techs := make([]string, 0, len(clues))
	for tech := range clues {
		techs = append(techs, tech)
	}
	sort.Strings(techs)

	lines := make([]string, 0, len(techs))
	for _, tech := range techs {
		clueList := make([]string, 0, len(clues[tech]))
		for clue := range clues[tech] {
			clueList = append(clueList, clue)
		}
		sort.Strings(clueList)
		lines = append(lines, fmt.Sprintf("- %s（%s）", tech, strings.Join(clueList, "、")))
	}
	return lines
}

// gitInfo 返回目录所在 Git 仓库的当前分支和 remote origin；不是 Git 仓库时返回空。
func gitInfo(dir string) (branch, remote string) {
	branch = gitOutput(dir, "branch", "--show-current")
	remote = gitOutput(dir, "config", "--get", "remote.origin.url")
	return branch, remote
}

// gitOutput 在指定目录运行 git 命令并返回去除首尾空白的标准输出。
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
