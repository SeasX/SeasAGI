package sync

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// HMACSigner HMAC-SHA256 签名器。
type HMACSigner struct {
	secret []byte
}

// NewHMACSigner 创建签名器。
func NewHMACSigner(secret string) *HMACSigner {
	return &HMACSigner{secret: []byte(secret)}
}

// Sign 对数据签名，返回 hex 编码的 HMAC。
func (s *HMACSigner) Sign(data []byte) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Verify 验证签名（常量时间比较，防时序攻击）。
func (s *HMACSigner) Verify(data []byte, signature string) bool {
	h := hmac.New(sha256.New, s.secret)
	h.Write(data)
	expected := h.Sum(nil)

	signatureBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	if len(expected) != len(signatureBytes) {
		return false
	}

	return subtle.ConstantTimeCompare(expected, signatureBytes) == 1
}

// SignString 对字符串签名。
func (s *HMACSigner) SignString(data string) string {
	return s.Sign([]byte(data))
}

// VerifyString 验证字符串签名。
func (s *HMACSigner) VerifyString(data, signature string) bool {
	return s.Verify([]byte(data), signature)
}

// ComputeVersionHash 计算配置数据的确定性版本哈希。
func ComputeVersionHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// VerifySignatureWithKey 使用指定密钥验证签名（便捷函数）。
func VerifySignatureWithKey(data []byte, signature, secret string) bool {
	signer := NewHMACSigner(secret)
	return signer.Verify(data, signature)
}

// SignWithKey 使用指定密钥签名（便捷函数）。
func SignWithKey(data []byte, secret string) string {
	signer := NewHMACSigner(secret)
	return signer.Sign(data)
}

// ValidateTimestamp 检查时间戳是否在有效期内（防重放攻击）。
func ValidateTimestamp(timestamp int64, maxAgeSeconds int64, currentTime int64) bool {
	diff := currentTime - timestamp
	if diff < 0 {
		diff = -diff
	}
	return diff <= maxAgeSeconds
}

// FormatSignatureError 格式化签名验证错误。
func FormatSignatureError(reason string) string {
	return fmt.Sprintf("signature verification failed: %s", reason)
}
