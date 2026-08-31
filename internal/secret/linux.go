package secret

import (
	"fmt"
	"strings"
)

// linuxSet 通过 secret-tool 保存凭据。
//
// TODO: 在具备 Linux + libsecret 开发环境的机器上接入 libsecret 原生 API，
// 避免密钥出现在 secret-tool 命令行参数中。
func (k *KeychainStore) linuxSet(service, key, value string) error {
	out, err := k.runner("secret-tool", "store", "--label="+service+"/"+key, service, key, value)
	if err != nil {
		return fmt.Errorf("secret: Linux secret-tool store: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// linuxGet 通过 secret-tool 读取凭据。
func (k *KeychainStore) linuxGet(service, key string) (string, error) {
	out, err := k.runner("secret-tool", "lookup", service, key)
	if err != nil {
		if isKeychainNotFound(k.osName(), out) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret: Linux secret-tool lookup: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// linuxDelete 通过 secret-tool 删除凭据。
func (k *KeychainStore) linuxDelete(service, key string) error {
	out, err := k.runner("secret-tool", "clear", service, key)
	if err != nil {
		return fmt.Errorf("secret: Linux secret-tool clear: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
