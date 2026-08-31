//go:build windows

package secret

import (
	"fmt"
	"syscall"
	"unsafe"
)

type nativeCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     uintptr
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
	errCredNotFound = syscall.Errno(1168) // ERROR_NOT_FOUND
)

// windowsSet 使用 Windows Credential Manager 原生 API 保存凭据。
func (k *KeychainStore) windowsSet(service, key, value string) error {
	target := service + ":" + key
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager target: %w", err)
	}
	userPtr, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager user: %w", err)
	}
	valueUTF16, err := syscall.UTF16FromString(value)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager value: %w", err)
	}

	cred := nativeCredential{
		Type:               1, // CRED_TYPE_GENERIC
		TargetName:         targetPtr,
		UserName:           userPtr,
		Persist:            2, // CRED_PERSIST_LOCAL_MACHINE
		CredentialBlobSize: uint32(len(valueUTF16) * 2),
		CredentialBlob:     uintptr(unsafe.Pointer(&valueUTF16[0])),
	}

	r1, _, callErr := procCredWriteW.Call(
		uintptr(unsafe.Pointer(&cred)),
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("secret: Windows Credential Manager set: %w", callErr)
	}
	return nil
}

// windowsGet 使用 Windows Credential Manager 原生 API 读取凭据。
func (k *KeychainStore) windowsGet(service, key string) (string, error) {
	target := service + ":" + key
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return "", fmt.Errorf("secret: Windows Credential Manager target: %w", err)
	}

	var credPtr uintptr
	r1, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		1, // CRED_TYPE_GENERIC
		0,
		uintptr(unsafe.Pointer(&credPtr)),
	)
	if r1 == 0 {
		if callErr == errCredNotFound {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret: Windows Credential Manager get: %w", callErr)
	}
	defer procCredFree.Call(credPtr)

	cred := (*nativeCredential)(unsafe.Pointer(credPtr))
	if cred.CredentialBlobSize == 0 || cred.CredentialBlob == 0 {
		return "", nil
	}
	u := unsafe.Slice((*uint16)(unsafe.Pointer(cred.CredentialBlob)), int(cred.CredentialBlobSize)/2)
	return syscall.UTF16ToString(u), nil
}

// windowsDelete 使用 Windows Credential Manager 原生 API 删除凭据。
func (k *KeychainStore) windowsDelete(service, key string) error {
	target := service + ":" + key
	targetPtr, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager target: %w", err)
	}

	r1, _, callErr := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetPtr)),
		1, // CRED_TYPE_GENERIC
		0,
	)
	if r1 == 0 && callErr != errCredNotFound {
		return fmt.Errorf("secret: Windows Credential Manager delete: %w", callErr)
	}
	return nil
}
