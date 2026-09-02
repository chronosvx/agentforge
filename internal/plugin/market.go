package plugin

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	// maxMarketIndexSize 限制市场索引 JSON 的最大读取字节数。
	maxMarketIndexSize = 10 << 20 // 10 MiB
	// maxPluginDownloadSize 限制单个插件包下载/解压的最大字节数。
	maxPluginDownloadSize = 200 << 20 // 200 MiB
)

// ErrMarketEntryNotFound 表示市场索引中找不到指定插件。
var ErrMarketEntryNotFound = errors.New("plugin: market entry not found")

// ErrMarketEntryIntegrityRequired 表示市场条目缺少可用于默认安装策略的完整性信息
// （没有 SHA256，也没有可验证的签名），安装应被拒绝。
var ErrMarketEntryIntegrityRequired = errors.New("plugin: market entry has neither SHA256 nor signature")

// MarketIndex 是插件仓库/市场索引中的一个条目。
type MarketIndex struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	SHA256      string    `json:"sha256,omitempty"`
	Signature   Signature `json:"signature,omitempty"`
}

// MarshalJSON 在签名为空时省略 signature 字段，保证用于签名的规范数据不含空签名对象。
func (m MarketIndex) MarshalJSON() ([]byte, error) {
	var sig *Signature
	if !m.Signature.IsZero() {
		s := m.Signature
		sig = &s
	}
	return json.Marshal(struct {
		Name        string     `json:"name"`
		Version     string     `json:"version"`
		Description string     `json:"description"`
		URL         string     `json:"url"`
		SHA256      string     `json:"sha256,omitempty"`
		Signature   *Signature `json:"signature,omitempty"`
	}{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		URL:         m.URL,
		SHA256:      m.SHA256,
		Signature:   sig,
	})
}

// LoadMarketIndex 解析市场索引 JSON。
// 支持两种格式：顶层数组，或 {"plugins":[...]}。
func LoadMarketIndex(data []byte) ([]MarketIndex, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("plugin: market index is empty")
	}

	var arr []MarketIndex
	if err := json.Unmarshal(trimmed, &arr); err == nil {
		return arr, nil
	}

	var obj struct {
		Plugins []MarketIndex `json:"plugins"`
	}
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, fmt.Errorf("plugin: parse market index: %w", err)
	}
	return obj.Plugins, nil
}

// FetchMarketIndex 从 URL 拉取并解析市场索引。
// 支持 http(s) URL、file:// URL 和本地文件路径。
func FetchMarketIndex(ctx context.Context, url string) ([]MarketIndex, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"):
		data, err = fetchHTTP(ctx, url, maxMarketIndexSize)
	case strings.HasPrefix(url, "file://"):
		data, err = os.ReadFile(strings.TrimPrefix(url, "file://"))
	default:
		data, err = os.ReadFile(url)
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: fetch market index: %w", err)
	}
	return LoadMarketIndex(data)
}

// InstallFromMarket 从市场索引中按名称安装一个插件到 destDir。
// 插件会被安装到 destDir/<name> 子目录；支持 HTTP(S) 插件包（zip/tar/tar.gz/原始可执行文件）
// 或 git 仓库。默认要求条目提供可校验的 SHA256；若传入 TrustStore，也可凭可信签名安装。
// SHA256 非空时会校验下载文件或克隆后的文件树；传入 TrustStore 时还会强制校验条目签名。
func InstallFromMarket(ctx context.Context, index []MarketIndex, name, destDir string, trustStores ...*TrustStore) (Plugin, error) {
	return installFromMarket(ctx, index, name, destDir, false, trustStores...)
}

// InstallFromMarketAllowUnsigned 与 InstallFromMarket 类似，但允许安装既没有 SHA256
// 也没有签名的市场条目。该函数仅应在用户显式选择不校验完整性时使用。
func InstallFromMarketAllowUnsigned(ctx context.Context, index []MarketIndex, name, destDir string, trustStores ...*TrustStore) (Plugin, error) {
	return installFromMarket(ctx, index, name, destDir, true, trustStores...)
}

