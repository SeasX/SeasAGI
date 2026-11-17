// Package secure 是 platform-api 的凭证加密入口，底层复用 shared/secure。
//
// 对外 API（EncryptString / DecryptString）、密文格式（"v2:" 前缀 + HKDF 派生）
// 以及无前缀历史密文的兼容解密均与重构前保持一致。
package secure

import (
	shared "github.com/SeasAGI/SeasAGI-Server/shared/secure"
)

// ciphertextV2Prefix 标记使用 HKDF 派生密钥加密的新密文。
// 旧密文无前缀，继续走历史派生路径解密，保证升级不丢数据。
const ciphertextV2Prefix = "v2:"

var (
	hkdfSalt = []byte("seasagi/data-key/salt/v1")
	hkdfInfo = []byte("seasagi/data-key/aes-gcm/v1")
)

// box 每次调用时读取 SEASAGI_DATA_KEY，支持测试与运行期切换环境变量。
var box = shared.Box{
	KeyEnv:         []string{"SEASAGI_DATA_KEY"},
	Salt:           hkdfSalt,
	Info:           hkdfInfo,
	Prefix:         ciphertextV2Prefix,
	LegacyFallback: true,
}

func EncryptString(plain string) (string, error) {
	return box.Encrypt(plain)
}

func DecryptString(cipherValue string) (string, error) {
	return box.Decrypt(cipherValue)
}

// dataKey 读取 SEASAGI_DATA_KEY 并用 HKDF-SHA256 派生 32 字节 AES 密钥。
func dataKey(raw string) ([]byte, error) {
	return shared.DeriveKey("SEASAGI_DATA_KEY", raw, hkdfSalt, hkdfInfo)
}

// legacyDataKey 保留升级前的派生语义，仅用于解密历史密文。
func legacyDataKey(raw string) ([]byte, error) {
	return shared.LegacyKey("SEASAGI_DATA_KEY", raw)
}
