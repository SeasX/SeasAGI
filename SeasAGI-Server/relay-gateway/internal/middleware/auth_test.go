package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 运维面 /ops 必须鉴权：未配置密钥时拒绝服务，密钥错误返回 401（P0-06 回归）。
func TestAdminAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("unconfigured secret rejects", func(t *testing.T) {
		t.Setenv("ADMIN_SECRET", "")
		mw := AdminAuth()
		w := httptest.NewRecorder()
		r := gin.New()
		r.GET("/ops/channels", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ops/channels", nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 when admin secret missing, got %d", w.Code)
		}
	})

	t.Run("wrong and missing token", func(t *testing.T) {
		t.Setenv("ADMIN_SECRET", "ops-secret")
		mw := AdminAuth()
		for _, header := range []string{"", "Bearer nope"} {
			w := httptest.NewRecorder()
			r := gin.New()
			r.GET("/ops/channels", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
			req := httptest.NewRequest(http.MethodGet, "/ops/channels", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 for header %q, got %d", header, w.Code)
			}
		}
	})

	t.Run("valid token passes", func(t *testing.T) {
		t.Setenv("ADMIN_SECRET", "ops-secret")
		mw := AdminAuth()
		w := httptest.NewRecorder()
		r := gin.New()
		r.GET("/ops/channels", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/ops/channels", nil)
		req.Header.Set("Authorization", "Bearer ops-secret")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}
