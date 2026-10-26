package logging

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
)

// PayloadLoggingEnabled 返回是否允许把请求体写入本地日志。
// 默认关闭；仅当显式设置 SEASAGI_LOG_PAYLOADS=1 时开启，避免 prompt/PII 长期落盘。
func PayloadLoggingEnabled() bool {
	return strings.TrimSpace(os.Getenv("SEASAGI_LOG_PAYLOADS")) == "1"
}

// sensitiveKeys 需要脱敏的敏感 key（统一去掉 `_`/`-` 后小写比较）。
var sensitiveKeys = map[string]bool{
	"apikey":        true,
	"apisecret":     true,
	"authorization": true,
	"auth":          true,
	"token":         true,
	"accesstoken":   true,
	"refreshtoken":  true,
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"clientsecret":  true,
	"privatekey":    true,
	"xapikey":       true,
	"apikeyid":      true,
	"credential":    true,
}

// tokenCounterKeys 是「计数字段」，虽以 tokens 结尾但不是凭证，避免误脱敏影响可观测性。
var tokenCounterKeys = map[string]bool{
	"maxtokens":           true,
	"maxcompletiontokens": true,
	"maxtokencount":       true,
	"prompttokens":        true,
	"completiontokens":    true,
	"totaltokens":         true,
	"inputtokens":         true,
	"outputtokens":        true,
	"cachedtokens":        true,
	"reasoningtokens":     true,
	"tokencount":          true,
	"tokens":              true,
	"tokensused":          true,
}

// normalizeKey 归一化字段名：小写并去掉分隔符。
func normalizeKey(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	k = strings.ReplaceAll(k, "_", "")
	k = strings.ReplaceAll(k, "-", "")
	return k
}

// maxPayloadLength payload 序列化最大字符数。
const maxPayloadLength = 65536

// IsSensitiveKey 判断 key 是否敏感（归一化后全等或按片段匹配）。
func IsSensitiveKey(key string) bool {
	k := normalizeKey(key)
	if k == "" {
		return false
	}
	if sensitiveKeys[k] {
		return true
	}
	for _, frag := range []string{"apikey", "secret", "password", "passwd", "credential", "privatekey", "authorization"} {
		if strings.Contains(k, frag) {
			return true
		}
	}
	if strings.HasSuffix(k, "token") && !tokenCounterKeys[k] {
		return true
	}
	return false
}

// RedactPayload 递归脱敏 map/slice 中的敏感字段。
func RedactPayload(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, value := range v {
			if IsSensitiveKey(key) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = RedactPayload(value)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = RedactPayload(item)
		}
		return result
	default:
		return data
	}
}

var (
	bearerPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-+/=]{8,}`)
	tokenPattern  = regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_\-]{12,}|AIza[0-9A-Za-z_\-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|xox[baprs]-[A-Za-z0-9-]{10,})`)
)

// RedactBearerToken 脱敏字符串中的全部 Bearer token 与常见 Key 形态（CL-S13）。
func RedactBearerToken(s string) string {
	if s == "" {
		return s
	}
	s = bearerPattern.ReplaceAllString(s, "Bearer [REDACTED]")
	return tokenPattern.ReplaceAllString(s, "[REDACTED]")
}

// SerializePayloadForStorage 序列化 payload 并截断到最大长度。
func SerializePayloadForStorage(data interface{}) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	s := string(bytes)
	if len(s) > maxPayloadLength {
		return s[:maxPayloadLength] + "...[truncated]"
	}
	return s
}

// IsOpaqueBinary 检测数据是否为二进制（[]byte / ArrayBuffer）。
func IsOpaqueBinary(data interface{}) bool {
	switch data.(type) {
	case []byte:
		return true
	default:
		return false
	}
}

// ProtectPayloadForLog 组合脱敏：normalize → PII sanitize → redact。
func ProtectPayloadForLog(data interface{}) string {
	if IsOpaqueBinary(data) {
		return "[binary data]"
	}
	redacted := RedactPayload(data)
	serialized := SerializePayloadForStorage(redacted)
	serialized = RedactBearerToken(serialized)
	return serialized
}
