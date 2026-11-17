package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

// ciphertextV2Prefix 标记使用 HKDF 派生密钥加密的新密文。
// 旧密文无前缀，继续走历史派生路径解密，保证升级不丢数据。
const ciphertextV2Prefix = "v2:"

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
	return ciphertextV2Prefix + base64.StdEncoding.EncodeToString(cipherText), nil
}

func DecryptString(cipherValue string) (string, error) {
	raw := strings.TrimSpace(cipherValue)
	var key []byte
	if strings.HasPrefix(raw, ciphertextV2Prefix) {
		k, err := dataKey(os.Getenv("SEASAGI_DATA_KEY"))
		if err != nil {
			return "", err
		}
		key = k
		raw = strings.TrimPrefix(raw, ciphertextV2Prefix)
	} else {
		// 历史密文：沿用旧的截断/补零派生，避免升级后既有凭证无法解密。
		k, err := legacyDataKey(os.Getenv("SEASAGI_DATA_KEY"))
		if err != nil {
			return "", err
		}
		key = k
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(raw)
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

// dataKey 读取 SEASAGI_DATA_KEY 并用 HKDF-SHA256 派生 32 字节 AES 密钥。
//
// 旧实现直接截断前 32 字节，密钥长度外的熵被浪费、短口令也会被静默补齐；
// 现按 RFC 5869 执行 Extract + Expand，salt 与 info 均绑定用途，避免跨场景复用。
func dataKey(raw string) ([]byte, error) {
	if err := validateSecret(raw); err != nil {
		return nil, err
	}
	return hkdfSHA256(
		[]byte(strings.TrimSpace(raw)),
		[]byte("seasagi/data-key/salt/v1"),
		[]byte("seasagi/data-key/aes-gcm/v1"),
		32,
	), nil
}

// legacyDataKey 保留升级前的派生语义，仅用于解密历史密文。
func legacyDataKey(raw string) ([]byte, error) {
	if err := validateSecret(raw); err != nil {
		return nil, err
	}
	buf := make([]byte, 32)
	copy(buf, strings.TrimSpace(raw))
	return buf, nil
}

// validateSecret 校验密钥存在且不短于 32 字节；空密钥等同于公开密钥，必须拒绝。
func validateSecret(raw string) error {
	key := strings.TrimSpace(raw)
	if key == "" {
		return fmt.Errorf("SEASAGI_DATA_KEY is required; refusing to use a built-in default key")
	}
	if len(key) < 32 {
		return fmt.Errorf("SEASAGI_DATA_KEY must be at least 32 bytes (got %d)", len(key))
	}
	return nil
}

// hkdfSHA256 实现 RFC 5869 的 HKDF（Extract + Expand）。
func hkdfSHA256(ikm, salt, info []byte, length int) []byte {
	extract := hmac.New(sha256.New, salt)
	extract.Write(ikm)
	prk := extract.Sum(nil)

	out := make([]byte, 0, length)
	var block []byte
	for counter := byte(1); len(out) < length; counter++ {
		expand := hmac.New(sha256.New, prk)
		expand.Write(block)
		expand.Write(info)
		expand.Write([]byte{counter})
		block = expand.Sum(nil)
		out = append(out, block...)
	}
	return out[:length]
}
