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

// 生产模式下 "*" 必须被拒绝，避免退化为反射任意 Origin。
func TestBuildCORSPolicyRejectsWildcardInProduction(t *testing.T) {
	if _, err := buildCORSPolicy("*", true); err == nil {
		t.Fatal("expected wildcard to be rejected in production mode")
	}
	if _, err := buildCORSPolicy("https://app.example.com,*", true); err == nil {
		t.Fatal("expected wildcard in mixed list to be rejected in production mode")
	}
}

// 开发模式下 "*" 不构成白名单，最终只保留本机来源。
func TestBuildCORSPolicyIgnoresWildcardOutsideProduction(t *testing.T) {
	policy, err := buildCORSPolicy("*", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.origins["*"] {
		t.Fatal("wildcard must not be stored as an allowed origin")
	}
	if !policy.allows("http://localhost:5173") {
		t.Fatal("loopback origin should be allowed when no explicit origins configured")
	}
	if policy.allows("https://evil.example.com") {
		t.Fatal("non-loopback origin must not be allowed")
	}
}

// 未配置时默认只允许本机来源（任意端口）。
func TestBuildCORSPolicyDefaultsToLoopback(t *testing.T) {
	policy, err := buildCORSPolicy("", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	allowed := []string{
		"http://localhost:5173",
		"http://127.0.0.1:9318",
		"http://[::1]:9318",
		"https://localhost",
	}
	for _, origin := range allowed {
		if !policy.allows(origin) {
			t.Fatalf("loopback origin %q should be allowed", origin)
		}
	}
	denied := []string{"https://evil.example.com", "null", "", "http://192.168.1.10:9318"}
	for _, origin := range denied {
		if policy.allows(origin) {
			t.Fatalf("origin %q should not be allowed", origin)
		}
	}
}

// 显式白名单必须精确匹配，且不再回退到本机来源。
func TestBuildCORSPolicyUsesExplicitWhitelist(t *testing.T) {
	policy, err := buildCORSPolicy("https://app.example.com, https://admin.example.com/", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !policy.allows("https://app.example.com") || !policy.allows("https://admin.example.com") {
		t.Fatal("explicit origins should be allowed")
	}
	if policy.allows("http://localhost:5173") {
		t.Fatal("loopback fallback must be disabled when explicit whitelist is configured")
	}
	if policy.allows("https://app.example.com.evil.com") {
		t.Fatal("origin matching must be exact")
	}
}

// CORS 中间件只对命中白名单的 Origin 回显并允许凭证。
func TestCORSMiddlewareAllowsOnlyWhitelistedOrigin(t *testing.T) {
	t.Setenv("SEASAGI_ALLOW_DEV_SECRETS", "1")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")

	router := gin.New()
	router.Use(corsMiddleware())
	router.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Origin", "https://app.example.com")
	router.ServeHTTP(allowed, req)
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allow origin = %q, want whitelisted origin", got)
	}
	if got := allowed.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q, want true", got)
	}

	denied := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	router.ServeHTTP(denied, req)
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unexpected allow origin for non-whitelisted request: %q", got)
	}
}

// 请求体超过上限时入口直接返回 413。
func TestBodyLimitMiddlewareRejectsOversizedBody(t *testing.T) {
	router := gin.New()
	router.Use(bodyLimitMiddleware(8))
	router.POST("/echo", func(c *gin.Context) { c.String(200, "ok") })

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/echo", strings.NewReader("0123456789"))
	router.ServeHTTP(recorder, req)

	if recorder.Code != 413 {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

// debug 日志只允许在显式开关下输出，且敏感字段必须打码。
func TestMaskSensitiveBodyRedactsSecrets(t *testing.T) {
	body := []byte(`{"email":"a@b.com","password":"p@ss","api_key":"sk-secret","nested":{"refresh_token":"rt"},"models":["m1"]}`)
	masked := maskSensitiveBody(body)

	for _, secret := range []string{"p@ss", "sk-secret", "rt"} {
		if strings.Contains(masked, secret) {
			t.Fatalf("masked body leaked %q: %s", secret, masked)
		}
	}
	if !strings.Contains(masked, "a@b.com") {
		t.Fatalf("non-sensitive field should be preserved: %s", masked)
	}
}

// 非 JSON 正文不得落盘原文。
func TestMaskSensitiveBodyOmitsNonJSON(t *testing.T) {
	if got := maskSensitiveBody([]byte("password=plain-text")); strings.Contains(got, "plain-text") {
		t.Fatalf("non-JSON body should be omitted, got %q", got)
	}
}
