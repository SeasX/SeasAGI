package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

func newTokenVersionDB(t *testing.T) *sql.DB {
	t.Helper()
	for _, driver := range []string{"sqlite3", "sqlite"} {
		db, err := sql.Open(driver, ":memory:")
		if err != nil {
			continue
		}
		if _, err := db.Exec(`
			CREATE TABLE users (
				user_id TEXT PRIMARY KEY,
				tenant_id TEXT NOT NULL DEFAULT 'default',
				email TEXT UNIQUE NOT NULL,
				hashed_password TEXT NOT NULL,
				token_version INTEGER NOT NULL DEFAULT 0,
				updated_at DATETIME
			);
			INSERT INTO users (user_id, tenant_id, email, hashed_password)
			VALUES ('u-revoke', 'tenant-a', 'revoke@example.com', 'hash');
		`); err == nil {
			return db
		}
		db.Close()
	}
	t.Skip("no sqlite driver available for tests")
	return nil
}

func useTokenVersionDB(t *testing.T) *sql.DB {
	t.Helper()
	db := newTokenVersionDB(t)
	prev := database.DB
	database.DB = db
	t.Cleanup(func() {
		database.DB = prev
		db.Close()
	})
	return db
}

func probeAuth(token string) int {
	r := gin.New()
	r.GET("/probe", Middleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// 注销后 token_version 递增，旧 access token 立即失效（P1-9）。
func TestLogoutRevokesAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "token-version-secret")
	t.Setenv("JWT_REFRESH_SECRET", "token-version-refresh-secret")
	useTokenVersionDB(t)

	tokenStr, err := generateAccessToken("u-revoke", "revoke@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}
	if code := probeAuth(tokenStr); code != http.StatusOK {
		t.Fatalf("fresh token status = %d, want 200", code)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u-revoke")
	Logout(c)
	if w.Code != http.StatusOK {
		t.Fatalf("logout failed: %d %s", w.Code, w.Body.String())
	}

	if code := probeAuth(tokenStr); code != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", code)
	}
}

// 注销同样使旧 refresh token 失效，且新签发的令牌携带当前版本。
func TestRefreshTokenRejectsRevokedVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "token-version-secret")
	t.Setenv("JWT_REFRESH_SECRET", "token-version-refresh-secret")
	useTokenVersionDB(t)

	refresh, err := generateRefreshToken("u-revoke")
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u-revoke")
	Logout(c)

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh",
		strings.NewReader(`{"refresh_token":"`+refresh+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	RefreshToken(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked refresh token status = %d, want 401: %s", w.Code, w.Body.String())
	}

	// 重新登录后签发的令牌版本与数据库一致，可以正常使用。
	database.DB.Exec(`UPDATE users SET token_version = 0 WHERE user_id = 'u-revoke'`)
	newToken, err := generateAccessToken("u-revoke", "revoke@example.com")
	if err != nil {
		t.Fatalf("regenerate access token: %v", err)
	}
	if code := probeAuth(newToken); code != http.StatusOK {
		t.Fatalf("new token status = %d, want 200", code)
	}
}
