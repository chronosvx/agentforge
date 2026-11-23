package enterprise

import (
	"context"
	"errors"
	"fmt"

	"github.com/chronosvx/agentforge/internal/security"
)

// KeyManager 是密钥管理服务（KMS）的抽象接口。
//
// 当前 LocalKMS 提供本地 AES-GCM 实现；后续可增加云 KMS 实现
// （如 AWS KMS、GCP KMS、Azure Key Vault）并保持该接口不变。
type KeyManager interface {
	// Encrypt 使用 KMS 主密钥加密明文，返回可安全传输/存储的密文。
	Encrypt(ctx context.Context, plain []byte) ([]byte, error)
	// Decrypt 使用 KMS 主密钥解密密文，返回原始明文。
	Decrypt(ctx context.Context, cipher []byte) ([]byte, error)
}

// LocalKMS 使用本地主密钥和 AES-GCM 提供加解密能力。
//
// 主密钥长度必须是 16、24 或 32 字节。适合单机/自托管部署；
// 企业集中管控时可替换为云 KMS 实现。
type LocalKMS struct {
	key []byte
}

// NewLocalKMS 创建本地 KMS，masterKey 必须是 16/24/32 字节 AES 密钥。
func NewLocalKMS(masterKey []byte) (*LocalKMS, error) {
	if len(masterKey) != 16 && len(masterKey) != 24 && len(masterKey) != 32 {
		return nil, fmt.Errorf("enterprise: invalid master key length %d: must be 16, 24, or 32 bytes", len(masterKey))
	}
	key := make([]byte, len(masterKey))
	copy(key, masterKey)
	return &LocalKMS{key: key}, nil
}

// NewLocalKMSFromConfig 从配置字符串创建本地 KMS。
//
// 配置值支持 hex 或 base64 编码的 16/24/32 字节密钥，与 internal/security
// 的 LoadKey 保持一致，便于直接从 DSH_SECRET_KEY / --secret-key 读取。
func NewLocalKMSFromConfig(encoded string) (*LocalKMS, error) {
	key, err := security.LoadKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("enterprise: load kms master key: %w", err)
	}
	return NewLocalKMS(key)
}

// Encrypt 使用 AES-GCM 加密 plain。
//
// 返回值为 nonce || ciphertext，与 internal/security.Encrypt 兼容。
func (k *LocalKMS) Encrypt(ctx context.Context, plain []byte) ([]byte, error) {
	if k == nil || k.key == nil {
		return nil, errors.New("enterprise: local kms is nil")
	}
	if ctx == nil {
		return nil, errors.New("enterprise: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sealed, err := security.Encrypt(plain, k.key)
	if err != nil {
		return nil, fmt.Errorf("enterprise: local kms encrypt: %w", err)
	}
	return sealed, nil
}

// Decrypt 解密 Encrypt 产生的密文。
func (k *LocalKMS) Decrypt(ctx context.Context, cipher []byte) ([]byte, error) {
	if k == nil || k.key == nil {
		return nil, errors.New("enterprise: local kms is nil")
	}
	if ctx == nil {
		return nil, errors.New("enterprise: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plain, err := security.Decrypt(cipher, k.key)
	if err != nil {
		return nil, fmt.Errorf("enterprise: local kms decrypt: %w", err)
	}
	return plain, nil
}
