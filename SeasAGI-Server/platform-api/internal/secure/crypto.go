package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func EncryptString(plain string) (string, error) {
	key := normalizeKey(os.Getenv("SEASAGI_DATA_KEY"))
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
	key := normalizeKey(os.Getenv("SEASAGI_DATA_KEY"))
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

func normalizeKey(key string) []byte {
	if key == "" {
		key = "seasagi-default-data-key-32bytes!"
	}
	buf := make([]byte, 32)
	copy(buf, []byte(key))
	return buf[:32]
}
