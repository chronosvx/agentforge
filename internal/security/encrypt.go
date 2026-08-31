// Package security 提供本地敏感数据保护工具：AES-GCM 加解密、日志脱敏与审计日志。
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Encrypt 使用 AES-GCM 加密 data。
//
// 返回值为 nonce || ciphertext，其中 nonce 为随机生成，长度由 GCM 决定。
// key 必须是 16、24 或 32 字节的 AES 密钥。
func Encrypt(data, key []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("security: create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("security: generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, data, nil), nil
}

// Decrypt 解密 Encrypt 产生的密文。
//
// ciphertext 必须是 nonce || ciphertext 格式，且 key 必须与加密时一致。
func Decrypt(ciphertext, key []byte) ([]byte, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("security: create aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("security: create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("security: ciphertext too short")
	}

	nonce, body := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("security: decrypt: %w", err)
	}
	return plain, nil
}

// LoadKey 解析 hex 或 base64 编码的 AES 密钥。
//
// 支持的编码：hex、标准 base64、无填充标准 base64、URL base64、无填充 URL base64。
// 密钥长度必须是 16、24 或 32 字节。
func LoadKey(hexOrBase64 string) ([]byte, error) {
	s := strings.TrimSpace(hexOrBase64)
	if s == "" {
		return nil, errors.New("security: empty key")
	}

	if decoded, err := hex.DecodeString(s); err == nil && validKeyLen(decoded) {
		return decoded, nil
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

	return nil, errors.New("security: invalid key: must be hex or base64 encoding of a 16, 24, or 32 byte key")
}

func validateKey(key []byte) error {
	if !validKeyLen(key) {
		return fmt.Errorf("security: invalid key length %d: must be 16, 24, or 32 bytes", len(key))
	}
	return nil
}

func validKeyLen(key []byte) bool {
	return len(key) == 16 || len(key) == 24 || len(key) == 32
}
