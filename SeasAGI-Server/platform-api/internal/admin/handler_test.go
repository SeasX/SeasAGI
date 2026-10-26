package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// 管理端必须拒绝匿名/错误密钥，只接受正确的 ADMIN_SECRET（P0-05 回归）。
func TestMiddlewareRequiresAdminSecret(t *testing.T) {
	t.Setenv("ADMIN_SECRET", "test-admin-secret")
	gin.SetMode(gin.TestMode)

	mw := Middleware()
	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"wrong", "Bearer wrong-secret", http.StatusUnauthorized},
		{"empty-bearer", "Bearer ", http.StatusUnauthorized},
		{"valid", "Bearer test-admin-secret", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := gin.New()
			r.GET("/api/v1/admin/users", mw, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			r.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, w.Code, w.Body.String())
			}
		})
	}
}
