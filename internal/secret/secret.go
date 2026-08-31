// Package secret 提供 API Key / 敏感凭据的安全存储。
//
// 支持两种后端：
//   - EncryptedFileStore：使用 internal/security 的 AES-GCM 加密后写入
//     <data-dir>/secrets.json，主密钥来自 DSH_SECRET_KEY 或 --secret-key。
//   - KeychainStore：调用操作系统 Keychain / 凭据管理器：
//     macOS 使用 security，Windows 使用 PowerShell + Credential Manager，
//     Linux 使用 secret-tool（libsecret）。
package secret

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/chronosvx/agentforge/internal/security"
)

const (
	// EnvSecretKey 是 EncryptedFileStore 主密钥的环境变量名。
	EnvSecretKey = "DSH_SECRET_KEY"
	// SecretsFileName 是加密凭据文件名。
	SecretsFileName = "secrets.json"
)

// 包级错误。
var (
	// ErrNotFound 表示请求的凭据不存在。
	ErrNotFound = errors.New("secret: not found")
	// ErrUnsupported 表示当前平台或系统工具不支持 Keychain 存储。
	ErrUnsupported = errors.New("secret: system keychain is not supported")
	// ErrNoSecretKey 表示使用 EncryptedFileStore 时未提供主密钥。
	ErrNoSecretKey = errors.New("secret: DSH_SECRET_KEY or --secret-key is required")
)

// Store 是敏感凭据的通用存储接口。
type Store interface {
	// Set 保存或更新一条凭据。
	Set(service, key, value string) error
	// Get 读取一条凭据；不存在时返回 ErrNotFound。
	Get(service, key string) (string, error)
	// Delete 删除一条凭据；不存在时视为成功（幂等删除）。
	Delete(service, key string) error
}

// commandRunner 是 KeychainStore 执行外部命令的抽象，便于测试。
type commandRunner func(name string, args ...string) ([]byte, error)

// defaultRunner 使用 os/exec 执行命令并返回合并输出。
func defaultRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// encryptedFile 是 secrets.json 的磁盘结构。
type encryptedFile struct {
	Services map[string]map[string]string `json:"services"`
}

// EncryptedFileStore 使用 AES-GCM 把凭据加密保存在本地 JSON 文件中。
type EncryptedFileStore struct {
	mu   sync.Mutex
	path string
	key  []byte
}

// NewEncryptedFileStore 创建一个加密文件存储。
//
// masterKey 为空时读取环境变量 DSH_SECRET_KEY。密钥支持 hex 或 base64
// 编码的 16/24/32 字节值。
func NewEncryptedFileStore(dataDir, masterKey string) (*EncryptedFileStore, error) {
	if masterKey == "" {
		masterKey = os.Getenv(EnvSecretKey)
	}
	if masterKey == "" {
		return nil, ErrNoSecretKey
	}
	keyBytes, err := security.LoadKey(masterKey)
	if err != nil {
		return nil, fmt.Errorf("secret: load master key: %w", err)
	}
	if dataDir == "" {
		return nil, errors.New("secret: data dir is empty")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("secret: create data dir: %w", err)
	}
	return &EncryptedFileStore{
		path: filepath.Join(dataDir, SecretsFileName),
		key:  keyBytes,
	}, nil
}

// Set 加密并保存一条凭据。重复 Set 会覆盖旧值。
func (s *EncryptedFileStore) Set(service, key, value string) error {
	if err := validateNames(service, key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return err
	}

	sealed, err := security.Encrypt([]byte(value), s.key)
	if err != nil {
		return fmt.Errorf("secret: encrypt %s/%s: %w", service, key, err)
	}

	if data.Services == nil {
		data.Services = make(map[string]map[string]string)
	}
	if data.Services[service] == nil {
		data.Services[service] = make(map[string]string)
	}
	data.Services[service][key] = base64.StdEncoding.EncodeToString(sealed)

	return s.saveLocked(data)
}

// Get 读取并解密一条凭据。
func (s *EncryptedFileStore) Get(service, key string) (string, error) {
	if err := validateNames(service, key); err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return "", err
	}
	svc := data.Services[service]
	if svc == nil {
		return "", ErrNotFound
	}
	encoded, ok := svc[key]
	if !ok {
		return "", ErrNotFound
	}

	sealed, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("secret: decode %s/%s: %w", service, key, err)
	}
	plain, err := security.Decrypt(sealed, s.key)
	if err != nil {
		return "", fmt.Errorf("secret: decrypt %s/%s: %w", service, key, err)
	}
	return string(plain), nil
}

// Delete 删除一条凭据。不存在时返回 nil。
func (s *EncryptedFileStore) Delete(service, key string) error {
	if err := validateNames(service, key); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.loadLocked()
	if err != nil {
		return err
	}
	svc := data.Services[service]
	if svc == nil {
		return nil
	}
	if _, ok := svc[key]; !ok {
		return nil
	}

	delete(svc, key)
	if len(svc) == 0 {
		delete(data.Services, service)
	}
	return s.saveLocked(data)
}

func (s *EncryptedFileStore) loadLocked() (*encryptedFile, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &encryptedFile{Services: make(map[string]map[string]string)}, nil
		}
		return nil, fmt.Errorf("secret: read secrets file: %w", err)
	}
	if len(raw) == 0 {
		return &encryptedFile{Services: make(map[string]map[string]string)}, nil
	}
	var data encryptedFile
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("secret: decode secrets file: %w", err)
	}
	if data.Services == nil {
		data.Services = make(map[string]map[string]string)
	}
	return &data, nil
}

