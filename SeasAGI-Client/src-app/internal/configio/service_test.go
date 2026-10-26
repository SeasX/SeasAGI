package configio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func TestNewServiceResolvesHomeDir(t *testing.T) {
	s := NewService(nil)
	if s == nil {
		t.Fatal("expected non-nil service")
	}
	if s.homeDir == "" {
		t.Fatal("expected home directory to be resolved")
	}
}

func TestImportRejectsMalformedJSON(t *testing.T) {
	s := NewService(nil)
	if err := s.Import([]byte("not json")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestImportRejectsMissingVersion(t *testing.T) {
	s := NewService(nil)
	if err := s.Import([]byte(`{"channels":[]}`)); err == nil {
		t.Fatal("expected error for missing version")
	}
}

// TestExportChannelsRedactCredentials 断言导出通道数据中不含 API Key / OAuth 凭证（CL-S1）。
func TestExportChannelsRedactCredentials(t *testing.T) {
	channels := []config.Channel{{
		ChannelID:         "ch_export",
		ChannelType:       "custom",
		ProviderType:      "openai",
		DisplayName:       "exported-channel",
		BaseURL:           "https://api.example.com",
		Enabled:           true,
		APIKey:            "sk-export-single",
		APIKeys:           []string{"sk-export-single", "sk-export-second"},
		OAuthRefreshToken: "oauth-export-refresh",
		OAuthClientSecret: "oauth-export-secret",
		Models:            []string{"gpt-4o"},
	}}

	raw, err := json.Marshal(redactChannels(channels))
	if err != nil {
		t.Fatalf("marshal redacted channels: %v", err)
	}
	out := string(raw)
	for _, secret := range []string{"sk-export-single", "sk-export-second", "oauth-export-refresh", "oauth-export-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("导出数据泄露凭证 %q: %s", secret, out)
		}
	}
	for _, field := range []string{`"api_key"`, `"api_keys"`, `"oauth_refresh_token"`, `"oauth_client_secret"`} {
		if strings.Contains(out, field) {
			t.Fatalf("导出数据不应包含凭证字段 %s: %s", field, out)
		}
	}
	if !strings.Contains(out, `"display_name":"exported-channel"`) || !strings.Contains(out, `"base_url":"https://api.example.com"`) {
		t.Fatalf("导出数据应保留展示字段: %s", out)
	}
}

// TestExportRedactsMCPSecretsAndMarksFlag 断言导出结果标记 secrets_redacted，
// 且 MCP env / args 与 URL 查询参数中的敏感值被脱敏。
func TestExportRedactsMCPSecretsAndMarksFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	seasagiDir := filepath.Join(home, ".seasagi")
	if err := os.MkdirAll(seasagiDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mcp := []MCPServerExport{{
		Name:      "srv",
		Command:   "node",
		Args:      []string{"server.js", "--api-key", "sk-arg-secret", "--port", "8080"},
		Env:       map[string]string{"OPENAI_API_KEY": "sk-env-secret", "LOG_LEVEL": "debug"},
		Transport: "stdio",
		Enabled:   true,
		Apps:      []string{"claude"},
	}}
	mcpRaw, err := json.Marshal(mcp)
	if err != nil {
		t.Fatalf("marshal mcp: %v", err)
	}
	if err := os.WriteFile(filepath.Join(seasagiDir, "mcp_servers.json"), mcpRaw, 0o644); err != nil {
		t.Fatalf("write mcp: %v", err)
	}

	cfgSvc := config.NewServiceWithPath(config.AppConfig{
		SelectedGrantRelayURL: "https://user:pass@relay.example.com/api?token=abc&keep=1",
		MarketNode:            &config.MarketNodeConfig{NodeURL: "https://node.example.com/x?signature=zzz&region=us"},
	}, filepath.Join(home, "config.json"))

	raw, err := NewService(cfgSvc).Export()
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	out := string(raw)

	var data ExportData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	if !data.SecretsRedacted {
		t.Error("secrets_redacted should be true")
	}
	if len(data.MCPServers) != 1 {
		t.Fatalf("expected 1 mcp server, got %d", len(data.MCPServers))
	}
	server := data.MCPServers[0]
	if got := server.Env["OPENAI_API_KEY"]; got != redactedValue {
		t.Errorf("OPENAI_API_KEY = %q, want %q", got, redactedValue)
	}
	if got := server.Env["LOG_LEVEL"]; got != "debug" {
		t.Errorf("LOG_LEVEL = %q, want debug", got)
	}
	if got := server.Args[2]; got != redactedValue {
		t.Errorf("args[2] = %q, want %q", got, redactedValue)
	}
	if got := server.Args[4]; got != "8080" {
		t.Errorf("args[4] = %q, want 8080", got)
	}
	for _, secret := range []string{"sk-env-secret", "sk-arg-secret", "user:pass", "token=abc", "signature=zzz"} {
		if strings.Contains(out, secret) {
			t.Fatalf("导出数据泄露敏感值 %q: %s", secret, out)
		}
	}
	if !strings.Contains(data.AppConfig.SelectedGrantRelayURL, "relay.example.com") ||
		!strings.Contains(data.AppConfig.MarketNode.NodeURL, "node.example.com") {
		t.Errorf("URL 脱敏应保留 scheme/host/path: %+v", data.AppConfig)
	}
}

// TestImportWritesPrivatePermissions 断言导入落盘目录 0700、文件 0600。
func TestImportWritesPrivatePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cfgSvc := config.NewServiceWithPath(config.AppConfig{}, filepath.Join(home, "config.json"))
	s := NewService(cfgSvc)

	data := ExportData{
		Version: "1.0",
		MCPServers: []MCPServerExport{{
			Name:      "srv",
			Command:   "node",
			Transport: "stdio",
			Enabled:   true,
		}},
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal import data: %v", err)
	}
	if err := s.Import(raw); err != nil {
		t.Fatalf("Import: %v", err)
	}

	dir := filepath.Join(home, ".seasagi")
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat mcp dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("mcp dir perm = %o, want 0700", perm)
	}
	fileInfo, err := os.Stat(filepath.Join(dir, "mcp_servers.json"))
	if err != nil {
		t.Fatalf("stat mcp file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("mcp file perm = %o, want 0600", perm)
	}
}
