package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

// cgo 构建注册 "sqlite3"，nocgo 构建注册 "sqlite"，与 oauth_test.go 保持一致的回退。
func newTenantBoundaryDB(t *testing.T) *sql.DB {
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
				hashed_password TEXT NOT NULL
			);
			INSERT INTO users (user_id, tenant_id, email, hashed_password)
			VALUES ('u-tenant', 'tenant-a', 'tenant@example.com', 'hash');
		`); err == nil {
			return db
		}
		db.Close()
	}
	t.Skip("no sqlite driver available for tests")
	return nil
}

// 服务端签发的 access token 必须携带数据库中的 tenant_id，
// 否则 relay-gateway 不共享数据库时无法验证租户身份。
func TestAccessTokenCarriesSignedTenantID(t *testing.T) {
	t.Setenv("JWT_SECRET", "tenant-boundary-secret")
	db := newTenantBoundaryDB(t)
	defer db.Close()
	prev := database.DB
	database.DB = db
	defer func() { database.DB = prev }()

	tokenStr, err := generateAccessToken("u-tenant", "tenant@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}
	claims, err := validateToken(tokenStr)
	if err != nil {
		t.Fatalf("validateToken: %v", err)
	}
	if claims.TenantID != "tenant-a" {
		t.Fatalf("access token tenant_id = %q, want tenant-a", claims.TenantID)
	}
}

// 伪造的 X-Tenant-Id 不能改变中间件解析出的租户身份。
// 两条路径分别覆盖：数据库可用（数据库为权威）与数据库不可用（签名声明兜底）。
func TestMiddlewareIgnoresSpoofedTenantHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("JWT_SECRET", "tenant-boundary-secret")

	cases := []struct {
		name   string
		withDB bool
	}{
		{name: "database wins", withDB: true},
		{name: "signed claim fallback", withDB: false},
	}
	const wantTenant = "tenant-a"

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTenantBoundaryDB(t)
			defer db.Close()
			prev := database.DB

			// 无论后续数据库是否可用，令牌都由服务端在持有数据库时签发。
			database.DB = db
			tokenStr, err := generateAccessToken("u-tenant", "tenant@example.com")
			if err != nil {
				database.DB = prev
				t.Fatalf("generateAccessToken: %v", err)
			}

			if tc.withDB {
				database.DB = db
			} else {
				database.DB = nil
			}
			defer func() { database.DB = prev }()

			var got string
			r := gin.New()
			r.GET("/probe", Middleware(), func(c *gin.Context) {
				got = c.GetString("tenant_id")
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			req.Header.Set("Authorization", "Bearer "+tokenStr)
			req.Header.Set("X-Tenant-Id", "tenant-evil")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
			}
			if got != wantTenant {
				t.Fatalf("tenant_id = %q, want %q (spoofed header must be ignored)", got, wantTenant)
			}
		})
	}
}
