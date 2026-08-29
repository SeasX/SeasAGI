package logging

import (
	"encoding/json"
	"strings"
)

// sensitiveKeys 需要脱敏的敏感 key 列表（小写匹配）。
var sensitiveKeys = map[string]bool{
	"api_key":       true,
	"apikey":        true,
	"api_secret":    true,
	"apisecret":     true,
	"authorization": true,
	"auth":          true,
	"token":         true,
	"access_token":  true,
	"accesstoken":   true,
	"refresh_token": true,
	"refreshtoken":  true,
	"password":      true,
	"passwd":        true,
	"secret":        true,
	"client_secret": true,
	"clientsecret":  true,
	"private_key":   true,
	"privatekey":    true,
}

// maxPayloadLength payload 序列化最大字符数。
const maxPayloadLength = 65536

// IsSensitiveKey 判断 key 是否敏感。
func IsSensitiveKey(key string) bool {
	return sensitiveKeys[strings.ToLower(key)]
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

// RedactBearerToken 脱敏字符串中的 Bearer token。
func RedactBearerToken(s string) string {
	idx := strings.Index(s, "Bearer ")
	if idx == -1 {
		return s
	}
	// 替换 "Bearer " 后面的 token 内容（到空格或字符串结尾）
	start := idx + 7 // len("Bearer ")
	end := start
	for end < len(s) && s[end] != ' ' && s[end] != '"' && s[end] != '\'' && s[end] != ',' {
		end++
	}
	return s[:start] + "[REDACTED]" + s[end:]
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
