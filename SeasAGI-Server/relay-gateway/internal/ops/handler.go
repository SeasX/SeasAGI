package ops

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/i18n"
	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/trace"
)

func GetOverview(c *gin.Context) {
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	chs := st.ListChannels()
	healthyCount := 0
	unhealthyCount := 0
	for _, ch := range chs {
		if ch.Health.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"channels": gin.H{
			"total":     len(chs),
			"healthy":   healthyCount,
			"unhealthy": unhealthyCount,
		},
	})
}

func ListChannelHealth(c *gin.Context) {
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	chs := st.ListChannels()
	items := make([]gin.H, 0, len(chs))
	for _, ch := range chs {
		items = append(items, gin.H{
			"channel_id":   ch.ChannelID,
			"display_name": ch.DisplayName,
			"enabled":      ch.Enabled,
			"healthy":      ch.Health.Healthy,
			"status_code":  ch.Health.StatusCode,
			"consec_fail":  ch.Health.ConsecFail,
			"error":        ch.Health.Error,
			"read_only":    ch.ReadOnly,
			"weight":       ch.Weight,
			"gray_percent": ch.GrayPercent,
		})
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}

func GetTraceByID(c *gin.Context) {
	traceID := c.Param("trace_id")
	s := trace.GetStore()
	if s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "trace.storeNotInitialized")})
		return
	}
	rec, err := s.QueryByTraceID(traceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "trace.notFound")})
		return
	}
	c.JSON(http.StatusOK, rec)
}

func QueryTraces(c *gin.Context) {
	s := trace.GetStore()
	if s == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "trace.storeNotInitialized")})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if userID := c.Query("user_id"); userID != "" {
		records, err := s.QueryByUserID(userID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": records})
		return
	}
	if deviceID := c.Query("device_id"); deviceID != "" {
		records, err := s.QueryByDeviceID(deviceID, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": records})
		return
	}
	records, err := s.QueryRecent(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": records})
}

func DrainChannel(c *gin.Context) {
	channelID := c.Param("channel_id")
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	ch, ok := st.GetChannel(channelID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	ch.Enabled = false
	st.UpdateChannel(*ch)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "channel.drained"), "channel_id": channelID})
}

func RestoreChannel(c *gin.Context) {
	channelID := c.Param("channel_id")
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	ch, ok := st.GetChannel(channelID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	ch.Enabled = true
	st.UpdateChannel(*ch)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "channel.restored"), "channel_id": channelID})
}

func SetChannelReadOnly(c *gin.Context) {
	channelID := c.Param("channel_id")
	var req struct {
		ReadOnly bool `json:"read_only"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.TFromContext(c, "server.badRequest")})
		return
	}
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	ch, ok := st.GetChannel(channelID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	ch.ReadOnly = req.ReadOnly
	st.UpdateChannel(*ch)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "channel.readOnlyUpdated"), "channel_id": channelID, "read_only": req.ReadOnly})
}

func SetChannelWeight(c *gin.Context) {
	channelID := c.Param("channel_id")
	var req struct {
		Weight int `json:"weight"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.TFromContext(c, "server.badRequest")})
		return
	}
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	ch, ok := st.GetChannel(channelID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	ch.Weight = req.Weight
	st.UpdateChannel(*ch)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "channel.weightUpdated"), "channel_id": channelID, "weight": req.Weight})
}

func SetChannelGray(c *gin.Context) {
	channelID := c.Param("channel_id")
	var req struct {
		GrayPercent int `json:"gray_percent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.TFromContext(c, "server.badRequest")})
		return
	}
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "channel.storeNotInitialized")})
		return
	}
	ch, ok := st.GetChannel(channelID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "channel.notFound")})
		return
	}
	ch.GrayPercent = req.GrayPercent
	st.UpdateChannel(*ch)
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "channel.grayUpdated"), "channel_id": channelID, "gray_percent": req.GrayPercent})
}

func GetAlertPanel(c *gin.Context) {
	st := channel.GetGlobalStore()
	if st == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "channel store not initialized"})
		return
	}
	chs := st.ListChannels()
	alerts := make([]gin.H, 0)
	for _, ch := range chs {
		if !ch.Enabled {
			continue
		}
		if !ch.Health.Healthy {
			alerts = append(alerts, gin.H{
				"channel_id":  ch.ChannelID,
				"severity":    "critical",
				"type":        "unhealthy",
				"consec_fail": ch.Health.ConsecFail,
				"error":       ch.Health.Error,
			})
		}
		if ch.ReadOnly {
			alerts = append(alerts, gin.H{
				"channel_id": ch.ChannelID,
				"severity":   "warning",
				"type":       "read_only",
				"message":    i18n.TFromContext(c, "channel.readOnlyMode"),
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"alert_count": len(alerts),
		"alerts":      alerts,
	})
}
