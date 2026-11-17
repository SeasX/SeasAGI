package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/auth"
	"github.com/gin-gonic/gin"
)

// 租户身份只能来自服务端签名令牌。历史上 X-Tenant-Id 被当作回退值，
// 任意客户端可借此伪装成其他租户写 trace、影响路由（P1-4）。
func TestAuthRequiredTenantComesFromSignedTokenOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "tenant-boundary-secret")

	probe := func(token, spoofedTenant string) (int, string) {
		var got string
		r := gin.New()
		r.GET("/probe", AuthRequired(), func(c *gin.Context) {
			got = c.GetString("tenant_id")
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		if spoofedTenant != "" {
			req.Header.Set("X-Tenant-Id", spoofedTenant)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code, got
	}

	signedToken, err := auth.GenerateToken("user-1", "channel-1", "tenant-a")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if code, got := probe(signedToken, "tenant-evil"); code != http.StatusOK || got != "tenant-a" {
		t.Fatalf("signed tenant overridden by header: code=%d tenant=%q, want 200/tenant-a", code, got)
	}

	// 令牌未携带 tenant_id 时统一落到 default，绝不采信请求头。
	legacyToken, err := auth.GenerateToken("user-1", "channel-1")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if code, got := probe(legacyToken, "tenant-evil"); code != http.StatusOK || got != "default" {
		t.Fatalf("header used as fallback: code=%d tenant=%q, want 200/default", code, got)
	}
	if code, got := probe(legacyToken, ""); code != http.StatusOK || got != "default" {
		t.Fatalf("missing tenant claim: code=%d tenant=%q, want 200/default", code, got)
	}
}
