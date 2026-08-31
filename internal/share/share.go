// Package share 将会话打包成可分享的加密链接或明文链接。
//
// 链接格式为 dsh-share://<payload>，其中 payload 是 base64 URL 编码的
// JSON 信封：
//
// {"v":1,"plain":false,"data":"<base64url(AES-GCM 密文)>"}
//
// 未提供密钥时允许明文分享，此时 plain=true 且 data 为会话 JSON 的
// base64 URL 编码。
package share

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/chronosvx/agentforge/internal/security"
	"github.com/chronosvx/agentforge/internal/session"
)

// Scheme 是可分享会话链接的 URL scheme。
const Scheme = "dsh-share://"

// EnvKey 是分享密钥的环境变量名。
const EnvKey = "DSH_SHARE_KEY"

// envelope 是 dsh-share:// 链接内部承载的 JSON 信封。
type envelope struct {
	Version int    `json:"v"`
	Plain   bool   `json:"plain"`
	Data    string `json:"data"`
}

// ShareSession 将会话编码为可分享链接。
//
// secretKey 为空时尝试读取环境变量 DSH_SHARE_KEY；仍未提供密钥时生成
// 明文分享链接（plain=true）。非空时使用 AES-GCM 加密，key 必须是
// 16、24 或 32 字节。
func ShareSession(entry session.Entry, secretKey []byte) (string, error) {
	secretKey, err := loadKey(secretKey)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return "", fmt.Errorf("share: encode session: %w", err)
	}

	env := envelope{Version: 1}
	if len(secretKey) == 0 {
		env.Plain = true
		env.Data = base64.RawURLEncoding.EncodeToString(data)
	} else {
		sealed, err := security.Encrypt(data, secretKey)
		if err != nil {
			return "", fmt.Errorf("share: encrypt session: %w", err)
		}
		env.Plain = false
		env.Data = base64.RawURLEncoding.EncodeToString(sealed)
	}

	payload, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("share: encode payload: %w", err)
	}

	return Scheme + base64.RawURLEncoding.EncodeToString(payload), nil
}

// OpenShare 解析并解密分享链接。
//
// payload 可以带 dsh-share:// 前缀，也可以只传前缀后面的部分。
// 加密链接必须提供正确的 secretKey；secretKey 为空时读取环境变量
// DSH_SHARE_KEY。明文链接无需密钥。
func OpenShare(payload string, secretKey []byte) (*session.Entry, error) {
	payload = strings.TrimSpace(payload)
	payload = strings.TrimPrefix(payload, Scheme)
	if payload == "" {
		return nil, errors.New("share: empty payload")
	}

	raw, err := decodeBase64(payload)
	if err != nil {
		return nil, fmt.Errorf("share: decode payload: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("share: decode envelope: %w", err)
	}
	if env.Version != 1 {
		return nil, fmt.Errorf("share: unsupported version %d", env.Version)
	}
	if env.Data == "" {
		return nil, errors.New("share: payload data is empty")
	}

	var plain []byte
	if env.Plain {
		plain, err = decodeBase64(env.Data)
		if err != nil {
			return nil, fmt.Errorf("share: decode plaintext data: %w", err)
		}
	} else {
		secretKey, err = loadKey(secretKey)
		if err != nil {
			return nil, err
		}
		if len(secretKey) == 0 {
			return nil, errors.New("share: encrypted payload requires a key (--key or DSH_SHARE_KEY)")
		}
		sealed, err := decodeBase64(env.Data)
		if err != nil {
			return nil, fmt.Errorf("share: decode encrypted data: %w", err)
		}
		plain, err = security.Decrypt(sealed, secretKey)
		if err != nil {
			return nil, fmt.Errorf("share: decrypt session: %w", err)
		}
	}

	var entry session.Entry
	if err := json.Unmarshal(plain, &entry); err != nil {
		return nil, fmt.Errorf("share: decode session: %w", err)
	}
	return &entry, nil
}

// loadKey 优先使用调用方传入的密钥，其次读取 DSH_SHARE_KEY 环境变量。
// 两者都未提供时返回 nil，表示明文模式。
func loadKey(secretKey []byte) ([]byte, error) {
	if len(secretKey) > 0 {
		return secretKey, nil
	}
	env := os.Getenv(EnvKey)
	if env == "" {
		return nil, nil
	}
	key, err := security.LoadKey(env)
	if err != nil {
		return nil, fmt.Errorf("share: load key from %s: %w", EnvKey, err)
	}
	return key, nil
}

func decodeBase64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if decoded, err := enc.DecodeString(s); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}
