//go:build linux && cgo && libsecret

package secret

/*
#cgo pkg-config: libsecret-1 glib-2.0
#include <libsecret/secret.h>
#include <stdlib.h>

static char* agentforge_g_error_message(GError *err) {
    if (err == NULL || err->message == NULL) {
        return NULL;
    }
    return err->message;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// linuxSet 使用 libsecret 原生 API 保存凭据，避免 secret-tool 命令行参数暴露密钥。
func (k *KeychainStore) linuxSet(service, key, value string) error {
	cService := C.CString(service)
	cKey := C.CString(key)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cKey))
	defer C.free(unsafe.Pointer(cValue))

	attrs := newLinuxAttrs(cService, cKey)
	if attrs == nil {
		return fmt.Errorf("secret: Linux libsecret: failed to create attributes")
	}
	defer C.g_hash_table_unref(attrs)

	var err *C.GError
	ok := C.secret_password_store_sync(nil, attrs, nil, &err, cValue, nil)
	if !ok {
		msg := linuxGError(err)
		return fmt.Errorf("secret: Linux libsecret store: %s", msg)
	}
	return nil
}

// linuxGet 使用 libsecret 原生 API 读取凭据。
func (k *KeychainStore) linuxGet(service, key string) (string, error) {
	cService := C.CString(service)
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cKey))

	attrs := newLinuxAttrs(cService, cKey)
	if attrs == nil {
		return "", fmt.Errorf("secret: Linux libsecret: failed to create attributes")
	}
	defer C.g_hash_table_unref(attrs)

	var err *C.GError
	password := C.secret_password_lookup_sync(nil, attrs, nil, &err)
	if password == nil {
		msg := linuxGError(err)
		if msg == "" {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret: Linux libsecret lookup: %s", msg)
	}
	defer C.secret_password_free(password)
	return C.GoString(password), nil
}

// linuxDelete 使用 libsecret 原生 API 删除凭据。
func (k *KeychainStore) linuxDelete(service, key string) error {
	cService := C.CString(service)
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cKey))

	attrs := newLinuxAttrs(cService, cKey)
	if attrs == nil {
		return fmt.Errorf("secret: Linux libsecret: failed to create attributes")
	}
	defer C.g_hash_table_unref(attrs)

	var err *C.GError
	ok := C.secret_password_clear_sync(nil, attrs, nil, &err)
	if !ok {
		msg := linuxGError(err)
		return fmt.Errorf("secret: Linux libsecret clear: %s", msg)
	}
	return nil
}

func newLinuxAttrs(cService, cKey *C.char) *C.GHashTable {
	attrs := C.g_hash_table_new_full(C.g_str_hash(), C.g_str_equal(), C.g_free, C.g_free)
	if attrs == nil {
		return nil
	}
	C.g_hash_table_insert(attrs, unsafe.Pointer(C.g_strdup(cService)), unsafe.Pointer(C.g_strdup(cKey)))
	return attrs
}

func linuxGError(err *C.GError) string {
	if err == nil {
		return ""
	}
	msg := C.agentforge_g_error_message(err)
	if msg == nil {
		C.g_error_free(err)
		return ""
	}
	text := C.GoString(msg)
	C.g_error_free(err)
	return text
}
