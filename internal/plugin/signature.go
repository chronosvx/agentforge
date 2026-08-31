// 本文件实现插件市场条目的 ECDSA P-256 签名与信任链校验。
package plugin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Signature 是市场索引条目中的插件签名。
// Signature 字段为 ECDSA P-256 签名结果的 base64 编码。
type Signature struct {
	PublicKeyID string `json:"publicKeyID,omitempty"`
	Signature   string `json:"signature,omitempty"`
}

// IsZero 报告 Signature 是否为空，供自定义 JSON 序列化判断是否省略该字段。
func (s Signature) IsZero() bool {
	return s.PublicKeyID == "" && s.Signature == ""
}

// 签名/信任链相关错误。
var (
	// ErrUnknownPublicKey 表示 TrustStore 中没有对应的可信公钥。
	ErrUnknownPublicKey = errors.New("plugin: unknown public key")
	// ErrMissingSignature 表示需要签名校验时条目缺少签名。
	ErrMissingSignature = errors.New("plugin: missing signature")
	// ErrNoTrustedKeys 表示 TrustStore 为空，无法校验任何签名。
	ErrNoTrustedKeys = errors.New("plugin: no trusted keys")
)

// Sign 使用 ECDSA P-256 私钥对 data 的 SHA-256 摘要签名，返回 base64 编码的 ASN.1 签名。
func Sign(data []byte, privateKeyPEM string) (string, error) {
	priv, err := parseECDSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		return "", fmt.Errorf("plugin: sign data: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// Verify 使用 ECDSA P-256 公钥验证 data 的 SHA-256 摘要签名。
func Verify(data []byte, signature, publicKeyPEM string) error {
	pub, err := parseECDSAPublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return fmt.Errorf("plugin: decode signature: %w", err)
	}
	digest := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(pub, digest[:], sig) {
		return errors.New("plugin: invalid signature")
	}
	return nil
}

// TrustStore 管理一组可信公钥，用于校验插件市场条目的签名。
type TrustStore struct {
	mu   sync.RWMutex
	keys map[string]string
}

// NewTrustStore 返回一个空的 TrustStore。
func NewTrustStore() *TrustStore {
	return &TrustStore{keys: make(map[string]string)}
}

// AddPublicKey 将 PEM 编码的 ECDSA P-256 公钥加入信任库。
// 公钥必须可解析且属于 P-256，否则返回错误。
func (ts *TrustStore) AddPublicKey(id, pem string) error {
	if ts == nil {
		return errors.New("plugin: trust store is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("plugin: trusted public key id cannot be empty")
	}
	if _, err := parseECDSAPublicKey(pem); err != nil {
		return fmt.Errorf("plugin: add trusted public key %q: %w", id, err)
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.keys == nil {
		ts.keys = make(map[string]string)
	}
	ts.keys[id] = strings.TrimSpace(pem)
	return nil
}

// Len 返回信任库中的公钥数量。
func (ts *TrustStore) Len() int {
	if ts == nil {
		return 0
	}
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return len(ts.keys)
}

// VerifyWithTrust 使用 TrustStore 中 PublicKeyID 对应的公钥验证 manifestData 的签名。
func (ts *TrustStore) VerifyWithTrust(manifestData []byte, sig Signature) error {
	if ts == nil {
		return errors.New("plugin: trust store is nil")
	}
	if len(strings.TrimSpace(sig.Signature)) == 0 {
		return ErrMissingSignature
	}
	id := strings.TrimSpace(sig.PublicKeyID)
	ts.mu.RLock()
	pem, ok := ts.keys[id]
	keyCount := len(ts.keys)
	ts.mu.RUnlock()
	if keyCount == 0 {
		return ErrNoTrustedKeys
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownPublicKey, id)
	}
	if err := Verify(manifestData, sig.Signature, pem); err != nil {
		return err
	}
	return nil
}

func parseECDSAPrivateKey(pemData string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemData)))
	if block == nil {
		return nil, errors.New("plugin: decode private key PEM: no block found")
	}
	var (
		key any
		err error
	)
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("plugin: unsupported private key PEM type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: parse private key: %w", err)
	}
	priv, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("plugin: private key is not ECDSA")
	}
	if !isP256(priv.Curve) {
		return nil, errors.New("plugin: private key is not P-256")
	}
	return priv, nil
}

func parseECDSAPublicKey(pemData string) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemData)))
	if block == nil {
		return nil, errors.New("plugin: decode public key PEM: no block found")
	}

	var (
		pub any
		err error
	)
	switch block.Type {
	case "PUBLIC KEY":
		pub, err = x509.ParsePKIXPublicKey(block.Bytes)
	case "EC PUBLIC KEY":
		pub, err = parseECPublicKeySEC1(block.Bytes)
	default:
		return nil, fmt.Errorf("plugin: unsupported public key PEM type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("plugin: parse public key: %w", err)
	}
	ecPub, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("plugin: public key is not ECDSA")
	}
	if !isP256(ecPub.Curve) {
		return nil, errors.New("plugin: public key is not P-256")
	}
	return ecPub, nil
}

// parseECPublicKeySEC1 尝试解析 "EC PUBLIC KEY" PEM 中的 SEC1 格式公钥。
// 部分工具会输出这种格式；标准库没有直接解析函数，这里同时兼容 PKIX 和 SEC1。
func parseECPublicKeySEC1(der []byte) (*ecdsa.PublicKey, error) {
	// 某些实现虽然使用 "EC PUBLIC KEY" 标签，内容仍是 PKIX SubjectPublicKeyInfo。
	if pub, err := x509.ParsePKIXPublicKey(der); err == nil {
		if ecPub, ok := pub.(*ecdsa.PublicKey); ok {
			return ecPub, nil
		}
	}
	// SEC1 ECPoint 是 OCTET STRING 包装的未压缩点。
	var point []byte
	if rest, err := asn1.Unmarshal(der, &point); err != nil || len(rest) != 0 {
		return nil, errors.New("plugin: unsupported EC public key encoding")
	}
	x, y := elliptic.Unmarshal(elliptic.P256(), point)
	if x == nil || y == nil {
		return nil, errors.New("plugin: invalid EC public key point")
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func isP256(curve elliptic.Curve) bool {
	return curve != nil && curve.Params().Name == elliptic.P256().Params().Name
}
