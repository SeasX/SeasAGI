package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

// isPublicWriteRoute 判断写路由是否公开：社区版 relay-gateway 的数据面与运维面
// 全部要求凭证，没有公开写端点；/relay/health、/healthz、/metrics 均为 GET。
func isPublicWriteRoute(path string) bool {
	switch path {
	case "/relay/health", "/healthz", "/metrics":
		return true
	}
	return false
}

func TestRelayRouteSecurityAllWriteEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ADMIN_SECRET", "relay-route-security-admin-secret")
	t.Setenv("JWT_SECRET", "relay-route-security-jwt-secret-0123456789")

	router, err := BuildRouter()
	if err != nil {
		t.Fatalf("BuildRouter failed: %v", err)
	}
	t.Cleanup(func() {
		if routerStore != nil {
			_ = routerStore.Close()
		}
	})

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
		if strings.HasPrefix(route.Path, "/ops") {
			if status != http.StatusUnauthorized && status != http.StatusForbidden && status != http.StatusServiceUnavailable {
				t.Errorf("unprotected ops route %s %s: got %d, want 401/403/503", route.Method, route.Path, status)
			}
			continue
		}
		if status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("unprotected relay route %s %s: got %d, want 401/403", route.Method, route.Path, status)
		}
	}

	t.Logf("relay-gateway write routes covered: %d", writeRoutes)
	if writeRoutes == 0 {
		t.Fatal("no write routes discovered; route registration is broken")
	}
}
