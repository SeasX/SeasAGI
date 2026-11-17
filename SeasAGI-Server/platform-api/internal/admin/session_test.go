package admin

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func initSessionTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := database.Init(filepath.Join(t.TempDir(), "admin-session.db")); err != nil {
		t.Fatalf("database init failed: %v", err)
	}
	t.Cleanup(database.Close)
}

// 登录端点必须把令牌写进 HttpOnly Cookie，且响应体不含令牌本身。
func TestCreateSessionSetsHttpOnlyCookie(t *testing.T) {
	initSessionTest(t)
	t.Setenv("ADMIN_SECRET", "session-test-secret")
	t.Setenv("GIN_MODE", "test")
	t.Setenv("SEASAGI_ENV", "")

	r := gin.New()
	r.POST("/admin/session", CreateSession)
	req := httptest.NewRequest(http.MethodPost, "/admin/session",
		strings.NewReader(`{"secret":"session-test-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), sessionCookieName) {
		t.Fatalf("response body exposes session material: %s", w.Body.String())
	}

	var cookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("session cookie was not set")
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("SameSite = %v, want Strict", cookie.SameSite)
	}
	if cookie.Value == "" || cookie.MaxAge <= 0 {
		t.Fatalf("invalid session cookie: value empty=%v maxAge=%d", cookie.Value == "", cookie.MaxAge)
	}
}

// 错误密钥不得签发会话；登录限流生效。
func TestCreateSessionRejectsWrongSecret(t *testing.T) {
	initSessionTest(t)
	t.Setenv("ADMIN_SECRET", "session-test-secret")

	r := gin.New()
	r.POST("/admin/session", CreateSession)
	req := httptest.NewRequest(http.MethodPost, "/admin/session",
		strings.NewReader(`{"secret":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func loginSession(t *testing.T) *http.Cookie {
	t.Helper()
	t.Setenv("ADMIN_SECRET", "session-test-secret")
	t.Setenv("GIN_MODE", "test")
	r := gin.New()
	r.POST("/admin/session", CreateSession)
	req := httptest.NewRequest(http.MethodPost, "/admin/session",
		strings.NewReader(`{"secret":"session-test-secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookieName {
			return c
		}
	}
	t.Fatal("no session cookie")
	return nil
}

// 会话 Cookie 可独立通过 Middleware；无凭证、注销后必须拒绝；Bearer 回退仍兼容 CLI。
func TestMiddlewareAcceptsSessionCookieAndRevokesOnLogout(t *testing.T) {
	initSessionTest(t)
	cookie := loginSession(t)

	r := gin.New()
	r.GET("/admin/users", Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	r.DELETE("/admin/session", DeleteSession)
	r.GET("/admin/session", Middleware(), GetSession)

	call := func(method, path string, withCookie bool, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if withCookie {
			req.AddCookie(cookie)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := call(http.MethodGet, "/admin/users", false, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", w.Code)
	}
	if w := call(http.MethodGet, "/admin/users", true, ""); w.Code != http.StatusOK {
		t.Fatalf("session cookie = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/admin/session", true, ""); w.Code != http.StatusOK {
		t.Fatalf("GetSession = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := call(http.MethodGet, "/admin/users", false, "session-test-secret"); w.Code != http.StatusOK {
		t.Fatalf("Bearer fallback = %d, want 200", w.Code)
	}
	if w := call(http.MethodDelete, "/admin/session", true, ""); w.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", w.Code)
	}
	if w := call(http.MethodGet, "/admin/users", true, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session = %d, want 401", w.Code)
	}
}
