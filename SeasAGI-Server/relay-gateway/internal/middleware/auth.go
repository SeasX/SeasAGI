package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/auth"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/i18n"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// AdminAuth 保护运维端点（/ops/*）。使用 ADMIN_SECRET 作为 Bearer 令牌，
// 未配置时一律拒绝，禁止匿名读取上游渠道与运维状态。
func AdminAuth() gin.HandlerFunc {
	adminSecret := strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
	if adminSecret == "" {
		logging.Warningf("ADMIN_SECRET is not set — ops API is disabled")
	}
	return func(c *gin.Context) {
		if adminSecret == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "admin secret is not configured"})
			c.Abort()
			return
		}
		token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(adminSecret)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid admin secret"})
			c.Abort()
			return
		}
		c.Set("admin_id", c.GetHeader("X-Admin-Id"))
		c.Next()
	}
}

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidToken")})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidToken")})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("channel_id", claims.ChannelID)
		c.Set("device_id", claims.DeviceID)

		tenantID := claims.TenantID
		if tenantID == "" {
			tenantID = c.GetHeader("X-Tenant-Id")
		}
		if tenantID == "" {
			tenantID = "default"
		}
		c.Set("tenant_id", tenantID)

		c.Next()
	}
}

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
		}
		c.Set("trace_id", traceID)
		c.Writer.Header().Set("X-Trace-Id", traceID)

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		userID := c.GetString("user_id")
		deviceID := c.GetString("device_id")
		channelID := c.GetString("channel_id")
		model := c.GetString("request_model")
		if model == "" {
			model = c.Query("model")
		}

		logging.Infof("trace_id=%s method=%s path=%s status=%d latency_ms=%d user_id=%s device_id=%s channel_id=%s model=%s client_ip=%s",
			traceID,
			c.Request.Method,
			c.Request.URL.Path,
			c.Writer.Status(),
			duration.Milliseconds(),
			userID,
			deviceID,
			channelID,
			model,
			c.ClientIP(),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return gin.Recovery()
}
