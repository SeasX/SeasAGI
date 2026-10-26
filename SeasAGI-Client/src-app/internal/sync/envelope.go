package sync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
)

const (
	// syncEnvelopeVersion 当前签名信封版本（v2 引入 AEAD 加密，避免凭证明文推送）。
	syncEnvelopeVersion = "2"
	// syncEnvelopeLegacyVersion v1 信封仅有 HMAC 签名，拉取时保持向后兼容。
	syncEnvelopeLegacyVersion = "1"
	// syncEnvelopeAlgorithm 固定为 HMAC-SHA256，客户端拒绝其它算法。
	syncEnvelopeAlgorithm = "hmac-sha256"
	// syncEnvelopeCipher v2 信封固定使用 AES-256-GCM。
	syncEnvelopeCipher = "aes-256-gcm"
	// syncEnvelopeMaxAge v2 信封的最大有效时长，超出即拒绝（防重放/陈旧凭证）。
	syncEnvelopeMaxAge = 30 * 24 * time.Hour
	// syncEnvelopeFutureSkew 容忍的时钟偏差上限（信封签发时间最多领先当前时间这么久）。
	syncEnvelopeFutureSkew = 5 * time.Minute
	// syncEncryptionKeyInfo v2 加密密钥的派生上下文，避免直接复用签名密钥做 AES 密钥。
	syncEncryptionKeyInfo = "seasagi-sync-encrypt-v2"
	// syncHMACEnv 允许用户显式提供同步密钥（多设备共享场景），优先级最高。
	syncHMACEnv = "SEASAGI_SYNC_HMAC_KEY"
	// syncHMACSecretName 默认把同步密钥保存在系统安全存储中。
	syncHMACSecretName = "sync_hmac_key"
	// syncHMACKeyBytes 自动生成密钥的长度。
	syncHMACKeyBytes = 32
)

