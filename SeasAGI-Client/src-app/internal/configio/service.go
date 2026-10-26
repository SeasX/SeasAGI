package configio

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

// redactedValue 是导出时替换敏感值的占位符。
const redactedValue = "[REDACTED]"

type ExportData struct {
	Version         string            `json:"version"`
	ExportedAt      string            `json:"exported_at"`
	SecretsRedacted bool              `json:"secrets_redacted"`
	AppConfig       config.AppConfig  `json:"app_config"`
	Channels        []config.Channel  `json:"channels"`
	MCPServers      []MCPServerExport `json:"mcp_servers,omitempty"`
	PromptPresets   []PromptExport    `json:"prompt_presets,omitempty"`
}

type MCPServerExport struct {
	Name        string            `json:"name"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	Transport   string            `json:"transport"`
	URL         string            `json:"url,omitempty"`
	Enabled     bool              `json:"enabled"`
	Apps        []string          `json:"apps"`
	Description string            `json:"description,omitempty"`
}

type PromptExport struct {
	Name        string `json:"name"`
	Content     string `json:"content"`
	TargetApp   string `json:"target_app"`
	IsDefault   bool   `json:"is_default"`
	Description string `json:"description,omitempty"`
}

type Service struct {
	configSvc *config.Service
	homeDir   string
}

func NewService(configSvc *config.Service) *Service {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return &Service{
		configSvc: configSvc,
		homeDir:   home,
	}
}

func (s *Service) Export() ([]byte, error) {
	cfg := s.configSvc.GetConfig()
	channels, err := s.configSvc.ListChannels()
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}

	data := ExportData{
		Version:         "1.0",
		ExportedAt:      time.Now().Format(time.RFC3339),
		SecretsRedacted: true,
		AppConfig:       redactAppConfig(cfg),
		Channels:        redactChannels(channels),
	}

	mcpData, _ := os.ReadFile(filepath.Join(s.homeDir, ".seasagi", "mcp_servers.json"))
	if len(mcpData) > 0 {
		json.Unmarshal(mcpData, &data.MCPServers)
		data.MCPServers = redactMCPServers(data.MCPServers)
	}

	promptData, _ := os.ReadFile(filepath.Join(s.homeDir, ".seasagi", "prompt_presets.json"))
	if len(promptData) > 0 {
		json.Unmarshal(promptData, &data.PromptPresets)
	}

	return json.MarshalIndent(data, "", "  ")
}

// redactChannels 按白名单 DTO 重建导出渠道，保证即使上游列表契约回归，
// 导出内容也不含 API Key / OAuth 凭证（CL-S1）。
func redactChannels(channels []config.Channel) []config.Channel {
	result := make([]config.Channel, 0, len(channels))
	for _, ch := range channels {
		result = append(result, ch.Sanitize())
	}
	return result
}

// redactAppConfig 复制配置并清除其中的令牌类 URL 参数，避免导出文件泄露凭证。
func redactAppConfig(cfg config.AppConfig) config.AppConfig {
	cfg.SelectedGrantRelayURL = redactURLSecrets(cfg.SelectedGrantRelayURL)
	if cfg.MarketNode != nil {
		node := *cfg.MarketNode
		node.NodeURL = redactURLSecrets(node.NodeURL)
		cfg.MarketNode = &node
	}
	return cfg
}

// normalizeSecretKey 把键名归一化（小写、去掉 _ 与 -），便于按语义匹配敏感字段。
func normalizeSecretKey(key string) string {
	key = strings.ToLower(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return key
}

// isSensitiveKey 判断键名是否包含 token/key/secret/password/credential/auth。
func isSensitiveKey(key string) bool {
	normalized := normalizeSecretKey(key)
	for _, marker := range []string{"token", "key", "secret", "password", "credential", "auth"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

// redactMCPServers 返回脱敏后的 MCP 服务器配置副本。
func redactMCPServers(servers []MCPServerExport) []MCPServerExport {
	result := make([]MCPServerExport, 0, len(servers))
	for _, server := range servers {
		result = append(result, redactMCPServer(server))
	}
	return result
}

func redactMCPServer(server MCPServerExport) MCPServerExport {
	if len(server.Env) > 0 {
		env := make(map[string]string, len(server.Env))
		for k, v := range server.Env {
			if isSensitiveKey(k) {
				v = redactedValue
			}
			env[k] = v
		}
		server.Env = env
	}
	if len(server.Args) > 0 {
		args := append([]string(nil), server.Args...)
		for i := 1; i < len(args); i++ {
			prev := args[i-1]
			if strings.HasPrefix(prev, "--") && isSensitiveKey(prev) {
				args[i] = redactedValue
			}
		}
		server.Args = args
	}
	return server
}

// redactURLSecrets 脱敏 URL 中的 userinfo 与 token/key/secret/signature 查询参数，
// 保留 scheme/host/path 等定位信息。
func redactURLSecrets(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	changed := false
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		values := u.Query()
		for key := range values {
			normalized := normalizeSecretKey(key)
			for _, marker := range []string{"token", "key", "secret", "signature"} {
				if strings.Contains(normalized, marker) {
					values[key] = []string{redactedValue}
					changed = true
					break
				}
			}
		}
		if changed {
			u.RawQuery = values.Encode()
		}
	}
	if !changed {
		return raw
	}
	return u.String()
}

// writePrivateFileAtomic 在目标目录写 0600 临时文件后 rename，目录权限固定 0700。
func writePrivateFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".import-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (s *Service) Import(jsonData []byte) error {
	var data ExportData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("parse import data: %w", err)
	}

	if data.Version == "" {
		return fmt.Errorf("invalid import data: missing version")
	}

	for _, ch := range data.Channels {
		if ch.ChannelID == "" {
			continue
		}
		if _, err := s.configSvc.SaveCustomChannel(ch); err != nil {
			return fmt.Errorf("save channel %s: %w", ch.ChannelID, err)
		}
	}

	s.configSvc.UpdateRoutingSettings(data.AppConfig.RoutingStrategy, data.AppConfig.StickyChannelUse)
	if data.AppConfig.DefaultModel != "" {
		s.configSvc.SetDefaultModel(data.AppConfig.DefaultModel, data.AppConfig.DefaultChannelID)
	}

	if len(data.MCPServers) > 0 {
		mcpBytes, err := json.MarshalIndent(data.MCPServers, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal mcp servers: %w", err)
		}
		mcpPath := filepath.Join(s.homeDir, ".seasagi", "mcp_servers.json")
		if err := writePrivateFileAtomic(mcpPath, mcpBytes); err != nil {
			return fmt.Errorf("write mcp servers: %w", err)
		}
	}

	if len(data.PromptPresets) > 0 {
		promptBytes, err := json.MarshalIndent(data.PromptPresets, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal prompt presets: %w", err)
		}
		promptPath := filepath.Join(s.homeDir, ".seasagi", "prompt_presets.json")
		if err := writePrivateFileAtomic(promptPath, promptBytes); err != nil {
			return fmt.Errorf("write prompt presets: %w", err)
		}
	}

	return nil
}
