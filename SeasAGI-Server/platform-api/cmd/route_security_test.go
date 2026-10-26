package cmd

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

// isPublicWriteRoute 判断写路由是否属于公开白名单：
// 登录/注册/刷新与 OAuth 端点由 handler 自身校验参数或第三方签名，
// 其余写路由必须先经过 JWT / ADMIN_SECRET / INTERNAL_API_TOKEN 中间件。
func isPublicWriteRoute(path string) bool {
	switch path {
	case "/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/auth/refresh":
		return true
	case "/healthz":
		return true
	}
	return strings.HasPrefix(path, "/api/v1/auth/oauth/") ||
		strings.HasPrefix(path, "/api/v1/version/")
}

func initRouteSecurityDB(t *testing.T) {
	t.Helper()

	database.Close()
	if err := database.Init(filepath.Join(t.TempDir(), "route-security.db")); err != nil {
		t.Fatalf("database.Init failed: %v", err)
	}
	t.Cleanup(database.Close)
}

func setRouteSecurityEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIN_MODE", "test")
	t.Setenv("SEASAGI_ALLOW_DEV_SECRETS", "1")
	t.Setenv("JWT_SECRET", "route-security-jwt-secret-0123456789")
	t.Setenv("JWT_REFRESH_SECRET", "route-security-refresh-secret-0123456789")
	t.Setenv("ADMIN_SECRET", "route-security-admin-secret")
	t.Setenv("INTERNAL_API_TOKEN", "route-security-internal-token")
}

// concretePath 把 gin 路由模板中的参数段替换为固定值，便于真实发起 HTTP 请求。
func concretePath(routePath string) string {
	segments := strings.Split(routePath, "/")
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
			segments[i] = "test"
		}
	}
	return strings.Join(segments, "/")
}

// TestRouteSecurityAllWriteEndpoints 遍历全部写路由，确保未挂鉴权的接口在无凭证时
// 返回 401/403（/internal 允许 503 表示未配置密钥即拒绝服务）。
func TestRouteSecurityAllWriteEndpoints(t *testing.T) {
	initRouteSecurityDB(t)
	setRouteSecurityEnv(t)

	router, err := BuildRouter()
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}

	writeRoutes := 0
	for _, route := range router.Routes() {
		switch route.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			continue
		}
		writeRoutes++

		if isPublicWriteRoute(route.Path) {
			continue
		}

		req := httptest.NewRequest(route.Method, concretePath(route.Path), nil)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		status := rec.Code
		if strings.HasPrefix(route.Path, "/api/v1/internal/") {
			if status != http.StatusUnauthorized && status != http.StatusForbidden && status != http.StatusServiceUnavailable {
				t.Errorf("unprotected internal route %s %s: got %d, want 401/403/503", route.Method, route.Path, status)
			}
			continue
		}
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("unprotected route %s %s: got %d, want 401/403", route.Method, route.Path, status)
		}
	}

	t.Logf("platform-api write routes covered: %d", writeRoutes)
	if writeRoutes == 0 {
		t.Fatal("no write routes discovered; route registration is broken")
	}
}
