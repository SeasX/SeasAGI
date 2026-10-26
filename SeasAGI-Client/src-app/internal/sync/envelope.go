package sync

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
)

const (
	// syncEnvelopeVersion 当前签名信封版本。
	syncEnvelopeVersion = "1"
	// syncEnvelopeAlgorithm 固定为 HMAC-SHA256，客户端拒绝其它算法。
	syncEnvelopeAlgorithm = "hmac-sha256"
	// syncHMACEnv 允许用户显式提供同步密钥（多设备共享场景），优先级最高。
	syncHMACEnv = "SEASAGI_SYNC_HMAC_KEY"
	// syncHMACSecretName 默认把同步密钥保存在系统安全存储中。
	syncHMACSecretName = "sync_hmac_key"
	// syncHMACKeyBytes 自动生成密钥的长度。
	syncHMACKeyBytes = 32
)

// signedEnvelope 是远端配置的统一封装：payload 为原始 JSON 的 base64，
// signature 为对原始 payload 字节的 HMAC-SHA256（hex）。
type signedEnvelope struct {
	Version   string `json:"version"`
	Algorithm string `json:"algorithm"`
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

// sealSyncPayload 将原始配置封装为签名信封。
func sealSyncPayload(payload []byte, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("refusing to sync without a signing key")
	}
	env := signedEnvelope{
		Version:   syncEnvelopeVersion,
		Algorithm: syncEnvelopeAlgorithm,
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: NewHMACSigner(key).Sign(payload),
	}
	return json.Marshal(env)
}

// openSyncPayload 校验签名并返回原始配置。任何解析/签名错误都必须拒绝。
func openSyncPayload(raw []byte, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("refusing to apply remote config without a signing key")
	}
	var env signedEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("remote config is not a signed envelope: %w", err)
	}
	if env.Version != syncEnvelopeVersion {
		return nil, fmt.Errorf("unsupported sync envelope version %q", env.Version)
	}
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
