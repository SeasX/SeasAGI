package admin

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/gin-gonic/gin"
)

func Middleware() gin.HandlerFunc {
	adminSecret := os.Getenv("ADMIN_SECRET")
	if adminSecret == "" {
		logging.Fatal("ADMIN_SECRET environment variable is required. Set it in production!")
	}
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token != adminSecret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "auth.invalidAdminSecret")})
			c.Abort()
			return
		}

		c.Next()
	}
}

func ListUsers(c *gin.Context) {
	rows, err := database.DB.Query(
		`SELECT user_id, email, plan, created_at, updated_at FROM users ORDER BY created_at DESC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type UserInfo struct {
		UserID    string `json:"user_id"`
		Email     string `json:"email"`
		Plan      string `json:"plan"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var users []UserInfo
	for rows.Next() {
		var u UserInfo
		if err := rows.Scan(&u.UserID, &u.Email, &u.Plan, &u.CreatedAt, &u.UpdatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		users = append(users, u)
	}

	if users == nil {
		users = []UserInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"data": users})
}

func GetUser(c *gin.Context) {
	userID := c.Param("id")

	type UserInfo struct {
		UserID    string `json:"user_id"`
		Email     string `json:"email"`
		Plan      string `json:"plan"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}

	var u UserInfo
	err := database.DB.QueryRow(
		`SELECT user_id, email, plan, created_at, updated_at FROM users WHERE user_id = ?`,
		userID,
	).Scan(&u.UserID, &u.Email, &u.Plan, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "admin.userNotFound")})
		return
	}

	c.JSON(http.StatusOK, u)
}

func DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	result, err := database.DB.Exec("DELETE FROM users WHERE user_id = ?", userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "admin.userNotFound")})
		return
	}

	database.DB.Exec("DELETE FROM subscriptions WHERE user_id = ?", userID)
	database.DB.Exec("DELETE FROM entitlements WHERE user_id = ?", userID)
	database.DB.Exec("DELETE FROM usage_records WHERE user_id = ?", userID)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "admin.userDeleted")})
}

func SeedChannel(c *gin.Context) {
	var req struct {
		ChannelID    string `json:"channel_id" binding:"required"`
		ChannelType  string `json:"channel_type"`
		ProviderType string `json:"provider_type" binding:"required"`
		DisplayName  string `json:"display_name" binding:"required"`
		BaseURL      string `json:"base_url" binding:"required"`
		Enabled      *bool  `json:"enabled"`
		SortOrder    int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := 1
	if req.Enabled != nil && !*req.Enabled {
		enabled = 0
	}
	if req.ChannelType == "" {
		req.ChannelType = "platform"
	}

	_, err := database.DB.Exec(
		`INSERT OR REPLACE INTO channels (channel_id, channel_type, provider_type, display_name, base_url, enabled, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		req.ChannelID, req.ChannelType, req.ProviderType, req.DisplayName, req.BaseURL, enabled, req.SortOrder,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "channel seeded", "channel_id": req.ChannelID})
}

func SeedChannelModels(c *gin.Context) {
	channelID := c.Param("id")

	var req struct {
		Models []struct {
			ModelID    string `json:"model_id" binding:"required"`
			ModelName  string `json:"model_name" binding:"required"`
			Capability string `json:"capability"`
		} `json:"models" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	database.DB.Exec("DELETE FROM channel_models WHERE channel_id = ?", channelID)

	for _, m := range req.Models {
		capability := m.Capability
		if capability == "" {
			capability = "chat"
		}
		database.DB.Exec(
			`INSERT OR IGNORE INTO channel_models (channel_id, model_id, model_name, capability)
			 VALUES (?, ?, ?, ?)`,
			channelID, m.ModelID, m.ModelName, capability,
		)
	}

	c.JSON(http.StatusOK, gin.H{"message": "models seeded", "count": len(req.Models)})
}

func GetUsageStats(c *gin.Context) {
	userID := c.Query("user_id")
	period := c.DefaultQuery("period", "month")

	now := time.Now()
	var periodStart time.Time
	switch period {
	case "day":
		periodStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		periodStart = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
	default:
		periodStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	}

	var query string
	var args []interface{}

	if userID != "" {
		query = `SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(is_error),0), COUNT(DISTINCT user_id)
				 FROM usage_records WHERE user_id = ? AND recorded_at >= ?`
		args = []interface{}{userID, periodStart.Format(time.RFC3339)}
	} else {
		query = `SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(is_error),0), COUNT(DISTINCT user_id)
				 FROM usage_records WHERE recorded_at >= ?`
		args = []interface{}{periodStart.Format(time.RFC3339)}
	}

	var totalReqs, totalInputTok, totalOutputTok, totalErrors, distinctUsers int
	_ = database.DB.QueryRow(query, args...).Scan(&totalReqs, &totalInputTok, &totalOutputTok, &totalErrors, &distinctUsers)

	c.JSON(http.StatusOK, gin.H{
		"period":              period,
		"total_requests":      totalReqs,
		"total_input_tokens":  totalInputTok,
		"total_output_tokens": totalOutputTok,
		"total_tokens":        totalInputTok + totalOutputTok,
		"total_errors":        totalErrors,
		"distinct_users":      distinctUsers,
	})
}

func ListAllUsage(c *gin.Context) {
	limit := 100
	offset := 0

	rows, err := database.DB.Query(
		`SELECT user_id, device_id, channel_id, model, capability, input_tokens, output_tokens, request_count, latency_ms, status_code, is_error, recorded_at
		 FROM usage_records ORDER BY recorded_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type UsageRecord struct {
		UserID       string `json:"user_id"`
		DeviceID     string `json:"device_id"`
		ChannelID    string `json:"channel_id"`
		Model        string `json:"model"`
		Capability   string `json:"capability"`
		InputTokens  int    `json:"input_tokens"`
		OutputTokens int    `json:"output_tokens"`
		RequestCount int    `json:"request_count"`
		LatencyMs    int    `json:"latency_ms"`
		StatusCode   int    `json:"status_code"`
		IsError      int    `json:"is_error"`
		RecordedAt   string `json:"recorded_at"`
	}

	var records []UsageRecord
	for rows.Next() {
		var r UsageRecord
		if err := rows.Scan(&r.UserID, &r.DeviceID, &r.ChannelID, &r.Model, &r.Capability, &r.InputTokens, &r.OutputTokens, &r.RequestCount, &r.LatencyMs, &r.StatusCode, &r.IsError, &r.RecordedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		records = append(records, r)
	}
	if records == nil {
		records = []UsageRecord{}
	}
	c.JSON(http.StatusOK, gin.H{"data": records})
}

func ListUserCustomChannels(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}

	rows, err := database.DB.Query(
		`SELECT channel_id, user_id, tenant_id, provider_type, display_name, base_url, models, enabled, created_at, updated_at
		 FROM user_custom_channels WHERE user_id = ? ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	type CustomChannel struct {
		ChannelID    string `json:"channel_id"`
		UserID       string `json:"user_id"`
		TenantID     string `json:"tenant_id"`
		ProviderType string `json:"provider_type"`
		DisplayName  string `json:"display_name"`
		BaseURL      string `json:"base_url"`
		Models       string `json:"models"`
		Enabled      bool   `json:"enabled"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}
	items := make([]CustomChannel, 0)
	for rows.Next() {
		var ch CustomChannel
		var enabledInt int
		if rows.Scan(&ch.ChannelID, &ch.UserID, &ch.TenantID, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &ch.Models, &enabledInt, &ch.CreatedAt, &ch.UpdatedAt) == nil {
			ch.Enabled = enabledInt == 1
			items = append(items, ch)
		}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}
