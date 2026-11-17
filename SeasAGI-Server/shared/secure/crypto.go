// Package secure 提供 platform-api 与 relay-gateway 共用的凭证加密原语。
//
// 两个服务各自持有独立的 Box 配置（密钥来源环境变量、HKDF salt/info、密文前缀），
// 即使运维把同一把主密钥配置给两个进程，派生出的 AES 密钥也互不相同（域隔离）。
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// MinSecretLength 是主密钥的最小字节数；更短的口令会被拒绝而不是被静默补齐。
const MinSecretLength = 32

// ErrKeyNotConfigured 表示 Box 的所有候选环境变量都未配置主密钥。
var ErrKeyNotConfigured = errors.New("data encryption key is not configured")

// Box 描述一个服务的加密配置。零值不可用，必须至少提供 KeyEnv/Salt/Info/Prefix。
type Box struct {
	// KeyEnv 是主密钥的候选环境变量名，按顺序取第一个非空值。
	KeyEnv []string
	// Salt / Info 绑定 HKDF 派生用途，不同服务必须使用不同取值。
	Salt []byte
	Info []byte
	// Prefix 标记本 Box 产出的密文，用于区分历史明文/历史密文。
	Prefix string
	// LegacyFallback 为 true 时，无前缀的值按旧的截断派生密钥解密（仅 platform-api 需要）。
	LegacyFallback bool
}

// keyMaterial 返回第一个已配置的环境变量名及其值。
func (b Box) keyMaterial() (string, string) {
	for _, name := range b.KeyEnv {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return name, v
		}
	}
	if len(b.KeyEnv) > 0 {
		return b.KeyEnv[0], ""
	}
	return "", ""
}

// KeySource 返回当前生效的密钥环境变量名；未配置时返回空串。
func (b Box) KeySource() string {
	name, raw := b.keyMaterial()
	if raw == "" {
		return ""
	}
	return name
}

// Check 校验主密钥已配置且足够强；不返回、不记录密钥本身。
func (b Box) Check() error {
	name, raw := b.keyMaterial()
	if raw == "" {
		return fmt.Errorf("%w: set %s", ErrKeyNotConfigured, strings.Join(b.KeyEnv, " or "))
	}
	return ValidateSecret(name, raw)
}

// IsCiphertext 判断值是否为本 Box 产出的密文。
func (b Box) IsCiphertext(v string) bool {
	return b.Prefix != "" && strings.HasPrefix(strings.TrimSpace(v), b.Prefix)
}

// Encrypt 使用 AES-256-GCM 加密并附加 Prefix。
func (b Box) Encrypt(plain string) (string, error) {
	name, raw := b.keyMaterial()
	key, err := DeriveKey(name, raw, b.Salt, b.Info)
	if err != nil {
		return "", err
	}
	sealed, err := seal(key, []byte(plain))
	if err != nil {
		return "", err
	}
	return b.Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt 解密本 Box 的密文；LegacyFallback 打开时兼容无前缀的历史密文。
func (b Box) Decrypt(value string) (string, error) {
	raw := strings.TrimSpace(value)
	name, secret := b.keyMaterial()

	var key []byte
	var err error
	switch {
	case b.Prefix != "" && strings.HasPrefix(raw, b.Prefix):
		raw = strings.TrimPrefix(raw, b.Prefix)
		key, err = DeriveKey(name, secret, b.Salt, b.Info)
	case b.LegacyFallback:
		key, err = LegacyKey(name, secret)
	default:
		return "", fmt.Errorf("value is not a recognised ciphertext")
	}
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", err
	}
	return open(key, data)
}

// DeriveKey 校验主密钥后用 HKDF-SHA256 派生 32 字节 AES 密钥。
func DeriveKey(envName, raw string, salt, info []byte) ([]byte, error) {
	if err := ValidateSecret(envName, raw); err != nil {
		return nil, err
	}
	return HKDFSHA256([]byte(strings.TrimSpace(raw)), salt, info, 32), nil
}

// LegacyKey 保留历史的「截断/补零」派生语义，仅用于解密升级前的密文。
func LegacyKey(envName, raw string) ([]byte, error) {
	if err := ValidateSecret(envName, raw); err != nil {
		return nil, err
	}
	buf := make([]byte, 32)
	copy(buf, strings.TrimSpace(raw))
	return buf, nil
}

// ValidateSecret 校验密钥存在且不短于 MinSecretLength；空密钥等同于公开密钥，必须拒绝。
func ValidateSecret(envName, raw string) error {
	key := strings.TrimSpace(raw)
	if envName == "" {
		envName = "data key"
	}
	if key == "" {
		return fmt.Errorf("%s is required; refusing to use a built-in default key", envName)
	}
	if len(key) < MinSecretLength {
		return fmt.Errorf("%s must be at least %d bytes (got %d)", envName, MinSecretLength, len(key))
	}
	return nil
}

// HKDFSHA256 实现 RFC 5869 的 HKDF（Extract + Expand）。
func HKDFSHA256(ikm, salt, info []byte, length int) []byte {
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

func seal(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	return gcm.Seal(iv, iv, plain, nil), nil
}

func open(key, data []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
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
