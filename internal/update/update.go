// Package update 提供自动更新检查所需的版本比较与 GitHub Release 查询能力。
//
// 当前只负责“检查并提示”，不会自动下载或安装新版本。
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/security"
)

// GitHubAPIBase 是 GitHub REST API 默认入口。
const GitHubAPIBase = "https://api.github.com"

// maxResponseSize 限制 GitHub API 响应读取上限，避免异常响应占用过多内存。
const maxResponseSize = 1 << 20 // 1 MiB

// Check 比较当前版本与最新版本，返回是否有可用更新（latest > current）。
//
// 版本号遵循 SemVer 2.0 的主要规则：可选前缀 v/V、major.minor.patch、
// 可选 prerelease 与 build metadata。build metadata 不参与优先级比较。
func Check(currentVersion, latestVersion string) (bool, error) {
	current, err := Parse(currentVersion)
	if err != nil {
		return false, fmt.Errorf("update: parse current version %q: %w", currentVersion, err)
	}
	latest, err := Parse(latestVersion)
	if err != nil {
		return false, fmt.Errorf("update: parse latest version %q: %w", latestVersion, err)
	}
	return Compare(latest, current) > 0, nil
}

// Version 表示一个解析后的语义化版本。
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease []string
	// Build 保留但不参与比较。
	Build []string
}

// Parse 解析语义化版本字符串，支持 v1.2.3、1.2.3、1.2.3-alpha.1+build。
func Parse(s string) (Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Version{}, errors.New("empty version")
	}
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		s = s[1:]
	}

	// 分离 build metadata。
	if i := strings.IndexByte(s, '+'); i >= 0 {
		build := s[i+1:]
		if build == "" {
			return Version{}, errors.New("empty build metadata")
		}
		s = s[:i]
		ver, err := parseCoreAndPre(s)
		if err != nil {
			return Version{}, err
		}
		ver.Build = strings.Split(build, ".")
		return ver, nil
	}
	return parseCoreAndPre(s)
}

func parseCoreAndPre(s string) (Version, error) {
	var v Version

	// 分离 prerelease。
	if i := strings.IndexByte(s, '-'); i >= 0 {
		pre := s[i+1:]
		if pre == "" {
			return Version{}, errors.New("empty prerelease")
		}
		v.PreRelease = strings.Split(pre, ".")
		s = s[:i]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("expected major.minor.patch, got %q", s)
	}

	var err error
	if v.Major, err = strconv.Atoi(parts[0]); err != nil || v.Major < 0 {
		return Version{}, fmt.Errorf("invalid major %q", parts[0])
	}
	if v.Minor, err = strconv.Atoi(parts[1]); err != nil || v.Minor < 0 {
		return Version{}, fmt.Errorf("invalid minor %q", parts[1])
	}
	if v.Patch, err = strconv.Atoi(parts[2]); err != nil || v.Patch < 0 {
		return Version{}, fmt.Errorf("invalid patch %q", parts[2])
	}
	return v, nil
}

// Compare 比较两个版本。a < b 返回负数，a == b 返回 0，a > b 返回正数。
func Compare(a, b Version) int {
	if c := compareInt(a.Major, b.Major); c != 0 {
		return c
	}
	if c := compareInt(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := compareInt(a.Patch, b.Patch); c != 0 {
		return c
	}

	// 没有 prerelease 的版本优先级更高。
	if len(a.PreRelease) == 0 && len(b.PreRelease) == 0 {
		return 0
	}
	if len(a.PreRelease) == 0 {
		return 1
	}
	if len(b.PreRelease) == 0 {
		return -1
	}

	for i := 0; i < len(a.PreRelease) && i < len(b.PreRelease); i++ {
		if c := compareIdentifier(a.PreRelease[i], b.PreRelease[i]); c != 0 {
			return c
		}
	}
	return compareInt(len(a.PreRelease), len(b.PreRelease))
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareIdentifier(a, b string) int {
	aNum, aErr := strconv.Atoi(a)
	bNum, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		return compareInt(aNum, bNum)
	case aErr == nil:
		// 数字标识符优先级低于非数字标识符。
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// LatestFromGitHub 查询指定 GitHub 仓库最新 Release 的 tag_name。
//
// 该函数仅用于未来接入自动更新提示，不会下载或安装任何文件。
func LatestFromGitHub(owner, repo string) (string, error) {
	return LatestFromGitHubWithClient(context.Background(), owner, repo, GitHubAPIBase, &http.Client{Timeout: 10 * time.Second})
}

// LatestFromGitHubWithClient 与 LatestFromGitHub 相同，但允许注入 API base URL
// 与 HTTP client，便于测试（例如 httptest）。
func LatestFromGitHubWithClient(ctx context.Context, owner, repo, apiBase string, hc *http.Client) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return "", errors.New("update: owner and repo are required")
	}
	if apiBase == "" {
		apiBase = GitHubAPIBase
	}
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}

	endpoint := strings.TrimRight(apiBase, "/") + "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("update: create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agentforge")

	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("update: request %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("update: github api http %d: %s", resp.StatusCode, security.Redact(strings.TrimSpace(string(body))))
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return "", fmt.Errorf("update: read github response: %w", err)
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("update: decode github response: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", errors.New("update: github latest release has empty tag_name")
	}
	return release.TagName, nil
}
