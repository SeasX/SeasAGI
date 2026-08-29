package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// CLITool CLI 工具配置定义
type CLITool struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	ConfigPath   string `json:"config_path"`
	ConfigFormat string `json:"config_format"` // json / yaml / toml
}

// SupportedTools 返回支持的 CLI 工具列表
func SupportedTools() []CLITool {
	home, _ := os.UserHomeDir()
	return []CLITool{
		{
			Name:         "claude-code",
			DisplayName:  "Claude Code",
			ConfigPath:   filepath.Join(home, ".claude", "settings.json"),
			ConfigFormat: "json",
		},
		{
			Name:         "cursor",
			DisplayName:  "Cursor",
			ConfigPath:   filepath.Join(home, ".cursor", "settings.json"),
			ConfigFormat: "json",
		},
		{
			Name:         "windsurf",
			DisplayName:  "Windsurf",
			ConfigPath:   filepath.Join(home, ".windsurf", "settings.json"),
			ConfigFormat: "json",
		},
		{
			Name:         "cline",
			DisplayName:  "Cline",
			ConfigPath:   filepath.Join(home, ".cline", "settings.json"),
			ConfigFormat: "json",
		},
		{
			Name:         "continue",
			DisplayName:  "Continue",
			ConfigPath:   filepath.Join(home, ".continue", "config.json"),
			ConfigFormat: "json",
		},
	}
}

// SetupConfig 配置 CLI 工具指向 SeasAGI
func SetupConfig(toolName, gatewayURL, apiKey string) error {
	tools := SupportedTools()
	var tool *CLITool
	for i := range tools {
		if tools[i].Name == toolName {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		return fmt.Errorf("unknown CLI tool: %s", toolName)
	}

	// 确保目录存在
	dir := filepath.Dir(tool.ConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 构建配置
	config := map[string]any{
		"api_base_url": gatewayURL + "/v1",
		"api_key":      apiKey,
	}

	// 读取现有配置并合并
	if existing, err := os.ReadFile(tool.ConfigPath); err == nil {
		var existingMap map[string]any
		if err := json.Unmarshal(existing, &existingMap); err == nil {
			for k, v := range config {
				existingMap[k] = v
			}
			config = existingMap
		}
	}

	// 写入配置
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(tool.ConfigPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// GetToolConfigPath 返回工具配置文件路径
func GetToolConfigPath(toolName string) (string, error) {
	tools := SupportedTools()
	for _, t := range tools {
		if t.Name == toolName {
			return t.ConfigPath, nil
		}
	}
	return "", fmt.Errorf("unknown tool: %s", toolName)
}

// IsSupported 检查工具是否支持
func IsSupported(toolName string) bool {
	tools := SupportedTools()
	for _, t := range tools {
		if t.Name == toolName {
			return true
		}
	}
	return false
}

// PlatformSupported 当前平台是否支持
func PlatformSupported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "linux" || runtime.GOOS == "windows"
}
