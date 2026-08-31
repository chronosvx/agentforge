//go:build !windows

package secret

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// windowsSet 通过 PowerShell 调用 Windows Credential Manager 保存凭据。
func (k *KeychainStore) windowsSet(service, key, value string) error {
	script := windowsCredentialScript("set", service, key, value)
	out, err := k.runner("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager set: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// windowsGet 通过 PowerShell 调用 Windows Credential Manager 读取凭据。
func (k *KeychainStore) windowsGet(service, key string) (string, error) {
	script := windowsCredentialScript("get", service, key, "")
	out, err := k.runner("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		if isKeychainNotFound("windows", out) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secret: Windows Credential Manager get: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// windowsDelete 通过 PowerShell 调用 Windows Credential Manager 删除凭据。
func (k *KeychainStore) windowsDelete(service, key string) error {
	script := windowsCredentialScript("delete", service, key, "")
	out, err := k.runner("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return fmt.Errorf("secret: Windows Credential Manager delete: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// windowsCredentialScript 生成 PowerShell P/Invoke 脚本，通过 base64 传递参数，
// 避免引号转义和命令注入。
func windowsCredentialScript(action, service, key, value string) string {
	serviceB64 := b64(service)
	keyB64 := b64(key)
	valueB64 := b64(value)
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public static class CredMan {
    [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Unicode)]
    public struct CREDENTIAL {
        public uint Flags;
        public uint Type;
        public IntPtr TargetName;
        public IntPtr Comment;
        public System.Runtime.InteropServices.ComTypes.FILETIME LastWritten;
        public uint CredentialBlobSize;
        public IntPtr CredentialBlob;
        public uint Persist;
        public uint AttributeCount;
        public IntPtr Attributes;
        public IntPtr TargetAlias;
        public IntPtr UserName;
    }
    [DllImport("advapi32.dll", SetLastError=true)]
    public static extern bool CredRead(string target, uint type, uint reservedFlag, out IntPtr credential);
    [DllImport("advapi32.dll", SetLastError=true)]
    public static extern bool CredWrite(ref CREDENTIAL credential, uint flags);
    [DllImport("advapi32.dll", SetLastError=true)]
    public static extern bool CredDelete(string target, uint type, uint flags);
    [DllImport("advapi32.dll", SetLastError=true)]
    public static extern bool CredFree(IntPtr buffer);
}
"@
function Decode-B64($s) {
    return [System.Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($s))
}
$action = '%s'
$service = Decode-B64 '%s'
$key = Decode-B64 '%s'
$value = Decode-B64 '%s'
$target = $service + ':' + $key
if ($action -eq 'set') {
    $cred = New-Object CredMan+CREDENTIAL
    $cred.Type = 1
    $cred.TargetName = [Runtime.InteropServices.Marshal]::StringToHGlobalUni($target)
    $cred.UserName = $key
    $cred.Persist = 2
    $cred.CredentialBlob = [Runtime.InteropServices.Marshal]::StringToHGlobalUni($value)
    $cred.CredentialBlobSize = $value.Length * 2
    if (-not [CredMan]::CredWrite([ref]$cred, 0)) {
        $err = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        throw "CredWrite failed: $err"
    }
    [Runtime.InteropServices.Marshal]::FreeHGlobal($cred.TargetName) | Out-Null
    [Runtime.InteropServices.Marshal]::FreeHGlobal($cred.CredentialBlob) | Out-Null
} elseif ($action -eq 'get') {
    $ptr = [IntPtr]::Zero
    if (-not [CredMan]::CredRead($target, 1, 0, [ref]$ptr)) {
        $err = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        if ($err -eq 1168) { Write-Error 'secret: not found'; exit 2 }
        throw "CredRead failed: $err"
    }
    $cred = [Runtime.InteropServices.Marshal]::PtrToStructure($ptr, [type][CredMan+CREDENTIAL])
    $bytes = New-Object byte[] $cred.CredentialBlobSize
    [Runtime.InteropServices.Marshal]::Copy($cred.CredentialBlob, $bytes, 0, $cred.CredentialBlobSize)
    [CredMan]::CredFree($ptr) | Out-Null
    [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
    Write-Output ([System.Text.Encoding]::Unicode.GetString($bytes))
} elseif ($action -eq 'delete') {
    if (-not [CredMan]::CredDelete($target, 1, 0)) {
        $err = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
        if ($err -ne 1168) { throw "CredDelete failed: $err" }
    }
} else {
    throw "Unknown action"
}
`, action, serviceB64, keyB64, valueB64)
}