func installFromMarket(ctx context.Context, index []MarketIndex, name, destDir string, allowUnsigned bool, trustStores ...*TrustStore) (Plugin, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Plugin{}, fmt.Errorf("plugin: market plugin name is empty")
	}
	var trustStore *TrustStore
	if len(trustStores) > 0 {
		trustStore = trustStores[0]
	}
	for _, entry := range index {
		if strings.TrimSpace(entry.Name) == name {
			return installMarketEntryWithPolicy(ctx, entry, destDir, trustStore, !allowUnsigned)
		}
	}
	return Plugin{}, fmt.Errorf("%w: %s", ErrMarketEntryNotFound, name)
}

// manifestDataForSignature 返回用于签名/校验的市场条目规范数据。
// 签名不包含条目自身的 signature 字段，避免自引用。
func manifestDataForSignature(entry MarketIndex) ([]byte, error) {
	unsigned := entry
	unsigned.Signature = Signature{}
	return json.Marshal(unsigned)
}

// installMarketEntry 保留旧内部签名：默认要求完整性校验。
func installMarketEntry(ctx context.Context, entry MarketIndex, destDir string, trustStore *TrustStore) (Plugin, error) {
	return installMarketEntryWithPolicy(ctx, entry, destDir, trustStore, true)
}

func installMarketEntryWithPolicy(ctx context.Context, entry MarketIndex, destDir string, trustStore *TrustStore, requireIntegrity bool) (Plugin, error) {
	name := strings.TrimSpace(entry.Name)
	url := strings.TrimSpace(entry.URL)
	if name == "" {
		return Plugin{}, fmt.Errorf("plugin: market entry missing name")
	}
	if url == "" {
		return Plugin{}, fmt.Errorf("plugin: market entry %q missing url", name)
	}
	if destDir == "" {
		return Plugin{}, fmt.Errorf("plugin: destination directory is empty")
	}
	if !isSafePluginName(name) {
		return Plugin{}, fmt.Errorf("plugin: invalid plugin name %q", name)
	}
	// 只要调用方传入了 TrustStore，签名校验就是强制的，即使条目同时带有 SHA256。
	// 这里先走 TrustStore，以保留原有“缺少签名”等验签错误语义。
	if trustStore != nil {
		manifestData, err := manifestDataForSignature(entry)
		if err != nil {
			return Plugin{}, fmt.Errorf("plugin: marshal manifest for signature %q: %w", name, err)
		}
		if err := trustStore.VerifyWithTrust(manifestData, entry.Signature); err != nil {
			return Plugin{}, fmt.Errorf("plugin: signature verification failed for %q: %w", name, err)
		}
	} else if requireIntegrity {
		// 默认安装策略拒绝没有 SHA256 的条目；没有 TrustStore 时签名无法被实际校验，
		// 因此仅有签名字段也不能视为已校验。
		if strings.TrimSpace(entry.SHA256) == "" {
			return Plugin{}, fmt.Errorf("%w: %s", ErrMarketEntryIntegrityRequired, name)
		}
	}

	pluginRoot := filepath.Join(destDir, name)
	if isGitURL(url) {
		return installFromGit(ctx, entry, pluginRoot)
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return installFromHTTP(ctx, entry, pluginRoot)
	}
	return Plugin{}, fmt.Errorf("plugin: unsupported market URL %q", url)
}

func installFromGit(ctx context.Context, entry MarketIndex, pluginRoot string) (Plugin, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return Plugin{}, fmt.Errorf("plugin: git is required to install %q: %w", entry.Name, err)
	}
	if err := preparePluginRoot(pluginRoot); err != nil {
		return Plugin{}, err
	}

	cmd := exec.CommandContext(ctx, gitPath, "-c", "core.autocrlf=false", "clone", "--depth", "1", "--", entry.URL, pluginRoot)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(pluginRoot)
		return Plugin{}, fmt.Errorf("plugin: git clone %q: %v: %s", entry.Name, err, strings.TrimSpace(string(out)))
	}

	if sha := strings.TrimSpace(entry.SHA256); sha != "" {
		got, err := hashDir(pluginRoot)
		if err != nil {
			_ = os.RemoveAll(pluginRoot)
			return Plugin{}, fmt.Errorf("plugin: hash installed plugin %q: %w", entry.Name, err)
		}
		if !strings.EqualFold(got, sha) {
			_ = os.RemoveAll(pluginRoot)
			return Plugin{}, fmt.Errorf("plugin: SHA256 mismatch for %q: got %s want %s", entry.Name, got, strings.ToLower(sha))
		}
	}
	return loadInstalledPlugin(pluginRoot, entry.Name)
}

