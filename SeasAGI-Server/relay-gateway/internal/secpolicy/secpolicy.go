package secpolicy

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// weakPatterns are substrings that indicate a placeholder, default or
// previously shipped development secret.
var weakPatterns = []string{
	"change-me",
	"change-in-production",
	"default",
	"xxxxxxxx",
	"sk_test_xxx",
	"placeholder",
	"your-",
	"example",
	"seasagi-dev-",
	"seasagi-market-default",
}

// minSecretLength is the minimum acceptable length for production secrets.
const minSecretLength = 16

// requiredSecrets 是 relay-gateway 生产环境必须显式配置的共享密钥。
var requiredSecrets = []string{
	"JWT_SECRET",
	"JWT_REFRESH_SECRET",
	"ADMIN_SECRET",
}

// devMode 判定是否允许开发期自动补全密钥。
func devMode() bool {
	return os.Getenv("GIN_MODE") == "debug" || os.Getenv("SEASAGI_ALLOW_DEV_SECRETS") == "1"
}

// ensureSecrets 仅用于开发模式：为空密钥生成随机值，保证本地可直接启动。
func ensureSecrets() {
	for _, name := range requiredSecrets {
		if os.Getenv(name) == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				logging.Warningf("failed to auto-generate %s: %v", name, err)
				continue
			}
			os.Setenv(name, hex.EncodeToString(b))
			logging.Warningf("[dev] %s not set — auto-generated a random secret. Set it explicitly in production via .env.", name)
		}
	}
}

// ValidateStartup 校验生产环境密钥：缺失或过弱一律终止启动。
// 开发模式（GIN_MODE=debug 或 SEASAGI_ALLOW_DEV_SECRETS=1）只告警并自动补全。
func ValidateStartup() {
	isDebug := devMode()
	if isDebug {
		ensureSecrets()
	}

	var missing, weak []string
	for _, name := range requiredSecrets {
		val := strings.TrimSpace(os.Getenv(name))
		if val == "" {
			missing = append(missing, name)
			continue
		}
		if reason := weakSecretReason(name, val); reason != "" {
			weak = append(weak, name+" ("+reason+")")
		}
	}

	if len(missing) == 0 && len(weak) == 0 {
		return
	}

	if isDebug {
		if len(missing) > 0 {
			logging.Warningf("[dev] missing secrets: %v", missing)
		}
		if len(weak) > 0 {
			logging.Warningf("[dev] weak secrets: %v", weak)
		}
		return
	}

	if len(missing) > 0 {
		logging.Errorf("FATAL: missing required secrets: %v", missing)
	}
	if len(weak) > 0 {
		logging.Errorf("FATAL: weak secrets: %v", weak)
	}
	logging.Fatal("Startup aborted: required secrets are missing or weak in production mode. Set explicit strong values in .env before starting.")
}

// weakSecretReason returns a non-empty string if the secret value is weak.
func weakSecretReason(name, val string) string {
	if val == "" {
		return "not set (empty)"
	}
	if len(val) < minSecretLength {
		return "too short (minimum 16 characters)"
	}
	valLower := strings.ToLower(val)
	for _, pattern := range weakPatterns {
		if strings.Contains(valLower, pattern) {
			return "contains weak pattern '" + pattern + "' — looks like a placeholder or default value"
		}
	}
	return ""
}
