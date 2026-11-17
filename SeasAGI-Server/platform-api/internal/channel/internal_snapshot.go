package channel

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ChannelSnapshot 是 relay-gateway 策略缓存所需的最小通道视图。
// 只包含路由与模型选择必需字段，任何凭证字段都不得进入服务间响应。
type ChannelSnapshot struct {
	ChannelID    string   `json:"channel_id"`
	ProviderType string   `json:"provider_type"`
	BaseURL      string   `json:"base_url"`
	Enabled      bool     `json:"enabled"`
	Models       []string `json:"models"`
	SortOrder    int      `json:"sort_order"`
}

// InternalChannelSnapshot 供 relay-gateway 拉取通道快照。
// 只能挂在 /internal 组（已由 usage.InternalAuth 校验服务间令牌）下：
// 此前网关对需要用户 JWT 的 /api/v1/channels 发匿名 http.Get，恒 401，
// 控制面与数据面实际断开。
func InternalChannelSnapshot(c *gin.Context) {
	channels, err := fetchAllChannels()
	if err != nil {
		// 不回显数据库错误，避免内部路径与 schema 细节外泄。
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load channel snapshot"})
		return
	}

	out := make([]ChannelSnapshot, 0, len(channels))
	for _, ch := range channels {
		models := make([]string, 0, len(ch.Models))
		for _, m := range ch.Models {
			models = append(models, m.ModelID)
		}
		out = append(out, ChannelSnapshot{
			ChannelID:    ch.ChannelID,
			ProviderType: ch.ProviderType,
			BaseURL:      ch.BaseURL,
			Enabled:      ch.Enabled,
			Models:       models,
			SortOrder:    ch.SortOrder,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