func installFromHTTP(ctx context.Context, entry MarketIndex, pluginRoot string) (Plugin, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, entry.URL, nil)
	if err != nil {
		return Plugin{}, fmt.Errorf("plugin: create download request for %q: %w", entry.Name, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Plugin{}, fmt.Errorf("plugin: download %q: %w", entry.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Plugin{}, fmt.Errorf("plugin: download %q: %s", entry.Name, resp.Status)
	}

	tmp, err := os.CreateTemp("", "dsh-plugin-download-*")
	if err != nil {
		return Plugin{}, fmt.Errorf("plugin: create temp download: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxPluginDownloadSize+1))
	if err != nil {
		tmp.Close()
		return Plugin{}, fmt.Errorf("plugin: read download %q: %w", entry.Name, err)
	}
	if n > maxPluginDownloadSize {
		tmp.Close()
		return Plugin{}, fmt.Errorf("plugin: download %q exceeds %d bytes", entry.Name, maxPluginDownloadSize)
	}
	if err := tmp.Close(); err != nil {
		return Plugin{}, fmt.Errorf("plugin: close download %q: %w", entry.Name, err)
	}

	if sha := strings.TrimSpace(entry.SHA256); sha != "" {
		got, err := sha256File(tmpName)
		if err != nil {
			return Plugin{}, fmt.Errorf("plugin: hash download %q: %w", entry.Name, err)
		}
		if !strings.EqualFold(got, sha) {
			return Plugin{}, fmt.Errorf("plugin: SHA256 mismatch for %q: got %s want %s", entry.Name, got, strings.ToLower(sha))
		}
	}

	if err := preparePluginRoot(pluginRoot); err != nil {
		return Plugin{}, err
	}

	kind := archiveKindFromName(entry.URL)
	if kind != "" {
		if err := extractArchiveKind(tmpName, pluginRoot, kind); err != nil {
			_ = os.RemoveAll(pluginRoot)
			return Plugin{}, err
		}
	} else {
		if err := installRawFile(tmpName, pluginRoot, entry); err != nil {
			_ = os.RemoveAll(pluginRoot)
			return Plugin{}, err
		}
	}
	return loadInstalledPlugin(pluginRoot, entry.Name)
}

func installRawFile(src, pluginRoot string, entry MarketIndex) error {
	if err := preparePluginRoot(pluginRoot); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("plugin: read downloaded plugin file: %w", err)
	}
	execName := entry.Name
	if runtime.GOOS == "windows" {
		execName += ".exe"
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, execName), data, 0o755); err != nil {
		return fmt.Errorf("plugin: write plugin executable: %w", err)
	}

	fm := fileManifest{
		Manifest: Manifest{
			Name:        entry.Name,
			Version:     entry.Version,
			Description: entry.Description,
			Commands:    []string{entry.Name},
		},
		Executable: execName,
	}
	mb, err := json.MarshalIndent(fm, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin: marshal generated manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginRoot, entry.Name+ManifestSuffix), mb, 0o644); err != nil {
		return fmt.Errorf("plugin: write generated manifest: %w", err)
	}
	return nil
}

func loadInstalledPlugin(pluginRoot, wantName string) (Plugin, error) {
	manifests, err := findManifests(pluginRoot)
	if err != nil {
		return Plugin{}, err
	}
	if len(manifests) == 0 {
		return Plugin{}, fmt.Errorf("plugin: installed package %q contains no %s manifest", wantName, ManifestSuffix)
	}
	p, err := loadManifest(manifests[0])
	if err != nil {
		return Plugin{}, err
	}
	if p.Manifest.Name == "" {
		return Plugin{}, fmt.Errorf("plugin: installed package %q has empty manifest name", wantName)
	}
	return *p, nil
}

