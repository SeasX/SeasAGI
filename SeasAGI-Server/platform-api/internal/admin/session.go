package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/gin-gonic/gin"
)

const (
	// sessionCookieName 是管理后台会话 Cookie 名，HttpOnly，前端 JS 不可读取。
	sessionCookieName = "seasagi_admin_session"
	// adminSessionTTL 是短时会话有效期，过期后必须重新用 ADMIN_SECRET 登录。
	adminSessionTTL = 12 * time.Hour
	// 登录爆破防护：单 IP 每分钟最多 10 次尝试。
	sessionMaxAttemptsPerMinute = 10
)

var (
	sessionAttemptsMu sync.Mutex
	sessionAttempts   = map[string]*sessionAttempt{}
)

type sessionAttempt struct {
	count   int
	resetAt time.Time
}

// allowSessionAttempt 是进程内限流，替代企业版依赖的 auth.RateLimiter / audit 模块。
// 社区版是单进程自托管形态，限流不需跨实例共享；重启后计数清零不影响安全性。
func allowSessionAttempt(ip string) bool {
	sessionAttemptsMu.Lock()
	defer sessionAttemptsMu.Unlock()

	now := time.Now()
	if len(sessionAttempts) > 4096 {
		for k, a := range sessionAttempts {
			if now.After(a.resetAt) {
				delete(sessionAttempts, k)
			}
		}
	}
	a := sessionAttempts[ip]
	if a == nil || now.After(a.resetAt) {
		sessionAttempts[ip] = &sessionAttempt{count: 1, resetAt: now.Add(time.Minute)}
		return true
	}
	if a.count >= sessionMaxAttemptsPerMinute {
		return false
	}
	a.count++
	return true
}

// CreateSession 用 ADMIN_SECRET 换取一次短时会话。
// 会话令牌只写入 HttpOnly Cookie，前端不再把长期管理员密钥留在 sessionStorage。
func CreateSession(c *gin.Context) {
	adminSecret := strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
	if adminSecret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin secret is not configured"})
		return
	}
	if !allowSessionAttempt(c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts"})
		return
	}

	var req struct {
		Secret string `json:"secret"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(adminSecret)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin secret"})
		return
	}

	token, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}
	sessionID, err := randomToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}
	expiresAt := time.Now().UTC().Add(adminSessionTTL)
	// 顺手清理过期会话，避免表随登录次数无限增长（只在登录时触发，开销可忽略）。
	_, _ = database.DB.Exec(
		`DELETE FROM admin_sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339),
	)
	// 数据库只存令牌摘要，即使库文件泄露也无法直接重放会话。
	if _, err := database.DB.Exec(
		`INSERT INTO admin_sessions (session_id, token_hash, admin_id, expires_at) VALUES (?, ?, ?, ?)`,
		sessionID, hashToken(token), "admin", expiresAt.Format(time.RFC3339),
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookieName, token, int(adminSessionTTL.Seconds()), "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"expires_at": expiresAt.Format(time.RFC3339)}})
}

// GetSession 供前端挂载时确认登录态；无效或过期会话一律 401。
func GetSession(c *gin.Context) {
	token, err := c.Cookie(sessionCookieName)
	if err != nil || token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "admin authentication required"})
		return
	}
	adminID, ok := resolveSession(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "admin authentication required"})
		return
	}
	var expiresAt string
	if err := database.DB.QueryRow(
		`SELECT expires_at FROM admin_sessions WHERE token_hash = ?`, hashToken(token),
	).Scan(&expiresAt); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "admin authentication required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"admin_id": adminID, "expires_at": expiresAt}})
}

// DeleteSession 注销当前会话并清理 Cookie。
func DeleteSession(c *gin.Context) {
	if token, err := c.Cookie(sessionCookieName); err == nil && token != "" {
		_, _ = database.DB.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, hashToken(token))
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookieName, "", -1, "/", "", requestIsSecure(c), true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// resolveSession 校验会话令牌摘要与有效期，过期记录顺手清理。
func resolveSession(token string) (string, bool) {
	if token == "" || database.DB == nil {
		return "", false
	}
	var adminID, expiresAt string
	err := database.DB.QueryRow(
		`SELECT admin_id, expires_at FROM admin_sessions WHERE token_hash = ?`, hashToken(token),
	).Scan(&adminID, &expiresAt)
	if err != nil {
		return "", false
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expires) {
		_, _ = database.DB.Exec(`DELETE FROM admin_sessions WHERE token_hash = ?`, hashToken(token))
		return "", false
	}
	if adminID == "" {
		adminID = "admin"
	}
	return adminID, true
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// requestIsSecure 判断请求是否经 TLS 到达，用于 Cookie 的 Secure 标记。
// 生产模式一律强制 Secure，避免反向代理头缺失导致 Cookie 明文下发。
func requestIsSecure(c *gin.Context) bool {
	if os.Getenv("GIN_MODE") == "release" || os.Getenv("SEASAGI_ENV") == "production" {
		return true
	}
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
