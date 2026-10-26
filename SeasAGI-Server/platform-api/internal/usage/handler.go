package usage

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/timeutil"
	"github.com/gin-gonic/gin"
)

type UsageEvent struct {
	UserID          string  `json:"user_id" binding:"required"`
	DeviceID        string  `json:"device_id"`
	ChannelID       string  `json:"channel_id" binding:"required"`
	ChannelType     string  `json:"channel_type"`
	IsHighCostModel bool    `json:"is_high_cost_model"`
	Model           string  `json:"model" binding:"required"`
	Capability      string  `json:"capability"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	LatencyMs       int     `json:"latency_ms"`
	StatusCode      int     `json:"status_code"`
	CostUSD         float64 `json:"cost_usd"`
	TotalCostUSD    float64 `json:"total_cost_usd"`
	ErrorCategory   string  `json:"error_category"`
}

type UsageSummary struct {
	Period         string `json:"period"`
	TotalRequests  int    `json:"total_requests"`
	TotalInputTok  int    `json:"total_input_tokens"`
	TotalOutputTok int    `json:"total_output_tokens"`
	TotalTokens    int    `json:"total_tokens"`
	TotalErrors    int    `json:"total_errors"`
}

type ModelUsage struct {
	Model          string `json:"model"`
	Capability     string `json:"capability"`
	TotalRequests  int    `json:"total_requests"`
	TotalInputTok  int    `json:"total_input_tokens"`
	TotalOutputTok int    `json:"total_output_tokens"`
}

// InternalAuth 保护内部用量上报入口。
// 令牌优先读取 INTERNAL_API_TOKEN，未配置时回退 ADMIN_SECRET；
// 两者皆未配置则返回 503（fail-closed），并使用常量时间比较。
func InternalAuth() gin.HandlerFunc {
	token := strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("ADMIN_SECRET"))
	}
	return func(c *gin.Context) {
		if token == "" {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "internal api token is not configured"})
			c.Abort()
			return
		}
		provided := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if provided == "" {
			provided = c.GetHeader("X-Internal-Token")
		}
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid internal api token"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func IngestUsage(c *gin.Context) {
	// 先一次性读取请求体：此前连续两次 ShouldBindJSON 会因 body 被消费而失败。
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	var events []UsageEvent
	if err := json.Unmarshal(body, &events); err != nil {
		var single UsageEvent
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		events = []UsageEvent{single}
	}

	count := 0
	for _, e := range events {
		if e.UserID == "" || e.ChannelID == "" || e.Model == "" {
			continue
		}
		capability := e.Capability
		if capability == "" {
			capability = "chat"
		}
		isError := 0
		if e.StatusCode >= 400 {
			isError = 1
		}
		errCategory := e.ErrorCategory
		if isError == 0 {
			errCategory = ""
		}

		_, err := database.DB.Exec(
			`INSERT INTO usage_records (user_id, device_id, channel_id, model, capability, input_tokens, output_tokens, request_count, latency_ms, status_code, is_error, error_category, recorded_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			e.UserID, e.DeviceID, e.ChannelID, e.Model, capability, e.InputTokens, e.OutputTokens, e.LatencyMs, e.StatusCode, isError, errCategory,
		)
		if err == nil {
			count++
		}
	}

	c.JSON(http.StatusOK, gin.H{"ingested": count})
}

// periodStartUTC 返回统计周期起点（UTC）。
// usage_records.recorded_at 由 CURRENT_TIMESTAMP 写入（UTC），
// 这里统一用 UTC 计算起点，避免本地时区/格式不一致导致当天统计恒 0、周月漏首日。
func periodStartUTC(now time.Time, period string) time.Time {
	now = now.UTC()
	switch period {
	case "day":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, time.UTC)
	default:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}

// sqliteTimestamp 统一时间参数格式，与 SQLite CURRENT_TIMESTAMP 保持一致。
func sqliteTimestamp(t time.Time) string {
	return timeutil.FormatSQLite(t)
}

