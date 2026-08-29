package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexWSConfigGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	config := CodexWSConfig{
		BaseURL:       "http://127.0.0.1:20128/v1",
		Model:         "gpt-5.5",
		ProviderName:  "omniroute-local",
		APIKeyEnvName: "OMNIROUTE_LOCAL_KEY",
		APIKey:        "test-key",
		CodexHome:     filepath.Join(tmpDir, ".codex-ws"),
		TrustDir:      tmpDir,
	}

	configPath, err := GenerateCodexWSConfig(config)
	if err != nil {
		t.Fatalf("生成配置失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(configPath); err != nil {
		t.Fatal("配置文件应存在")
	}

	// 读取并验证内容
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("读取配置文件失败: %v", err)
	}
	configStr := string(content)

	// 验证关键配置项
	if !strings.Contains(configStr, `model = "gpt-5.5"`) {
		t.Fatal("配置应包含 model = gpt-5.5")
	}
	if !strings.Contains(configStr, `supports_websockets = true`) {
		t.Fatal("配置应包含 supports_websockets = true")
	}
	if !strings.Contains(configStr, `wire_api = "responses"`) {
		t.Fatal("配置应包含 wire_api = responses")
	}
	if !strings.Contains(configStr, `base_url = "http://127.0.0.1:20128/v1"`) {
		t.Fatal("配置应包含 base_url")
	}
	if !strings.Contains(configStr, `env_key = "OMNIROUTE_LOCAL_KEY"`) {
		t.Fatal("配置应包含 env_key")
	}

	// 验证 env.sh 也被创建
	envPath := filepath.Join(config.CodexHome, "env.sh")
	if _, err := os.Stat(envPath); err != nil {
		t.Fatal("env.sh 应存在")
	}

	envContent, _ := os.ReadFile(envPath)
	envStr := string(envContent)
	if !strings.Contains(envStr, `CODEX_HOME="`) {
		t.Fatal("env.sh 应包含 CODEX_HOME")
	}
	if !strings.Contains(envStr, `OMNIROUTE_LOCAL_KEY="test-key"`) {
		t.Fatal("env.sh 应包含 API key")
	}
}

func TestCodexWSConfigIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "custom-codex-home")

	config := CodexWSConfig{
		BaseURL:       "http://localhost:3000/v1",
		Model:         "gpt-4",
		ProviderName:  "seasagi-local",
		APIKeyEnvName: "MY_KEY",
		APIKey:        "secret",
		CodexHome:     codexHome,
		TrustDir:      tmpDir,
	}

	configPath, err := GenerateCodexWSConfig(config)
	if err != nil {
		t.Fatalf("生成配置失败: %v", err)
	}

	// 验证配置在隔离目录中
	if !strings.HasPrefix(configPath, codexHome) {
		t.Fatalf("配置文件应在 CODEX_HOME 中，实际 %s", configPath)
	}

	// 验证 CODEX_HOME 目录被创建
	if _, err := os.Stat(codexHome); err != nil {
		t.Fatal("CODEX_HOME 目录应被创建")
	}

	// 验证不会污染默认的 ~/.codex
	defaultCodex := filepath.Join(os.Getenv("HOME"), ".codex")
	// 只有当默认 .codex 不存在时才验证（避免在已有配置的环境上误判）
	if _, err := os.Stat(defaultCodex); err == nil {
		// 如果 .codex 存在，检查我们的配置不在其中
		ourConfig := filepath.Join(defaultCodex, "config.toml")
		ourContent, err := os.ReadFile(ourConfig)
		if err == nil && strings.Contains(string(ourContent), "seasagi-local") {
			t.Fatal("配置不应污染默认 ~/.codex/config.toml")
		}
	}
}

func TestCodexWSModelNameStrip(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"codex/gpt-5.5", "gpt-5.5"},
		{"openai/gpt-4", "gpt-4"},
		{"anthropic/claude-3", "claude-3"},
		{"gpt-4", "gpt-4"},          // 无前缀
		{"gpt-4/mini", "mini"},      // 多个斜线取第一个
		{"", ""},                    // 空字符串
	}

	for _, tt := range tests {
		result := StripProviderPrefix(tt.input)
		if result != tt.expected {
			t.Fatalf("StripProviderPrefix(%q) = %q, 期望 %q", tt.input, result, tt.expected)
		}
	}
}

func TestValidateCodexWSConfig(t *testing.T) {
	// 合法配置
	validConfig := CodexWSConfig{
		BaseURL:       "http://localhost:3000/v1",
		Model:         "gpt-4",
		ProviderName:  "test",
		APIKeyEnvName: "KEY",
		CodexHome:     "/tmp/codex",
	}
	if err := ValidateCodexWSConfig(validConfig); err != nil {
		t.Fatalf("合法配置应通过验证: %v", err)
	}

	// 缺少 base URL
	invalidConfig := validConfig
	invalidConfig.BaseURL = ""
	if err := ValidateCodexWSConfig(invalidConfig); err == nil {
		t.Fatal("缺少 base URL 应验证失败")
	}

	// model 含 provider 前缀
	invalidConfig2 := validConfig
	invalidConfig2.Model = "codex/gpt-4"
	if err := ValidateCodexWSConfig(invalidConfig2); err == nil {
		t.Fatal("model 含 provider 前缀应验证失败")
	}
}

func TestDefaultCodexWSConfig(t *testing.T) {
	config := DefaultCodexWSConfig()

	if config.BaseURL == "" {
		t.Fatal("默认 base URL 不应为空")
	}
	if config.Model == "" {
		t.Fatal("默认 model 不应为空")
	}
	if !strings.Contains(config.CodexHome, ".codex-ws") {
		t.Fatal("默认 CODEX_HOME 应包含 .codex-ws")
	}
	// 默认 model 不应包含 provider 前缀
	if strings.Contains(config.Model, "/") {
		t.Fatal("默认 model 不应包含 provider 前缀")
	}
}
