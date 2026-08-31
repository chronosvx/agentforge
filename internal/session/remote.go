// Package session 的远程存储适配器实现。
//
// 支持 HTTP/对象存储风格的远程会话存储，并可为任意 RemoteStore 增加
// AES-GCM 加密能力。
package session

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chronosvx/agentforge/internal/security"
)

const (
	// maxRemoteSessionSize 限制单个远程会话下载大小，避免恶意远端耗尽内存。
	maxRemoteSessionSize = 50 << 20 // 50 MiB
	// maxRemoteListSize 限制远程会话 ID 列表响应大小。
	maxRemoteListSize = 10 << 20 // 10 MiB
)

// RemoteStore 表示一个可远程存取会话数据的存储后端。
type RemoteStore interface {
	// Put 上传一个会话的原始数据。
	Put(ctx context.Context, id string, data []byte) error
	// Get 下载一个会话的原始数据。
	Get(ctx context.Context, id string) ([]byte, error)
	// List 返回远程存储中全部会话 ID。
	List(ctx context.Context) ([]string, error)
}

// HTTPStore 是基于 HTTP 的简单 RemoteStore。
//
// baseURL 形如 https://host/sessions。Put/Get 使用
// https://host/sessions/{id}，List 使用 https://host/sessions?list=1，
// 期望返回 JSON 字符串数组（会话 ID）。
type HTTPStore struct {
	base   string
	client *http.Client
}

// NewHTTPStore 创建一个 HTTPStore。baseURL 末尾的斜杠会被去掉。
func NewHTTPStore(baseURL string) *HTTPStore {
	return NewHTTPStoreWithClient(baseURL, nil)
}

// NewHTTPStoreWithClient 允许注入自定义 HTTP client，便于测试。
func NewHTTPStoreWithClient(baseURL string, client *http.Client) *HTTPStore {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &HTTPStore{
		base:   strings.TrimRight(baseURL, "/"),
		client: client,
	}
}

// Put 上传数据到 https://host/sessions/{id}。
func (s *HTTPStore) Put(ctx context.Context, id string, data []byte) error {
	if err := validateID(id); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.url(id), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("session: put %s: new request: %w", id, err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("session: put %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("session: put %s: http %d: %s", id, resp.StatusCode, security.Redact(strings.TrimSpace(string(body))))
	}
	return nil
}

// Get 从 https://host/sessions/{id} 下载数据。
func (s *HTTPStore) Get(ctx context.Context, id string) ([]byte, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(id), nil)
	if err != nil {
		return nil, fmt.Errorf("session: get %s: new request: %w", id, err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("session: get %s: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("session: get %s: not found", id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("session: get %s: http %d: %s", id, resp.StatusCode, security.Redact(strings.TrimSpace(string(body))))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteSessionSize+1))
	if err != nil {
		return nil, fmt.Errorf("session: get %s: read body: %w", id, err)
	}
	if int64(len(data)) > maxRemoteSessionSize {
		return nil, fmt.Errorf("session: get %s: response exceeds %d bytes", id, maxRemoteSessionSize)
	}
	return data, nil
}

// List 使用 https://host/sessions?list=1 获取会话 ID 列表。
func (s *HTTPStore) List(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.listURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("session: list: new request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("session: list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("session: list: http %d: %s", resp.StatusCode, security.Redact(strings.TrimSpace(string(body))))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteListSize+1))
	if err != nil {
		return nil, fmt.Errorf("session: list: read response: %w", err)
	}
	if int64(len(data)) > maxRemoteListSize {
		return nil, fmt.Errorf("session: list: response exceeds %d bytes", maxRemoteListSize)
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("session: list: decode response: %w", err)
	}
	return ids, nil
}

func (s *HTTPStore) url(id string) string {
	return s.base + "/" + url.PathEscape(id)
}

func (s *HTTPStore) listURL() string {
	u, err := url.Parse(s.base)
	if err != nil {
		return s.base + "?list=1"
	}
	q := u.Query()
	q.Set("list", "1")
	u.RawQuery = q.Encode()
	return u.String()
}

// RemoteEncryptedStore 包装任意 RemoteStore，使用 AES-GCM 加密后再上传。
//
// 密钥来自 NewRemoteEncryptedStore 的 key 参数；key 为空时读取环境变量
// DSH_SYNC_KEY。密钥支持 hex 或 base64 编码的 16/24/32 字节值。
type RemoteEncryptedStore struct {
	remote RemoteStore
	key    []byte
}

// NewRemoteEncryptedStore 创建一个加密 RemoteStore 包装器。
func NewRemoteEncryptedStore(remote RemoteStore, key string) (*RemoteEncryptedStore, error) {
	if remote == nil {
		return nil, errors.New("session: remote encrypted store: remote is nil")
	}
	if key == "" {
		key = os.Getenv("DSH_SYNC_KEY")
	}
	if key == "" {
		return nil, errors.New("session: remote encrypted store: DSH_SYNC_KEY or key is required")
	}
	keyBytes, err := decodeSyncKey(key)
	if err != nil {
		return nil, err
	}
	return &RemoteEncryptedStore{remote: remote, key: keyBytes}, nil
}

// Put 加密并上传数据。
func (s *RemoteEncryptedStore) Put(ctx context.Context, id string, data []byte) error {
	sealed, err := s.seal(data)
	if err != nil {
		return err
	}
	if err := s.remote.Put(ctx, id, sealed); err != nil {
		return fmt.Errorf("session: remote encrypted put %s: %w", id, err)
	}
	return nil
}

// Get 下载并解密数据。
func (s *RemoteEncryptedStore) Get(ctx context.Context, id string) ([]byte, error) {
	sealed, err := s.remote.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	plain, err := s.open(sealed)
	if err != nil {
		return nil, fmt.Errorf("session: remote encrypted get %s: %w", id, err)
	}
	return plain, nil
}

// List 透传底层 RemoteStore 的列表。
func (s *RemoteEncryptedStore) List(ctx context.Context) ([]string, error) {
	return s.remote.List(ctx)
}

func (s *RemoteEncryptedStore) seal(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("session: create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("session: create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("session: generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func (s *RemoteEncryptedStore) open(sealed []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("session: create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("session: create gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, errors.New("session: ciphertext too short")
	}
	nonce, ciphertext := sealed[:nonceSize], sealed[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("session: decrypt: %w", err)
	}
	return plain, nil
}

// decodeSyncKey 解析 hex 或 base64 编码的 AES 密钥。
func decodeSyncKey(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("session: empty sync key")
	}

	if decoded, err := hex.DecodeString(s); err == nil {
		if validKeyLen(decoded) {
			return decoded, nil
		}
	}

	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		decoded, err := enc.DecodeString(s)
		if err == nil && validKeyLen(decoded) {
			return decoded, nil
		}
	}

	return nil, errors.New("session: invalid sync key: must be hex or base64 encoding of a 16, 24, or 32 byte key")
}

func validKeyLen(key []byte) bool {
	return len(key) == 16 || len(key) == 24 || len(key) == 32
}