func (s *EncryptedFileStore) saveLocked(data *encryptedFile) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("secret: create secrets dir: %w", err)
	}

	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("secret: encode secrets file: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".secrets-*")
	if err != nil {
		return fmt.Errorf("secret: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("secret: write temp file: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secret: chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("secret: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("secret: close temp file: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("secret: rename temp file: %w", err)
	}
	return nil
}

// KeychainStore 使用系统 Keychain / 凭据管理器保存凭据。
type KeychainStore struct {
	runner   commandRunner
	platform string // 用于测试覆盖不同平台；为空时使用 runtime.GOOS
}

// NewKeychainStore 创建系统 Keychain 存储，并检查当前平台所需命令是否存在。
func NewKeychainStore() (*KeychainStore, error) {
	return newKeychainStore(runtime.GOOS)
}

func newKeychainStore(goos string) (*KeychainStore, error) {
	if err := checkKeychainTool(goos); err != nil {
		return nil, err
	}
	return &KeychainStore{runner: defaultRunner, platform: goos}, nil
}

func checkKeychainTool(goos string) error {
	switch goos {
	case "darwin":
		if _, err := exec.LookPath("security"); err != nil {
			return fmt.Errorf("%w: macOS requires the security command", ErrUnsupported)
		}
	case "windows":
		if _, err := exec.LookPath("powershell"); err != nil {
			return fmt.Errorf("%w: Windows requires PowerShell", ErrUnsupported)
		}
	case "linux":
		if _, err := exec.LookPath("secret-tool"); err != nil {
			return fmt.Errorf("%w: Linux requires secret-tool (libsecret)", ErrUnsupported)
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, goos)
	}
	return nil
}

// Set 保存一条凭据到系统 Keychain。
func (k *KeychainStore) Set(service, key, value string) error {
	if err := validateNames(service, key); err != nil {
		return err
	}
	switch k.osName() {
	case "darwin":
		// 交互式终端下使用 security 的安全提示模式（-w 作为最后一个参数），
		// 避免密钥出现在进程命令行中；非交互调用只能回退到参数传递。
		if isTerminal(os.Stdin) {
			out, err := k.runner("security", "add-generic-password", "-U", "-a", key, "-s", service, "-w")
			if err != nil {
				return fmt.Errorf("secret: macOS keychain add: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
		out, err := k.runner("security", "add-generic-password", "-U", "-a", key, "-s", service, "-w", value)
		if err != nil {
			return fmt.Errorf("secret: macOS keychain add: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "windows":
		return k.windowsSet(service, key, value)
	case "linux":
		out, err := k.runner("secret-tool", "store", "--label="+service+"/"+key, service, key, value)
		if err != nil {
			return fmt.Errorf("secret: Linux secret-tool store: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, k.osName())
	}
}

// Get 从系统 Keychain 读取一条凭据。
func (k *KeychainStore) Get(service, key string) (string, error) {
	if err := validateNames(service, key); err != nil {
		return "", err
	}
	switch k.osName() {
	case "darwin":
		out, err := k.runner("security", "find-generic-password", "-a", key, "-s", service, "-w")
		if err != nil {
			if isKeychainNotFound(k.osName(), out) {
				return "", ErrNotFound
			}
			return "", fmt.Errorf("secret: macOS keychain find: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	case "windows":
		return k.windowsGet(service, key)
	case "linux":
		out, err := k.runner("secret-tool", "lookup", service, key)
		if err != nil {
			if isKeychainNotFound(k.osName(), out) {
				return "", ErrNotFound
			}
			return "", fmt.Errorf("secret: Linux secret-tool lookup: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return strings.TrimSpace(string(out)), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupported, k.osName())
	}
}

// Delete 从系统 Keychain 删除一条凭据。
func (k *KeychainStore) Delete(service, key string) error {
	if err := validateNames(service, key); err != nil {
		return err
	}
	switch k.osName() {
	case "darwin":
		out, err := k.runner("security", "delete-generic-password", "-a", key, "-s", service)
		if err != nil {
			return fmt.Errorf("secret: macOS keychain delete: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "windows":
		return k.windowsDelete(service, key)
	case "linux":
		out, err := k.runner("secret-tool", "clear", service, key)
		if err != nil {
			return fmt.Errorf("secret: Linux secret-tool clear: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, k.osName())
	}
}

func (k *KeychainStore) osName() string {
	if k.platform != "" {
		return k.platform
	}
	return runtime.GOOS
}

// isKeychainNotFound 根据外部命令输出判断凭据是否不存在。
func isKeychainNotFound(goos string, out []byte) bool {
	text := strings.ToLower(string(out))
	switch goos {
	case "darwin":
		return strings.Contains(text, "could not be found") || strings.Contains(text, "not found")
	case "windows":
		return strings.Contains(text, "secret: not found") || strings.Contains(text, "not found")
	case "linux":
		return strings.TrimSpace(text) == "" || strings.Contains(text, "not found")
	default:
		return false
	}
}

// isTerminal 粗略判断文件描述符是否连接到交互式终端。
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// NewStore 自动选择平台存储后端。
//
// useKeychain 为 true 时使用系统 Keychain；为 false 时使用 EncryptedFileStore
// 并读取 DSH_SECRET_KEY（或由调用方通过 NewEncryptedFileStore 显式传入 --secret-key）。
func NewStore(dataDir string, useKeychain bool) (Store, error) {
	if useKeychain {
		return NewKeychainStore()
	}
	return NewEncryptedFileStore(dataDir, "")
}

func validateNames(service, key string) error {
	if strings.TrimSpace(service) == "" {
		return errors.New("secret: service is empty")
	}
	if strings.TrimSpace(key) == "" {
		return errors.New("secret: key is empty")
	}
	return nil
}
