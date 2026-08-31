// 本文件实现插件市场条目的签名发布流程：计算包 SHA256、签名规范 JSON，并把条目写入市场索引。
package plugin

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// MarketIndexEntry 是发布流程使用的市场条目类型。
// 为保持与现有市场索引/安装代码兼容，这里直接复用 MarketIndex。
type MarketIndexEntry = MarketIndex

// PublishEntry 是准备发布到市场索引的条目。
// 它与 MarketIndex 字段一致，但由 PreparePublish 生成，包含完整的 SHA256 与签名。
type PublishEntry struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	SHA256      string    `json:"sha256,omitempty"`
	Signature   Signature `json:"signature,omitempty"`
}

// MarshalJSON 在签名为空时省略 signature 字段，保证签名/发布流程中不会出现空签名对象。
func (p PublishEntry) MarshalJSON() ([]byte, error) {
	var sig *Signature
	if !p.Signature.IsZero() {
		s := p.Signature
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
		Name:        p.Name,
		Version:     p.Version,
		Description: p.Description,
		URL:         p.URL,
		SHA256:      p.SHA256,
		Signature:   sig,
	})
}

// PreparePublish 根据市场条目与 ECDSA P-256 私钥生成可发布的 PublishEntry。
// 它会从 entry.URL 指向的包地址计算 SHA256（支持 http(s)://、file:// 与本地文件路径），
// 使用去掉 signature 字段后的规范 JSON 生成签名，并自动从私钥派生 PublicKeyID。
func PreparePublish(entry MarketIndexEntry, privateKeyPEM string) (PublishEntry, error) {
	name := strings.TrimSpace(entry.Name)
	version := strings.TrimSpace(entry.Version)
	url := strings.TrimSpace(entry.URL)
	if name == "" {
		return PublishEntry{}, errors.New("plugin: publish entry name is empty")
	}
	if version == "" {
		return PublishEntry{}, errors.New("plugin: publish entry version is empty")
	}
	if url == "" {
		return PublishEntry{}, errors.New("plugin: publish entry url is empty")
	}

	sha, err := hashPackageSource(url)
	if err != nil {
		return PublishEntry{}, fmt.Errorf("plugin: hash package %q: %w", url, err)
	}

	unsigned := entry
	unsigned.Name = name
	unsigned.Version = version
	unsigned.URL = url
	unsigned.Description = strings.TrimSpace(entry.Description)
	unsigned.SHA256 = sha
	unsigned.Signature = Signature{}
	manifestData, err := manifestDataForSignature(unsigned)
	if err != nil {
		return PublishEntry{}, fmt.Errorf("plugin: marshal manifest for signature: %w", err)
	}

	sig, err := Sign(manifestData, privateKeyPEM)
	if err != nil {
		return PublishEntry{}, fmt.Errorf("plugin: sign manifest: %w", err)
	}
	keyID, err := publicKeyIDFromPrivateKey(privateKeyPEM)
	if err != nil {
		return PublishEntry{}, fmt.Errorf("plugin: derive public key id: %w", err)
	}

	return PublishEntry{
		Name:        name,
		Version:     version,
		Description: strings.TrimSpace(entry.Description),
		URL:         url,
		SHA256:      sha,
		Signature: Signature{
			PublicKeyID: keyID,
			Signature:   sig,
		},
	}, nil
}

// AppendToIndex 把 PublishEntry 追加或更新到市场索引 JSON 文件。
// 支持两种已有格式：顶层数组，或 {"plugins":[...]}；文件不存在时创建 {"plugins":[...]}。
// 同名条目会被更新，避免重复。
func AppendToIndex(indexPath string, entry PublishEntry) error {
	indexPath = strings.TrimSpace(indexPath)
	if indexPath == "" {
		return errors.New("plugin: market index path is empty")
	}
	if strings.TrimSpace(entry.Name) == "" {
		return errors.New("plugin: publish entry name is empty")
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("plugin: read market index: %w", err)
		}
		data = nil
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return writeMarketIndex(indexPath, marketIndexFile{Plugins: []PublishEntry{entry}})
	}

	switch trimmed[0] {
	case '[':
		var entries []PublishEntry
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return fmt.Errorf("plugin: parse market index array: %w", err)
		}
		return writeMarketIndex(indexPath, upsertPublishEntry(entries, entry))
	case '{':
		var file marketIndexFile
		if err := json.Unmarshal(trimmed, &file); err != nil {
			return fmt.Errorf("plugin: parse market index object: %w", err)
		}
		return writeMarketIndex(indexPath, marketIndexFile{Plugins: upsertPublishEntry(file.Plugins, entry)})
	default:
		return errors.New("plugin: unsupported market index format")
	}
}

type marketIndexFile struct {
	Plugins []PublishEntry `json:"plugins"`
}

func upsertPublishEntry(entries []PublishEntry, entry PublishEntry) []PublishEntry {
	for i := range entries {
		if entries[i].Name == entry.Name {
			entries[i] = entry
			return entries
		}
	}
	return append(entries, entry)
}

func writeMarketIndex(indexPath string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin: marshal market index: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return fmt.Errorf("plugin: write market index: %w", err)
	}
	return nil
}

// hashPackageSource 计算插件包文件的 SHA-256。
// 支持 http(s)://、file:// 与本地文件路径。
func hashPackageSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("plugin: package source is empty")
	}

	switch {
	case strings.HasPrefix(source, "http://"), strings.HasPrefix(source, "https://"):
		resp, err := http.Get(source)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("HTTP %s", resp.Status)
		}
		h := sha256.New()
		n, err := io.Copy(h, io.LimitReader(resp.Body, maxPluginDownloadSize+1))
		if err != nil {
			return "", err
		}
		if n > maxPluginDownloadSize {
			return "", fmt.Errorf("plugin package exceeds %d bytes", maxPluginDownloadSize)
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	case strings.HasPrefix(source, "file://"):
		return sha256File(strings.TrimPrefix(source, "file://"))
	default:
		return sha256File(source)
	}
}

// publicKeyIDFromPrivateKey 从 ECDSA P-256 私钥派生公钥 ID。
// ID 使用公钥 DER 编码的 SHA-256 指纹，带 "sha256:" 前缀。
func publicKeyIDFromPrivateKey(privateKeyPEM string) (string, error) {
	priv, err := parseECDSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return "", fmt.Errorf("plugin: marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
