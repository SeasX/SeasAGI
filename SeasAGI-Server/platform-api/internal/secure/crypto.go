package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func EncryptString(plain string) (string, error) {
	key, err := dataKey(os.Getenv("SEASAGI_DATA_KEY"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	cipherText := gcm.Seal(iv, iv, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func DecryptString(cipherValue string) (string, error) {
	key, err := dataKey(os.Getenv("SEASAGI_DATA_KEY"))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(cipherValue)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid cipher text")
	}
	nonce, cipherBytes := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// dataKey 读取并校验 SEASAGI_DATA_KEY。缺失或过短时直接失败：
// 内置默认密钥等同于公开密钥，任何部署都能离线解密全部凭证。
func dataKey(raw string) ([]byte, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil, fmt.Errorf("SEASAGI_DATA_KEY is required; refusing to use a built-in default key")
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("SEASAGI_DATA_KEY must be at least 32 bytes (got %d)", len(key))
	}
	buf := make([]byte, 32)
	copy(buf, key)
	return buf, nil
}

// normalizeKey 保留旧的补零/截断语义，供测试复用；空值或过短返回错误。
func normalizeKey(key string) ([]byte, error) {
	return dataKey(key)
}
