package configio

import (
	"encoding/json"
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
