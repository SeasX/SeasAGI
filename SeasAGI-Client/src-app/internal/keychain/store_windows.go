//go:build windows

package keychain

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows：使用 DPAPI（CryptProtectData / CryptUnprotectData）按当前用户加密，
// 密文以 0600 语义落盘。DPAPI 的密钥由操作系统按用户派生，进程无法直接读取。

var (
	crypt32DLL             = windows.NewLazySystemDLL("crypt32.dll")
	kernel32DLL            = windows.NewLazySystemDLL("kernel32.dll")
	procCryptProtectData   = crypt32DLL.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32DLL.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32DLL.NewProc("LocalFree")
)

func dpapiProtect(plaintext []byte) ([]byte, error) {
	var in, out windows.DataBlob
	if len(plaintext) > 0 {
		in = windows.DataBlob{Size: uint32(len(plaintext)), Data: &plaintext[0]}
	}
	ret, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		0, // CRYPTPROTECT_UI_FORBIDDEN 在部分环境下仍可弹出，这里保持默认无 UI 流程
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return unsafe.Slice(out.Data, int(out.Size)), nil
}

func dpapiUnprotect(ciphertext []byte) ([]byte, error) {
	var in, out windows.DataBlob
	if len(ciphertext) > 0 {
		in = windows.DataBlob{Size: uint32(len(ciphertext)), Data: &ciphertext[0]}
	}
	ret, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0, 0, 0, 0,
		0,
		uintptr(unsafe.Pointer(&out)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return unsafe.Slice(out.Data, int(out.Size)), nil
}

func platformGet(keyID string) (string, error) {
	path, err := entryFilePath(keyID)
	if err != nil {
		return "", err
	}
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read keychain entry %s: %w", keyID, err)
	}
	plaintext, err := dpapiUnprotect(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decrypt keychain entry %s: %w", keyID, err)
	}
	return string(plaintext), nil
}

func platformSave(keyID, secret string) error {
	ciphertext, err := dpapiProtect([]byte(secret))
	if err != nil {
		return err
	}
	path, err := entryFilePath(keyID)
	if err != nil {
		return err
	}
	return writeKeyFile(path, ciphertext)
}

func platformDelete(keyID string) error {
	return fileStoreDelete(keyID)
}