// signedEnvelope 是远端配置的统一封装：
//   - v1：payload 为原始 JSON 的 base64，signature 为对原始 payload 字节的 HMAC-SHA256（hex）。
//   - v2：payload 为 AES-256-GCM 密文的 base64，nonce 为随机 nonce 的 base64，
//     signature 为对 canonical 元数据串的 HMAC-SHA256（hex）。
type signedEnvelope struct {
	Version   string `json:"version"`
	Algorithm string `json:"algorithm"`
	Cipher    string `json:"cipher,omitempty"`
	Nonce     string `json:"nonce,omitempty"`
	IssuedAt  int64  `json:"issued_at"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// syncKeyFromEnv 读取显式配置的同步密钥。
func syncKeyFromEnv() string {
	return strings.TrimSpace(os.Getenv(syncHMACEnv))
}

// syncVerificationKey 获取「仅用于校验」的密钥。缺失时返回错误，
// 调用方必须拒绝应用远端配置（fail closed）。
func syncVerificationKey() (string, error) {
	if key := syncKeyFromEnv(); key != "" {
		return key, nil
	}
	key, err := keychain.GetSecret(syncHMACSecretName)
	if err != nil {
		return "", fmt.Errorf("cloud sync signing key unavailable: %w", err)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("cloud sync signing key is not configured; refusing to apply remote config")
	}
	return key, nil
}

// syncSigningKey 获取推送用密钥；首次使用时自动生成并写入系统安全存储，
// 保证「开箱即用」且推送内容始终可被校验。
func syncSigningKey() (string, error) {
	if key := syncKeyFromEnv(); key != "" {
		return key, nil
	}
	if key, err := keychain.GetSecret(syncHMACSecretName); err == nil && strings.TrimSpace(key) != "" {
		return strings.TrimSpace(key), nil
	}
	buf := make([]byte, syncHMACKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate cloud sync signing key: %w", err)
	}
	key := hex.EncodeToString(buf)
	if err := keychain.SaveSecret(syncHMACSecretName, key); err != nil {
		return "", fmt.Errorf("persist cloud sync signing key: %w", err)
	}
	return key, nil
}

// deriveSyncEncryptionKey 从同步密钥派生 32 字节 AES-256 密钥。
// 使用 HKDF 式的 HMAC-SHA256(key, info) 派生，与签名用途做域分离。
func deriveSyncEncryptionKey(key string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(syncEncryptionKeyInfo))
	return mac.Sum(nil)
}

// syncEnvelopeCanonical 构造 v2 信封被签名的规范化元数据串。
// 覆盖除 signature 外的所有元数据字段，任一字段被篡改都会导致验签失败。
func syncEnvelopeCanonical(version, algorithm, cipherName, nonce string, issuedAt int64, payload string) []byte {
	return []byte(version + "\n" + algorithm + "\n" + cipherName + "\n" + nonce + "\n" +
		strconv.FormatInt(issuedAt, 10) + "\n" + payload)
}

// sealSyncPayload 将原始配置封装为 v2 加密签名信封。
// 原始 payload 先经 AES-256-GCM 加密，密文再整体纳入签名的 canonical 串，
// 因此远端不会出现可逆的凭证明文。
func sealSyncPayload(payload []byte, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("refusing to sync without a signing key")
	}
	block, err := aes.NewCipher(deriveSyncEncryptionKey(key))
	if err != nil {
		return nil, fmt.Errorf("init sync cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init sync AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate sync nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, payload, nil)

	payloadB64 := base64.StdEncoding.EncodeToString(ciphertext)
	nonceB64 := base64.StdEncoding.EncodeToString(nonce)
	issuedAt := time.Now().Unix()

	canonical := syncEnvelopeCanonical(syncEnvelopeVersion, syncEnvelopeAlgorithm, syncEnvelopeCipher, nonceB64, issuedAt, payloadB64)
	env := signedEnvelope{
		Version:   syncEnvelopeVersion,
		Algorithm: syncEnvelopeAlgorithm,
		Cipher:    syncEnvelopeCipher,
		Nonce:     nonceB64,
		IssuedAt:  issuedAt,
		Payload:   payloadB64,
		Signature: NewHMACSigner(key).Sign(canonical),
	}
	return json.Marshal(env)
}

// openSyncPayload 校验签名并在 v2 时解密，返回原始配置。任何解析/签名/解密错误都必须拒绝。
func openSyncPayload(raw []byte, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("refusing to apply remote config without a signing key")
	}
	var env signedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("remote config is not a signed envelope: %w", err)
	}
	switch env.Version {
	case syncEnvelopeLegacyVersion:
		return openLegacySyncPayload(env, key)
	case syncEnvelopeVersion:
		return openEncryptedSyncPayload(env, key)
	default:
		return nil, fmt.Errorf("unsupported sync envelope version %q", env.Version)
	}
}

// openLegacySyncPayload 按 v1 语义验签：payload 即明文的 base64，签名覆盖明文，不校验 issued_at。
func openLegacySyncPayload(env signedEnvelope, key string) ([]byte, error) {
	if !strings.EqualFold(env.Algorithm, syncEnvelopeAlgorithm) {
		return nil, fmt.Errorf("unsupported sync signature algorithm %q", env.Algorithm)
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode sync payload: %w", err)
	}
	if !NewHMACSigner(key).Verify(payload, env.Signature) {
		return nil, fmt.Errorf("sync signature verification failed")
	}
	return payload, nil
}

// openEncryptedSyncPayload 校验 v2 信封元数据、签名与时效，再解密还原明文。
func openEncryptedSyncPayload(env signedEnvelope, key string) ([]byte, error) {
	if !strings.EqualFold(env.Algorithm, syncEnvelopeAlgorithm) {
		return nil, fmt.Errorf("unsupported sync signature algorithm %q", env.Algorithm)
	}
	if env.Cipher != syncEnvelopeCipher {
		return nil, fmt.Errorf("unsupported sync cipher %q", env.Cipher)
	}
	if env.IssuedAt <= 0 {
		return nil, fmt.Errorf("sync envelope missing issuance time")
	}
	issued := time.Unix(env.IssuedAt, 0)
	now := time.Now()
	if issued.After(now.Add(syncEnvelopeFutureSkew)) {
		return nil, fmt.Errorf("sync envelope issued in the future")
	}
	if now.Sub(issued) > syncEnvelopeMaxAge {
		return nil, fmt.Errorf("sync envelope expired")
	}

	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode sync nonce: %w", err)
	}

	canonical := syncEnvelopeCanonical(env.Version, env.Algorithm, env.Cipher, env.Nonce, env.IssuedAt, env.Payload)
	if !NewHMACSigner(key).Verify(canonical, env.Signature) {
		return nil, fmt.Errorf("sync signature verification failed")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("decode sync payload: %w", err)
	}
	block, err := aes.NewCipher(deriveSyncEncryptionKey(key))
	if err != nil {
		return nil, fmt.Errorf("init sync cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init sync AEAD: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid sync nonce length %d", len(nonce))
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt sync payload: %w", err)
	}
	return plaintext, nil
}
