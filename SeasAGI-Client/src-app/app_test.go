package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/gateway"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
)

// newTestApp 直构一个仅挂载 RTK/Caveman 设置链路所需服务的 App。
// HOME 隔离避免日志/状态存储写入真实用户目录。
func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfgSvc := config.NewServiceWithPath(config.AppConfig{}, filepath.Join(t.TempDir(), "config.json"))
	return &App{
		authSvc:    auth.NewService(),
		configSvc:  cfgSvc,
		gatewaySvc: gateway.NewService(0, "test-token", cfgSvc, auth.NewService(), logs.NewService()),
		logSvc:     logs.NewService(),
	}
}

// TestAppSetRTKSettingsPersistsConfig 覆盖设置页「启用按钮」的绑定层：
// SetRTKSettings 应把 RTK/Caveman 两组值完整持久化，供网关下一请求读取。
func TestAppSetRTKSettingsPersistsConfig(t *testing.T) {
	app := newTestApp(t)

	if err := app.SetRTKSettings(false, 8000, true, "terse"); err != nil {
		t.Fatalf("SetRTKSettings: %v", err)
	}

	cfg := app.configSvc.GetConfig()
	if cfg.RTKEnabled {
		t.Error("RTKEnabled = true, want false")
	}
	if cfg.RTKMaxOutputChars != 8000 {
		t.Errorf("RTKMaxOutputChars = %d, want 8000", cfg.RTKMaxOutputChars)
	}
	if !cfg.CavemanEnabled || cfg.CavemanStyle != "terse" {
		t.Errorf("Caveman = (%v, %q), want (true, terse)", cfg.CavemanEnabled, cfg.CavemanStyle)
	}
}

// TestAppSetRTKSettingsRejectsInvalidStyle 非法风格必须在校验层被拒，
// 且不得写入任何配置（避免注入时静默降级为 concise）。
func TestAppSetRTKSettingsRejectsInvalidStyle(t *testing.T) {
	app := newTestApp(t)

	if err := app.SetRTKSettings(true, 8000, true, "shouty"); err == nil {
		t.Fatal("SetRTKSettings with invalid style should fail")
	}

	// 拒绝后配置应保持初始值（未写入 enabled/style）。
	cfg := app.configSvc.GetConfig()
	if cfg.RTKEnabled {
		t.Error("RTKEnabled must stay false after rejected call")
	}
	if cfg.CavemanEnabled || cfg.CavemanStyle != "" {
		t.Errorf("Caveman = (%v, %q), want untouched (false, \"\")", cfg.CavemanEnabled, cfg.CavemanStyle)
	}
}

// TestAppSetRTKSettingsAcceptsEmptyStyle 空风格表示「保持既有值」，应被放行。
func TestAppSetRTKSettingsAcceptsEmptyStyle(t *testing.T) {
	app := newTestApp(t)

	if err := app.SetRTKSettings(true, 8000, true, ""); err != nil {
		t.Fatalf("SetRTKSettings with empty style: %v", err)
	}
	if cfg := app.configSvc.GetConfig(); !cfg.RTKEnabled {
		t.Error("RTKEnabled = false, want true")
	}
}

// TestCurlCommandUsesCredentialPlaceholders 校验 Playground 回传的 curl 命令
// 只使用环境变量占位符，不含任何明文凭证（CL-S2）。
func TestCurlCommandUsesCredentialPlaceholders(t *testing.T) {
	headers := map[string]string{
		"channel": curlChannelKeyAuthHeader,
		"local":   curlLocalTokenAuthHeader,
	}
	for name, header := range headers {
		if !strings.Contains(header, "$") {
			t.Fatalf("%s 鉴权头必须是环境变量占位符，got %q", name, header)
		}
		cmd := buildCurlCommand("http://127.0.0.1:4318/v1/images/generations", header, `{"prompt":"p"}`)
		for _, secret := range []string{"sk-", "oauth_", "Bearer eyJ"} {
			if strings.Contains(cmd, secret) {
				t.Fatalf("%s curl 命令不得包含明文凭证 %q: %s", name, secret, cmd)
			}
		}
	}
}
