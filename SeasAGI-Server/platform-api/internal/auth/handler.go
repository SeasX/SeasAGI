package auth

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// 登录/注册限流：内存令牌桶（进程内），用于缓解在线爆破与弱口令注册。
var (
	loginLimiter    = newAttemptLimiter(10, time.Minute)
	registerLimiter = newAttemptLimiter(5, time.Hour)
)

// attemptLimiter 是基于固定窗口的简单限流器，按 key（IP 或账号）计数。
type attemptLimiter struct {
	mu      sync.Mutex
	entries map[string]*attemptEntry
	limit   int
	window  time.Duration
}

type attemptEntry struct {
	count   int
	resetAt time.Time
}

func newAttemptLimiter(limit int, window time.Duration) *attemptLimiter {
	return &attemptLimiter{entries: make(map[string]*attemptEntry), limit: limit, window: window}
}

// Allow 在窗口内累计一次访问，超过阈值返回 false。
func (l *attemptLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || now.After(entry.resetAt) {
		l.entries[key] = &attemptEntry{count: 1, resetAt: now.Add(l.window)}
		l.cleanupLocked(now)
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	return true
}

// cleanupLocked 在条目过多时清理过期项，避免内存无限增长。
func (l *attemptLimiter) cleanupLocked(now time.Time) {
	if len(l.entries) < 1024 {
		return
	}
	for k, e := range l.entries {
		if now.After(e.resetAt) {
			delete(l.entries, k)
		}
	}
}

// validatePasswordStrength 校验最小长度与复杂度（至少包含字母和数字）。
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return fmt.Errorf("password must be at most 128 characters")
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("password must contain both letters and digits")
	}
	return nil
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 同时按 IP 与账号限流，避免单一维度被绕过。
	if !loginLimiter.Allow("ip:"+c.ClientIP()) || !loginLimiter.Allow("account:"+strings.ToLower(req.Email)) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many login attempts, please try again later"})
		return
	}

	userID, hashedPassword, err := findUserByEmail(req.Email)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidCredentials")})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidCredentials")})
		return
	}

	accessToken, err := generateAccessToken(userID, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}

	refreshToken, err := generateRefreshToken(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}

	c.JSON(http.StatusOK, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    86400,
	})
}

func Register(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !registerLimiter.Allow("ip:" + c.ClientIP()) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many registration attempts, please try again later"})
		return
	}

	if err := validatePasswordStrength(req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "password hashing failed"})
		return
	}

	userID, err := createUser(req.Email, string(hashedPassword))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := generateAccessToken(userID, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}

	refreshToken, err := generateRefreshToken(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}

	c.JSON(http.StatusCreated, TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    86400,
	})
}

func RefreshToken(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// refresh token 必须使用 refresh 密钥强校验，避免密钥未配置时以空密钥通过校验。
	claims, err := validateTokenWithSecret(req.RefreshToken, getJWTRefreshSecret())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidToken")})
		return
	}

	accessToken, err := generateAccessToken(claims.UserID, claims.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": accessToken, "expires_in": 86400})
}

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidToken")})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := validateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidToken")})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("device_id", c.GetHeader("X-Device-Id"))

		var tenantID string
		err = database.DB.QueryRow(`SELECT tenant_id FROM users WHERE user_id = ?`, claims.UserID).Scan(&tenantID)
		if err == nil && tenantID != "" {
			c.Set("tenant_id", tenantID)
		}

		c.Next()
	}
}

func getJWTSecret() string {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		logging.Fatal("JWT_SECRET environment variable is required. Set it in production!")
	}
	return secret
}

func getJWTRefreshSecret() string {
	secret := os.Getenv("JWT_REFRESH_SECRET")
	if secret == "" {
		logging.Fatal("JWT_REFRESH_SECRET environment variable is required. Set it in production!")
	}
	return secret
}

func generateAccessToken(userID, email string) (string, error) {
	secret := getJWTSecret()
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func generateRefreshToken(userID string) (string, error) {
	secret := getJWTRefreshSecret()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func validateToken(tokenString string) (*Claims, error) {
	secret := getJWTSecret()
	return validateTokenWithSecret(tokenString, secret)
}

func validateTokenWithSecret(tokenString, secret string) (*Claims, error) {
	if secret == "" {
		return nil, fmt.Errorf("jwt secret is not configured")
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}

func findUserByEmail(email string) (string, string, error) {
	if database.DB != nil {
		var userID, hashedPassword string
		err := database.DB.QueryRow(
			"SELECT user_id, hashed_password FROM users WHERE email = ?",
			strings.ToLower(email),
		).Scan(&userID, &hashedPassword)
		if err != nil {
			return "", "", fmt.Errorf("user not found")
		}
		return userID, hashedPassword, nil
	}
	return "", "", fmt.Errorf("database not initialized")
}

func createUser(email, hashedPassword string) (string, error) {
	if database.DB == nil {
		return "", fmt.Errorf("database not initialized")
	}

	userID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	_, err := database.DB.Exec(
		"INSERT INTO users (user_id, email, hashed_password) VALUES (?, ?, ?)",
		userID, strings.ToLower(email), hashedPassword,
	)
	if err != nil {
		return "", fmt.Errorf("email already exists")
	}
	return userID, nil
}
