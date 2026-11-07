package cmd

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// 生产模式下 "*" 必须被拒绝（原实现默认 `*`，等价于任意站点可跨域读）。
func TestBuildCORSPolicyRejectsWildcardInProduction(t *testing.T) {
	if _, err := buildCORSPolicy("*", true); err == nil {
		t.Fatal("expected wildcard to be rejected in production mode")
	}
}

// 未配置时默认只允许本机来源；通配符不构成白名单条目。
func TestBuildCORSPolicyDefaultsToLoopback(t *testing.T) {
	policy, err := buildCORSPolicy("*", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.origins["*"] {
		t.Fatal("wildcard must not be stored as an allowed origin")
	}
	if !policy.allows("http://localhost:5173") || !policy.allows("http://127.0.0.1:8318") {
		t.Fatal("loopback origins should be allowed by default")
	}
	if policy.allows("https://evil.example.com") {
		t.Fatal("non-loopback origin must not be allowed")
	}
}

// 显式白名单精确匹配。
func TestBuildCORSPolicyUsesExplicitWhitelist(t *testing.T) {
	policy, err := buildCORSPolicy("https://console.example.com", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !policy.allows("https://console.example.com") {
		t.Fatal("explicit origin should be allowed")
	}
	if policy.allows("http://localhost:5173") {
		t.Fatal("loopback fallback must be disabled when explicit whitelist is configured")
	}
}

// 中间件只对白名单 Origin 下发跨域头。
func TestCORSMiddlewareAllowsOnlyWhitelistedOrigin(t *testing.T) {
	t.Setenv("SEASAGI_ALLOW_DEV_SECRETS", "1")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://console.example.com")

	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/healthz", func(c *gin.Context) { c.String(200, "ok") })

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("Origin", "https://console.example.com")
	router.ServeHTTP(allowed, req)
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Fatalf("allow origin = %q", got)
	}

	denied := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	router.ServeHTTP(denied, req)
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin: %q", got)
	}
}

// 请求体超过上限时入口直接返回 413。
func TestBodyLimitMiddlewareRejectsOversizedBody(t *testing.T) {
	router := gin.New()
	router.Use(bodyLimitMiddleware(8))
	router.POST("/relay/chat/completions", func(c *gin.Context) { c.String(200, "ok") })

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/relay/chat/completions", strings.NewReader("0123456789"))
	router.ServeHTTP(recorder, req)

	if recorder.Code != 413 {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}