func preparePluginRoot(pluginRoot string) error {
	if err := os.MkdirAll(filepath.Dir(pluginRoot), 0o755); err != nil {
		return fmt.Errorf("plugin: create parent directory: %w", err)
	}
	if err := os.RemoveAll(pluginRoot); err != nil {
		return fmt.Errorf("plugin: clean existing plugin directory: %w", err)
	}
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		return fmt.Errorf("plugin: create plugin directory: %w", err)
	}
	return nil
}

func isGitURL(url string) bool {
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "git://") ||
		strings.HasPrefix(lower, "git+ssh://") ||
		strings.HasPrefix(lower, "ssh://") ||
		strings.HasSuffix(lower, ".git") {
		return true
	}
	if info, err := os.Stat(url); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(url, ".git")); err == nil {
			return true
		}
	}
	return false
}

func isSafePluginName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

func fetchHTTP(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashDir(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func archiveKindFromName(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if i := strings.Index(trimmed, "?"); i >= 0 {
		trimmed = trimmed[:i]
	}
	switch {
	case strings.HasSuffix(trimmed, ".zip"):
		return "zip"
	case strings.HasSuffix(trimmed, ".tar.gz"), strings.HasSuffix(trimmed, ".tgz"):
		return "targz"
	case strings.HasSuffix(trimmed, ".tar"):
		return "tar"
	default:
		return ""
	}
}

func extractArchiveKind(path, destDir, kind string) error {
	switch kind {
	case "zip":
		return extractZip(path, destDir)
	case "tar":
		return extractTar(path, destDir, false)
	case "targz":
		return extractTar(path, destDir, true)
	default:
		return fmt.Errorf("plugin: unsupported archive kind %q", kind)
	}
}

func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return fmt.Errorf("plugin: open zip: %w", err)
	}
	defer r.Close()

	var total int64
	for _, f := range r.File {
		target, err := secureJoin(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("plugin: create zip directory: %w", err)
			}
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("plugin: create zip parent: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("plugin: open zip entry %q: %w", f.Name, err)
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return fmt.Errorf("plugin: create zip file %q: %w", target, err)
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, maxPluginDownloadSize+1))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("plugin: extract zip entry %q: %w", f.Name, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("plugin: close zip file %q: %w", target, closeErr)
		}
		if n > maxPluginDownloadSize {
			return fmt.Errorf("plugin: zip entry %q exceeds %d bytes", f.Name, maxPluginDownloadSize)
		}
		total += n
		if total > maxPluginDownloadSize {
			return fmt.Errorf("plugin: zip archive exceeds %d bytes uncompressed", maxPluginDownloadSize)
		}
	}
	return nil
}

func extractTar(src, destDir string, gzipped bool) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("plugin: open tar: %w", err)
	}
	defer f.Close()

	var r io.Reader = f
	if gzipped {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("plugin: open gzip: %w", err)
		}
		defer gz.Close()
		r = gz
	}

	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("plugin: read tar: %w", err)
		}
		target, err := secureJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("plugin: create tar directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("plugin: create tar parent: %w", err)
			}
			mode := os.FileMode(hdr.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return fmt.Errorf("plugin: create tar file %q: %w", target, err)
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, maxPluginDownloadSize+1))
			closeErr := out.Close()
			if copyErr != nil {
				return fmt.Errorf("plugin: extract tar entry %q: %w", hdr.Name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("plugin: close tar file %q: %w", target, closeErr)
			}
			if n > maxPluginDownloadSize {
				return fmt.Errorf("plugin: tar entry %q exceeds %d bytes", hdr.Name, maxPluginDownloadSize)
			}
			total += n
			if total > maxPluginDownloadSize {
				return fmt.Errorf("plugin: tar archive exceeds %d bytes uncompressed", maxPluginDownloadSize)
			}
		default:
			// 跳过符号链接、设备等特殊条目，避免逃逸或意外行为。
		}
	}
	return nil
}

func secureJoin(base, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("plugin: archive entry name is empty")
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin: archive entry escapes destination: %s", name)
	}
	target := filepath.Join(base, clean)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin: archive entry escapes destination: %s", name)
	}
	return target, nil
}
