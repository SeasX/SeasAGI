package keychain

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"runtime"
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

func generateToken() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, tokenLength)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

func getKey(keyID string) (string, error) {
	if runtime.GOOS != "darwin" {
		return os.Getenv(envKeyName(keyID)), nil
	}

	cmd := exec.Command("security", "find-generic-password", "-a", keyID, "-s", serviceName, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read keychain entry %s: %w", keyID, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func saveKey(keyID, secret string) error {
	if runtime.GOOS != "darwin" {
		return os.Setenv(envKeyName(keyID), secret)
	}

	_ = deleteKey(keyID)
	cmd := exec.Command("security", "add-generic-password", "-U", "-a", keyID, "-s", serviceName, "-w", secret)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("save keychain entry %s: %s (%w)", keyID, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func deleteKey(keyID string) error {
	if runtime.GOOS != "darwin" {
		return os.Unsetenv(envKeyName(keyID))
	}

	cmd := exec.Command("security", "delete-generic-password", "-a", keyID, "-s", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(output))
		if strings.Contains(msg, "could not be found") {
			return nil
		}
		return fmt.Errorf("delete keychain entry %s: %s (%w)", keyID, msg, err)
	}
	return nil
}

func envKeyName(keyID string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_")
	return "SEASAGI_" + strings.ToUpper(replacer.Replace(keyID))
}
