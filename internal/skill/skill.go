// Package skill 负责 DSH Skill 的本地扫描、远端 available_skills 解析与合并。
package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Entry 表示一个 Skill 条目。
type Entry struct {
	Name        string // 唯一名称，本地以文件名（不含 .md）为准，远端以 available_skills 中的名称为准
	Description string // 说明摘要
	Detail      string // 详细说明；本地为完整 markdown 内容，远端为可用描述
	Source      string // "local" 或 "remote"
	Enabled     bool   // 当前是否启用（UI 内可切换，进程内生效）
}

// LoadLocal 扫描 <data-dir>/skills/*.md，返回本地 Skill 列表。
// 目录不存在时返回空列表而不是错误。
func LoadLocal(dataDir string) ([]Entry, error) {
	if dataDir == "" {
		return []Entry{}, nil
	}
	dir := filepath.Join(dataDir, "skills")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	out := make([]Entry, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".md") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			// 单个文件读取失败不拖垮整个目录。
			continue
		}
		name := strings.TrimSuffix(de.Name(), filepath.Ext(de.Name()))
		out = append(out, ParseLocal(name, string(data)))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// InstallFromDir 将 srcDir 中的 *.md 复制到 <destDir>/skills/。
// destDir 是数据根目录（例如 ~/.agentforge），Skill 文件会写入其 skills 子目录。
// 返回本次安装/更新的 Skill 列表。
func InstallFromDir(srcDir, destDir string) ([]Entry, error) {
	if srcDir == "" {
		return nil, fmt.Errorf("skill: source directory is empty")
	}
	if destDir == "" {
		return nil, fmt.Errorf("skill: destination directory is empty")
	}
	info, err := os.Stat(srcDir)
	if err != nil {
		return nil, fmt.Errorf("skill: source directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skill: source is not a directory: %s", srcDir)
	}

	skillsDir := filepath.Join(destDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return nil, fmt.Errorf("skill: create skills directory: %w", err)
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("skill: read source directory: %w", err)
	}

	out := make([]Entry, 0, len(entries))
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".md") {
			continue
		}
		srcPath := filepath.Join(srcDir, de.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return nil, fmt.Errorf("skill: read %s: %w", srcPath, err)
		}
		name := strings.TrimSuffix(de.Name(), filepath.Ext(de.Name()))
		destName := name + ".md"
		if err := os.WriteFile(filepath.Join(skillsDir, destName), data, 0o644); err != nil {
			return nil, fmt.Errorf("skill: write %s: %w", destName, err)
		}
		out = append(out, ParseLocal(name, string(data)))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// InstallFromRepo 使用 `git clone --depth 1` 将远端仓库克隆到临时目录，
