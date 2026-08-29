package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCLISetupClaudeCode(t *testing.T) {
	tmpDir := t.TempDir()
	// 重写 SupportedTools 的路径为临时目录
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := SetupConfig("claude-code", "http://127.0.0.1:4318", "test-key")
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}

	// 验证配置文件存在
	tools := SupportedTools()
	var configPath string
	for _, tool := range tools {
		if tool.Name == "claude-code" {
			configPath = tool.ConfigPath
			break
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	if config["api_base_url"] != "http://127.0.0.1:4318/v1" {
		t.Errorf("expected api_base_url, got %v", config["api_base_url"])
	}
	if config["api_key"] != "test-key" {
		t.Errorf("expected api_key, got %v", config["api_key"])
	}
}

func TestCLISetupCursor(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := SetupConfig("cursor", "http://127.0.0.1:4318", "test-key")
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}

	tools := SupportedTools()
	for _, tool := range tools {
		if tool.Name == "cursor" {
			if _, err := os.Stat(tool.ConfigPath); os.IsNotExist(err) {
				t.Error("expected config file to exist for cursor")
			}
		}
	}
}

func TestCLISetupWindsurf(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := SetupConfig("windsurf", "http://127.0.0.1:4318", "test-key")
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}
}

func TestCLIUnknownTool(t *testing.T) {
	err := SetupConfig("unknown-tool", "http://127.0.0.1:4318", "test-key")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestCLISupportedTools(t *testing.T) {
	tools := SupportedTools()
	if len(tools) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(tools))
	}
}

func TestCLIIsSupported(t *testing.T) {
	if !IsSupported("claude-code") {
		t.Error("expected claude-code to be supported")
	}
	if IsSupported("unknown") {
		t.Error("expected unknown to not be supported")
	}
}

func TestCLIGetToolConfigPath(t *testing.T) {
	path, err := GetToolConfigPath("claude-code")
	if err != nil {
		t.Fatalf("GetToolConfigPath: %v", err)
	}
	if path == "" {
		t.Error("expected non-empty path")
	}
}

func TestCLIMergeExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// 先写入现有配置
	configPath := filepath.Join(tmpDir, ".claude", "settings.json")
	os.MkdirAll(filepath.Dir(configPath), 0755)
	existing := map[string]any{"theme": "dark"}
	data, _ := json.Marshal(existing)
	os.WriteFile(configPath, data, 0644)

	// 执行配置
	err := SetupConfig("claude-code", "http://127.0.0.1:4318", "test-key")
	if err != nil {
		t.Fatalf("SetupConfig: %v", err)
	}

	// 验证合并
	result, _ := os.ReadFile(configPath)
	var config map[string]any
	json.Unmarshal(result, &config)
	if config["theme"] != "dark" {
		t.Error("expected existing config to be preserved")
	}
	if config["api_key"] != "test-key" {
		t.Error("expected new config to be added")
	}
}
