package usage

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
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

func IngestUsage(c *gin.Context) {
	var events []UsageEvent
	if err := c.ShouldBindJSON(&events); err != nil {
		var single UsageEvent
		if err2 := c.ShouldBindJSON(&single); err2 != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		events = []UsageEvent{single}
	}

	count := 0
	for _, e := range events {
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

func GetUserUsage(c *gin.Context) {
	userID := c.GetString("user_id")
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

	var summary UsageSummary
	summary.Period = period
	err := database.DB.QueryRow(
		`SELECT COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0), COALESCE(SUM(is_error),0)
		 FROM usage_records WHERE user_id = ? AND recorded_at >= ?`,
		userID, periodStart.Format(time.RFC3339),
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

	rows, err := database.DB.Query(
		`SELECT model, capability, COALESCE(SUM(request_count),0), COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0)
		 FROM usage_records WHERE user_id = ? AND recorded_at >= ?
		 GROUP BY model, capability ORDER BY SUM(request_count) DESC`,
		userID, periodStart.Format(time.RFC3339),
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

	rows, err := database.DB.Query(
		`SELECT COALESCE(error_category, '') AS category, COUNT(*) AS cnt
		 FROM usage_records
		 WHERE user_id = ? AND is_error = 1 AND recorded_at >= ?
		 GROUP BY error_category
		 ORDER BY cnt DESC`,
		userID, periodStart.Format(time.RFC3339),
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

	now := time.Now()
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

	rows, err := database.DB.Query(
		`SELECT `+timeExpr+` AS bucket,
			COUNT(*) AS total_count,
			SUM(is_error) AS error_count
		 FROM usage_records
		 WHERE user_id = ? AND recorded_at >= ?
		 GROUP BY bucket
		 ORDER BY bucket ASC`,
		userID, periodStart.Format(time.RFC3339),
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