// 再调用 InstallFromDir 安装其中的 *.md。若本机没有 git 会返回明确错误。
func InstallFromRepo(repoURL, destDir string) ([]Entry, error) {
	if repoURL == "" {
		return nil, fmt.Errorf("skill: repository URL is empty")
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("skill: git is required to install from repository: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "dsh-skill-repo-*")
	if err != nil {
		return nil, fmt.Errorf("skill: create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command(gitPath, "-c", "core.autocrlf=false", "clone", "--depth", "1", "--", repoURL, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("skill: git clone failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return InstallFromDir(tmpDir, destDir)
}

// manifestItem 表示远端 Skill 清单中的一项。
type manifestItem struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// SyncFromManifest 拉取 JSON manifest（可为 HTTP(S) URL 或本地文件路径），
// 逐个安装其中的 Skill。manifest 支持顶层数组或 {"skills":[...]} / {"items":[...]}。
func SyncFromManifest(manifestURL, destDir string) ([]Entry, error) {
	if manifestURL == "" {
		return nil, fmt.Errorf("skill: manifest URL is empty")
	}
	data, err := fetchManifest(manifestURL)
	if err != nil {
		return nil, err
	}
	items, err := parseManifest(data)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("skill: manifest contains no skills")
	}

	var out []Entry
	for _, item := range items {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			return nil, fmt.Errorf("skill: manifest item %q has empty url", item.Name)
		}
		var installed []Entry
		if isLocalDir(url) {
			installed, err = InstallFromDir(url, destDir)
		} else {
			installed, err = InstallFromRepo(url, destDir)
		}
		if err != nil {
			return nil, fmt.Errorf("skill: install %q from %q: %w", item.Name, url, err)
		}
		out = append(out, installed...)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func fetchManifest(manifestURL string) ([]byte, error) {
	switch {
	case strings.HasPrefix(manifestURL, "http://"), strings.HasPrefix(manifestURL, "https://"):
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(manifestURL)
		if err != nil {
			return nil, fmt.Errorf("skill: fetch manifest: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("skill: fetch manifest: %s", resp.Status)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			return nil, fmt.Errorf("skill: read manifest: %w", err)
		}
		return data, nil
	case strings.HasPrefix(manifestURL, "file://"):
		data, err := os.ReadFile(strings.TrimPrefix(manifestURL, "file://"))
		if err != nil {
			return nil, fmt.Errorf("skill: read manifest: %w", err)
		}
		return data, nil
	default:
		data, err := os.ReadFile(manifestURL)
		if err != nil {
			return nil, fmt.Errorf("skill: read manifest: %w", err)
		}
		return data, nil
	}
}

func parseManifest(data []byte) ([]manifestItem, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("skill: manifest is empty")
	}

	var arr []manifestItem
	if err := json.Unmarshal(trimmed, &arr); err == nil {
		return arr, nil
	}

	var obj struct {
		Skills []manifestItem `json:"skills"`
		Items  []manifestItem `json:"items"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("skill: parse manifest: %w", err)
	}
	if len(obj.Skills) > 0 {
		return obj.Skills, nil
	}
	return obj.Items, nil
}

func isLocalDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ParseLocal 解析一个本地 Skill 文件内容。
// 支持可选 YAML frontmatter（name/description/enabled），
// 缺省时名称使用文件名，摘要使用正文第一行，默认启用。
func ParseLocal(name, content string) Entry {
	e := Entry{
		Name:    strings.TrimSpace(name),
		Source:  "local",
		Enabled: true,
		Detail:  content,
	}

	lines := strings.Split(content, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFront := true
		for _, line := range lines[1:] {
			trim := strings.TrimSpace(line)
			if inFront && trim == "---" {
				break
			}
			key, val, ok := strings.Cut(trim, ":")
			if !ok {
				continue
			}
			key = strings.ToLower(strings.TrimSpace(key))
			val = strings.TrimSpace(strings.Trim(val, `"'`))
			switch key {
			case "name":
				if val != "" {
					e.Name = val
				}
			case "description":
				e.Description = val
			case "enabled":
				e.Enabled = parseBool(val, true)
			}
		}
	}
	if e.Description == "" {
		e.Description = summaryFromMarkdown(content)
	}
	return e
}

// availableSkillsRe 在包初始化时编译一次，避免每次解析会话文本都重复编译正则。
var availableSkillsRe = regexp.MustCompile(`(?is)<available_skills[^>]*>(.*?)</available_skills>`)

// ParseAvailableSkills 从会话文本中提取 <available_skills> 块。
// 支持 Markdown 列表行（- name: desc）和 JSON 数组两种常见形式。
func ParseAvailableSkills(text string) []Entry {
	var out []Entry
	for _, match := range availableSkillsRe.FindAllStringSubmatch(text, -1) {
		out = append(out, parseAvailableBlock(match[1])...)
	}
	return out
}

// ParseRemoteValue 将 state 中可能出现的 skills / available_skills 字段解析为 Skill 列表。
// 支持 string、[]any、map[string]any 三种常见形态。
func ParseRemoteValue(v any) []Entry {
	switch val := v.(type) {
	case string:
		return ParseAvailableSkills(val)
	case []any:
		var out []Entry
		for _, item := range val {
			switch it := item.(type) {
			case string:
				out = append(out, parseRemoteLine(it)...)
			case map[string]any:
				if e, ok := entryFromMap(it); ok {
					out = append(out, e)
				}
			}
		}
		return out
	case map[string]any:
		if e, ok := entryFromMap(val); ok {
			return []Entry{e}
		}
	}
	return nil
}

// Merge 合并本地与远端 Skill，按名称去重；同名时本地优先。
func Merge(local, remote []Entry) []Entry {
	out := make([]Entry, 0, len(local)+len(remote))
	seen := make(map[string]bool, len(local)+len(remote))
	for _, e := range local {
		if e.Name == "" || seen[e.Name] {
			continue
		}
		out = append(out, e)
		seen[e.Name] = true
	}
	for _, e := range remote {
		if e.Name == "" || seen[e.Name] {
			continue
		}
		out = append(out, e)
		seen[e.Name] = true
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// Toggle 切换指定名称 Skill 的启用状态，返回是否找到并切换。
func Toggle(entries []Entry, name string) bool {
	for i := range entries {
		if entries[i].Name == name {
			entries[i].Enabled = !entries[i].Enabled
			return true
		}
	}
	return false
}

func parseAvailableBlock(block string) []Entry {
	trimmed := strings.TrimSpace(block)
	if strings.HasPrefix(trimmed, "[") {
		var items []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Summary     string `json:"summary"`
		}
		if err := json.Unmarshal([]byte(trimmed), &items); err == nil {
			out := make([]Entry, 0, len(items))
			for _, it := range items {
				name := strings.TrimSpace(it.Name)
				if name == "" {
					continue
				}
				desc := strings.TrimSpace(it.Description)
				if desc == "" {
					desc = strings.TrimSpace(it.Summary)
				}
				out = append(out, Entry{Name: name, Description: desc, Detail: desc, Source: "remote", Enabled: true})
			}
			return out
		}
	}

	var out []Entry
	for _, line := range strings.Split(block, "\n") {
		out = append(out, parseRemoteLine(line)...)
	}
	return out
}

func parseRemoteLine(line string) []Entry {
	line = strings.TrimSpace(line)
	line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
	line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") {
		return nil
	}
	name, desc, ok := strings.Cut(line, ":")
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if ok {
		desc = strings.TrimSpace(desc)
	}
	detail := desc
	return []Entry{{Name: name, Description: desc, Detail: detail, Source: "remote", Enabled: true}}
}

func entryFromMap(m map[string]any) (Entry, bool) {
	name, _ := m["name"].(string)
	if name == "" {
		name, _ = m["id"].(string)
	}
	if name == "" {
		name, _ = m["skill"].(string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Entry{}, false
	}
	desc, _ := m["description"].(string)
	if desc == "" {
		desc, _ = m["summary"].(string)
	}
	desc = strings.TrimSpace(desc)
	return Entry{Name: name, Description: desc, Detail: desc, Source: "remote", Enabled: true}, true
}

func summaryFromMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	inFront := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if i == 0 && trim == "---" {
			inFront = true
			continue
		}
		if inFront {
			if trim == "---" {
				inFront = false
			}
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(trim, "- "))
	}
	return ""
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return def
	}
}
