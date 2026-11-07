package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 非 macOS 平台的持久化实现：凭证以密文落盘，绝不再写入进程环境变量。
// 文件与目录权限固定为 0600 / 0700；读取失败（文件损坏、解密失败、权限不足）
// 一律返回错误，由上层决定是否视为「未配置」。
const (
	keychainDirMode  os.FileMode = 0o700
	keychainFileMode os.FileMode = 0o600
	masterKeySize                = 32
)

// keychainRoot 返回凭证存储根目录。SEASAGI_KEYCHAIN_DIR 仅用于测试/便携部署，
// 默认 ~/.seasagi/keychain。
func keychainRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv("SEASAGI_KEYCHAIN_DIR")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".seasagi", "keychain"), nil
}

// entryFilePath 用 keyID 的 SHA-256 作为文件名，避免路径穿越与特殊字符问题。
func entryFilePath(keyID string) (string, error) {
	root, err := keychainRoot()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(keyID))
	return filepath.Join(root, "entries", hex.EncodeToString(sum[:])+".bin"), nil
}

func masterKeyPath() (string, error) {
	root, err := keychainRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "master.key"), nil
}

// loadMasterKey 仅读取已存在的主密钥；不存在时返回错误。
func loadMasterKey() ([]byte, error) {
	path, err := masterKeyPath()
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read keychain master key: %w", err)
	}
	if len(key) != masterKeySize {
		return nil, fmt.Errorf("invalid keychain master key length: %d", len(key))
	}
	return key, nil
}

// loadOrCreateMasterKey 读取主密钥，不存在时生成并以 0600 独占创建。
func loadOrCreateMasterKey() ([]byte, error) {
	key, err := loadMasterKey()
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	root, err := keychainRoot()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, keychainDirMode); err != nil {
		return nil, fmt.Errorf("create keychain dir: %w", err)
	}

	key = make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate keychain master key: %w", err)
	}

	path, err := masterKeyPath()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, keychainFileMode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadMasterKey()
		}
		return nil, fmt.Errorf("create keychain master key: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(key); err != nil {
		return nil, fmt.Errorf("write keychain master key: %w", err)
	}
	return key, nil
}

// encryptEntry 使用 AES-256-GCM 加密单条凭证，keyID 作为附加认证数据，
// 防止密文在不同条目之间被挪用。
func encryptEntry(keyID string, plaintext, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, []byte(keyID)), nil
}

func decryptEntry(keyID string, ciphertext, masterKey []byte) ([]byte, error) {
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("init aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("keychain entry is truncated")
	}
	nonce, sealed := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, sealed, []byte(keyID))
	if err != nil {
		return nil, fmt.Errorf("decrypt keychain entry: %w", err)
	}
	return plaintext, nil
}

// writeKeyFile 以 0600 原子写入（同目录临时文件 + rename）。
func writeKeyFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, keychainDirMode); err != nil {
		return fmt.Errorf("create keychain entry dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".entry-*")
	if err != nil {
		return fmt.Errorf("create temp keychain entry: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(keychainFileMode); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp keychain entry: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp keychain entry: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp keychain entry: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp keychain entry: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace keychain entry: %w", err)
	}
	return nil
}

// fileStoreSave 使用 AES-256-GCM 加密后落盘。
func fileStoreSave(keyID, secret string) error {
	masterKey, err := loadOrCreateMasterKey()
	if err != nil {
		return err
	}
	ciphertext, err := encryptEntry(keyID, []byte(secret), masterKey)
	if err != nil {
		return err
	}
	path, err := entryFilePath(keyID)
	if err != nil {
		return err
	}
	return writeKeyFile(path, ciphertext)
}

// fileStoreGet 读取并解密；任何失败都返回错误。
func fileStoreGet(keyID string) (string, error) {
	masterKey, err := loadMasterKey()
	if err != nil {
		return "", err
	}
	path, err := entryFilePath(keyID)
	if err != nil {
		return "", err
	}
	ciphertext, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read keychain entry %s: %w", keyID, err)
	}
	plaintext, err := decryptEntry(keyID, ciphertext, masterKey)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// fileStoreDelete 删除条目；不存在时视为成功。
func fileStoreDelete(keyID string) error {
	path, err := entryFilePath(keyID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete keychain entry %s: %w", keyID, err)
	}
	return nil
}