func GetUserUsage(c *gin.Context) {
	userID := c.GetString("user_id")
	period := c.DefaultQuery("period", "month")

	periodStart := periodStartUTC(time.Now(), period)

	var summary UsageSummary
	summary.Period = period
	err := database.Reader().QueryRow(
		`SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(is_error),0)
		 FROM usage_records WHERE user_id = ? AND datetime(recorded_at) >= datetime(?)`,
		userID, sqliteTimestamp(periodStart),
	).Scan(&summary.TotalRequests, &summary.TotalInputTok, &summary.TotalOutputTok, &summary.TotalErrors)
	if err != nil {
		summary = UsageSummary{Period: period}
	}
	summary.TotalTokens = summary.TotalInputTok + summary.TotalOutputTok

	c.JSON(http.StatusOK, summary)
}

func GetUserModelUsage(c *gin.Context) {
	userID := c.GetString("user_id")
	period := c.DefaultQuery("period", "month")

	periodStart := periodStartUTC(time.Now(), period)

	rows, err := database.Reader().Query(
		`SELECT model, capability, COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
		 FROM usage_records WHERE user_id = ? AND datetime(recorded_at) >= datetime(?)
		 GROUP BY model, capability ORDER BY SUM(request_count) DESC`,
		userID, sqliteTimestamp(periodStart),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var models []ModelUsage
	for rows.Next() {
		var m ModelUsage
		if err := rows.Scan(&m.Model, &m.Capability, &m.TotalRequests, &m.TotalInputTok, &m.TotalOutputTok); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		models = append(models, m)
	}
	if models == nil {
		models = []ModelUsage{}
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
}

type ErrorDistribution struct {
	Category string `json:"category"`
	Count    int    `json:"count"`
}

func GetErrorDistribution(c *gin.Context) {
	userID := c.GetString("user_id")
	period := c.DefaultQuery("period", "month")

	periodStart := periodStartUTC(time.Now(), period)

	rows, err := database.Reader().Query(
		`SELECT COALESCE(error_category, '') AS category, COUNT(*) AS cnt
		 FROM usage_records
		 WHERE user_id = ? AND is_error = 1 AND datetime(recorded_at) >= datetime(?)
		 GROUP BY error_category
		 ORDER BY cnt DESC`,
		userID, sqliteTimestamp(periodStart),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var distributions []ErrorDistribution
	for rows.Next() {
		var d ErrorDistribution
		if err := rows.Scan(&d.Category, &d.Count); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		distributions = append(distributions, d)
	}
	if distributions == nil {
		distributions = []ErrorDistribution{}
	}

	c.JSON(http.StatusOK, gin.H{"object": "list", "data": distributions})
}

type TimelinePoint struct {
	Bucket      string  `json:"bucket"`
	TotalCount  int     `json:"total_count"`
	ErrorCount  int     `json:"error_count"`
	SuccessRate float64 `json:"success_rate"`
}

func GetUsageTimeline(c *gin.Context) {
	userID := c.GetString("user_id")
	period := c.DefaultQuery("period", "24h")
	interval := c.DefaultQuery("interval", "hour")

	// 时间线使用相对窗口（24h/7d/30d），统一按 UTC 计算并与 SQLite 时间格式对齐。
	now := time.Now().UTC()
	var periodStart time.Time
	switch period {
	case "7d":
		periodStart = now.AddDate(0, 0, -7)
	case "30d":
		periodStart = now.AddDate(0, 0, -30)
	default:
		periodStart = now.Add(-24 * time.Hour)
	}

	var timeExpr string
	switch interval {
	case "day":
		timeExpr = "strftime('%Y-%m-%d', recorded_at)"
	default:
		timeExpr = "strftime('%Y-%m-%dT%H:00:00', recorded_at)"
	}

	rows, err := database.Reader().Query(
		`SELECT `+timeExpr+` AS bucket,
			COUNT(*) AS total_count,
			SUM(is_error) AS error_count
		 FROM usage_records
		 WHERE user_id = ? AND datetime(recorded_at) >= datetime(?)
		 GROUP BY bucket
		 ORDER BY bucket ASC`,
		userID, sqliteTimestamp(periodStart),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var points []TimelinePoint
	for rows.Next() {
		var p TimelinePoint
		if err := rows.Scan(&p.Bucket, &p.TotalCount, &p.ErrorCount); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if p.TotalCount > 0 {
			p.SuccessRate = float64(p.TotalCount-p.ErrorCount) / float64(p.TotalCount) * 100.0
		}
		points = append(points, p)
	}
	if points == nil {
		points = []TimelinePoint{}
	}

	c.JSON(http.StatusOK, gin.H{"object": "list", "data": points})
}
