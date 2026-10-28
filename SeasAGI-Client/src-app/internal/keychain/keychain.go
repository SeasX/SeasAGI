package keychain

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

const (
	serviceName      = "com.seasagi.desktop"
	accessTokenKey   = "local_access_token"
	platformTokenKey = "platform_access_token"
	tokenLength      = 32
)

func GetOrCreateAccessToken() (string, error) {
	token, err := getKey(accessTokenKey)
	if err == nil && token != "" {
		return token, nil
	}
	token = generateToken()
	if err := saveKey(accessTokenKey, token); err != nil {
		return "", err
	}
	return token, nil
}

// GetAccessToken 读取本地网关令牌；不存在时返回空字符串（不自动生成）。
func GetAccessToken() (string, error) {
	return getKey(accessTokenKey)
}

func GetMaskedAccessToken() (string, error) {
	token, err := getKey(accessTokenKey)
	if err != nil {
		return "", err
	}
	if len(token) > 8 {
		return token[:4] + "••••" + token[len(token)-4:], nil
	}
	return token, nil
}

func ResetAccessToken() (string, error) {
	token := generateToken()
	if err := saveKey(accessTokenKey, token); err != nil {
		return "", err
	}
	return token, nil
}

func SavePlatformToken(token string) error {
	return saveKey(platformTokenKey, token)
}

func GetPlatformToken() (string, error) {
	return getKey(platformTokenKey)
}

func ClearPlatformToken() error {
	_ = deleteKey(platformTokenKey)
	return nil
}

func SaveChannelKey(channelID, apiKey string) error {
	return saveKey("channel_"+channelID, apiKey)
}

func GetChannelKey(channelID string) (string, error) {
	return getKey("channel_" + channelID)
}

func DeleteChannelKey(channelID string) error {
	return deleteKey("channel_" + channelID)
}

// SaveChannelKeys 将多 key 以 JSON 数组形式存入 keychain，避免明文落盘到 config.json。
func SaveChannelKeys(channelID string, apiKeys []string) error {
	if len(apiKeys) == 0 {
		return nil
	}
	data, err := json.Marshal(apiKeys)
	if err != nil {
		return err
	}
	return saveKey("channel_keys_"+channelID, string(data))
}

// GetChannelKeys 从 keychain 读取多 key。未配置时返回空切片。
func GetChannelKeys(channelID string) ([]string, error) {
	raw, err := getKey("channel_keys_" + channelID)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteChannelKeys 删除多 key 的 keychain 条目。
func DeleteChannelKeys(channelID string) error {
	return deleteKey("channel_keys_" + channelID)
}

// SaveSecret 保存任意命名密钥（如 A 端节点共享密钥、OAuth 刷新令牌）。
func SaveSecret(name, value string) error {
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	return saveKey("secret_"+name, value)
}

// GetSecret 读取任意命名密钥；不存在时返回空字符串。
// 读取失败（如存储损坏、权限不足、解密失败）会返回错误，不再静默吞掉。
func GetSecret(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("secret name is required")
	}
	return getKey("secret_" + name)
}

// DeleteSecret 删除命名密钥。
func DeleteSecret(name string) error {
	return deleteKey("secret_" + name)
}

func generateToken() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, tokenLength)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

// envKeyName 仅用于兼容旧环境变量命名（诊断/迁移提示），不再作为凭证存储。
func envKeyName(keyID string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	return "SEASAGI_" + strings.ToUpper(replacer.Replace(keyID))
}

// 平台实现（见 store_darwin.go / store_windows.go / store_file.go）：
//   - macOS：系统 Keychain（Security.framework，密钥不经过子进程 argv）
//   - Windows：DPAPI 加密后落盘到 0600 文件
//   - 其它（Linux 等）：AES-256-GCM 加密后落盘到 0600 文件，主密钥同样 0600
//
// 任何平台都不得把凭证写入进程环境变量：环境变量对子进程可见且不持久化。
func getKey(keyID string) (string, error) { return platformGet(keyID) }
func saveKey(keyID, secret string) error  { return platformSave(keyID, secret) }
func deleteKey(keyID string) error        { return platformDelete(keyID) }
