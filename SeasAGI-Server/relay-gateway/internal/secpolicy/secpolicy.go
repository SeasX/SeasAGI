package secpolicy

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// weakPatterns are substrings that indicate a placeholder or default secret.
var weakPatterns = []string{
	"change-me",
	"default",
	"xxxxxxxx",
	"sk_test_xxx",
	"placeholder",
	"your-",
	"example",
}

// minSecretLength is the minimum acceptable length for production secrets.
const minSecretLength = 16

// ensureSecrets ensures JWT_SECRET and JWT_REFRESH_SECRET are set.
// If they are empty, auto-generates secure random values so the build artifact
// can start without a .env file.
func ensureSecrets() {
	secrets := []string{"JWT_SECRET", "JWT_REFRESH_SECRET"}
	for _, name := range secrets {
		if os.Getenv(name) == "" {
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				logging.Warningf("failed to auto-generate %s: %v", name, err)
				continue
			}
			os.Setenv(name, hex.EncodeToString(b))
			logging.Warningf("%s not set — auto-generated a random secret. Set it explicitly in production via .env.", name)
		}
	}
}

// ValidateStartup checks configured secrets for weak/default values.
// In production mode (GIN_MODE != "debug"), it logs a fatal error and exits
// if any weak secret is detected. In debug mode, it only logs warnings.
// If secrets are empty, auto-generates secure random defaults.
func ValidateStartup() {
	ensureSecrets()

	secrets := []string{
		"JWT_SECRET",
		"JWT_REFRESH_SECRET",
	}

	isDebug := os.Getenv("GIN_MODE") == "debug"
	weak := false

	for _, name := range secrets {
		val := os.Getenv(name)
		reason := weakSecretReason(name, val)
		if reason != "" {
			if isDebug {
				logging.Warningf("weak secret %s: %s", name, reason)
			} else {
				logging.Errorf("FATAL: weak secret %s: %s", name, reason)
				weak = true
			}
		}
	}

	if weak && !isDebug {
		logging.Fatal("Startup aborted: weak or default secrets detected in production mode. Set strong secrets in your .env file before starting.")
	}
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
