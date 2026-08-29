package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// CodexWSConfig Codex WebSocket 配置参数。
type CodexWSConfig struct {
	BaseURL           string // OmniRoute 本地 base URL
	Model             string // 纯模型 ID（去除 provider 前缀）
	ProviderName      string // model_provider 名称
	APIKeyEnvName     string // API key 的环境变量名
	APIKey            string // API key 值
	CodexHome         string // CODEX_HOME 隔离目录
	TrustDir          string // 信任目录
	ExtraProviderOpts map[string]string // 额外的 provider 选项
}

// DefaultCodexWSConfig 返回默认配置。
func DefaultCodexWSConfig() CodexWSConfig {
	homeDir, _ := os.UserHomeDir()
	return CodexWSConfig{
		BaseURL:       "http://127.0.0.1:20128/v1",
		Model:         "gpt-5.5",
		ProviderName:  "omniroute-local",
		APIKeyEnvName: "OMNIROUTE_LOCAL_KEY",
		APIKey:        "local",
		CodexHome:     filepath.Join(homeDir, ".codex-ws"),
		TrustDir:      homeDir,
	}
}

// codexConfigTemplate config.toml 模板。
const codexConfigTemplate = `# 由 SeasAGI 生成 — Codex WebSocket 隔离配置
model = "{{.Model}}"
model_provider = "{{.ProviderName}}"

[model_providers.{{.ProviderName}}]
name = "SeasAGI Local (WS)"
base_url = "{{.BaseURL}}"
wire_api = "responses"
supports_websockets = true
env_key = "{{.APIKeyEnvName}}"
{{range $key, $val := .ExtraProviderOpts}}{{$key}} = {{$val}}
{{end}}
[projects."{{.TrustDir}}"]
trust_level = "trusted"
`

// GenerateCodexWSConfig 生成 Codex WebSocket 配置文件。
// 创建 $CODEX_HOME/config.toml 并返回文件路径。
func GenerateCodexWSConfig(config CodexWSConfig) (string, error) {
	if config.BaseURL == "" {
		return "", fmt.Errorf("base URL 不能为空")
	}
	if config.Model == "" {
		return "", fmt.Errorf("model 不能为空")
	}
	if config.CodexHome == "" {
		return "", fmt.Errorf("CODEX_HOME 不能为空")
	}

	// 确保隔离目录存在
	if err := os.MkdirAll(config.CodexHome, 0755); err != nil {
		return "", fmt.Errorf("创建 CODEX_HOME 失败: %w", err)
	}

	// 渲染模板
	tmpl, err := template.New("codex-config").Parse(codexConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("模板解析失败: %w", err)
	}

	configPath := filepath.Join(config.CodexHome, "config.toml")
	f, err := os.Create(configPath)
	if err != nil {
		return "", fmt.Errorf("创建配置文件失败: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, config); err != nil {
		return "", fmt.Errorf("模板渲染失败: %w", err)
	}

	// 写入环境变量文件（供 source 使用）
	envPath := filepath.Join(config.CodexHome, "env.sh")
	envContent := fmt.Sprintf(`#!/bin/bash
export %s="%s"
export CODEX_HOME="%s"
`, config.APIKeyEnvName, config.APIKey, config.CodexHome)
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		return "", fmt.Errorf("写入 env.sh 失败: %w", err)
	}

	return configPath, nil
}

// StripProviderPrefix 去除模型 ID 的 provider 前缀。
// 如 "codex/gpt-5.5" → "gpt-5.5"，"openai/gpt-4" → "gpt-4"。
// 如果没有前缀，则原样返回。
func StripProviderPrefix(modelID string) string {
	idx := strings.Index(modelID, "/")
	if idx == -1 {
		return modelID
	}
	return modelID[idx+1:]
}

// ValidateCodexWSConfig 验证配置。
func ValidateCodexWSConfig(config CodexWSConfig) error {
	if config.BaseURL == "" {
		return fmt.Errorf("base URL 不能为空")
	}
	if config.Model == "" {
		return fmt.Errorf("model 不能为空")
	}
	if config.ProviderName == "" {
		return fmt.Errorf("provider name 不能为空")
	}
	if config.APIKeyEnvName == "" {
		return fmt.Errorf("API key env name 不能为空")
	}
	if config.CodexHome == "" {
		return fmt.Errorf("CODEX_HOME 不能为空")
	}

	// model 必须是纯模型 ID（不含 provider 前缀）
	if strings.Contains(config.Model, "/") {
		return fmt.Errorf("model 应为纯模型 ID，不应包含 provider 前缀: %s", config.Model)
	}

	return nil
}

// LoadCodexWSConfig 从环境变量加载配置。
func LoadCodexWSConfig() CodexWSConfig {
	config := DefaultCodexWSConfig()

	if v := os.Getenv("OMNIROUTE_WS_BASE"); v != "" {
		config.BaseURL = v
	}
	if v := os.Getenv("OMNIROUTE_WS_MODEL"); v != "" {
		config.Model = StripProviderPrefix(v)
	}
	if v := os.Getenv("CODEX_WS_HOME"); v != "" {
		config.CodexHome = v
	}
	if v := os.Getenv("OMNIROUTE_LOCAL_KEY"); v != "" {
		config.APIKey = v
	}

	return config
}
