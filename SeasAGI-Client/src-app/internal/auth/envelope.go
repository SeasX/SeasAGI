package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 平台请求信封（P2-2）：客户端每次进程启动生成 X25519 会话密钥，
// 用平台公钥做 ECDH 派生 AES-256-GCM，密钥只存在于本次进程内。
const (
	envelopeHeader      = "X-SeasAGI-Envelope"
	envelopeClientPub   = "X-SeasAGI-Client-Pub"
	envelopeInfo        = "seasagi-envelope-v1"
	envelopeCacheTTL    = 10 * time.Minute
	envelopeMaxBodySize = 8 << 20
)

var (
	envelopeKeyOnce sync.Once
	envelopeKey     *ecdh.PrivateKey
	envelopeKeyErr  error

	envelopePubMu      sync.Mutex
	envelopePub        []byte
	envelopePubFetched time.Time
)

func clientEnvelopeKey() (*ecdh.PrivateKey, error) {
	envelopeKeyOnce.Do(func() {
		envelopeKey, envelopeKeyErr = ecdh.X25519().GenerateKey(rand.Reader)
	})
	return envelopeKey, envelopeKeyErr
}

// fetchEnvelopePubKey 拉取平台公钥并缓存 10 分钟。
func fetchEnvelopePubKey() ([]byte, error) {
	envelopePubMu.Lock()
	defer envelopePubMu.Unlock()
	if len(envelopePub) > 0 && time.Since(envelopePubFetched) < envelopeCacheTTL {
		return envelopePub, nil
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/crypto/envelope", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("envelope public key unavailable (%d)", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&payload); err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload.Data.PublicKey))
	if err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("invalid envelope public key")
	}
	envelopePub = raw
	envelopePubFetched = time.Now()
	return raw, nil
}

// resetEnvelopeCache 丢弃缓存的平台公钥（平台重启导致密钥变化时重试）。
func resetEnvelopeCache() {
	envelopePubMu.Lock()
	envelopePub = nil
	envelopePubFetched = time.Time{}
	envelopePubMu.Unlock()
}

// encryptPlatformBody 加密请求体。返回（密文 base64, 客户端公钥 base64, 是否成功）。
// 平台未开放信封或本地异常时返回 ok=false，调用方回退明文（仍由 TLS 保护）。
func encryptPlatformBody(body []byte) (string, string, bool) {
	if len(body) == 0 || len(body) > envelopeMaxBodySize {
		return "", "", false
	}
	priv, err := clientEnvelopeKey()
	if err != nil {
		return "", "", false
	}
	pubBytes, err := fetchEnvelopePubKey()
	if err != nil {
		return "", "", false
	}
	pub, err := ecdh.X25519().NewPublicKey(pubBytes)
	if err != nil {
		return "", "", false
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return "", "", false
	}
	key, err := hkdf.Key(sha256.New, shared, nil, envelopeInfo, 32)
	if err != nil {
		return "", "", false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", false
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", false
	}
	sealed := gcm.Seal(nil, nonce, body, nil)
	blob := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(blob), base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()), true
}
