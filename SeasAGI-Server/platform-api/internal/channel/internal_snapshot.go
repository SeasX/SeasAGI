package channel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/gin-gonic/gin"
)

// ChannelSnapshot 是旧版 relay 策略缓存使用的最小通道视图（不含任何凭证），
// 为兼容尚未升级的 relay 保留。
type ChannelSnapshot struct {
	ChannelID    string   `json:"channel_id"`
	ProviderType string   `json:"provider_type"`
	BaseURL      string   `json:"base_url"`
	Enabled      bool     `json:"enabled"`
	Models       []string `json:"models"`
	SortOrder    int      `json:"sort_order"`
}

// InternalChannelSnapshot 供旧版 relay-gateway 拉取通道快照。
// 只能挂在 /internal 组（已由 usage.InternalAuth 校验服务间令牌）下。
func (h *Handler) InternalChannelSnapshot(c *gin.Context) {
	channels, err := h.repo.List(c.Request.Context())
	if err != nil {
		// 不回显数据库错误，避免内部路径与 schema 细节外泄。
		logging.Errorf("internal channel snapshot: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load channel snapshot"})
		return
	}

	out := make([]ChannelSnapshot, 0, len(channels))
	for _, ch := range channels {
		out = append(out, ChannelSnapshot{
			ChannelID:    ch.ChannelID,
			ProviderType: ch.ProviderType,
			BaseURL:      ch.BaseURL,
			Enabled:      ch.Enabled,
			Models:       modelIDs(ch.Models),
			SortOrder:    ch.SortOrder,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// RelayChannelConfig 是下发给 relay-gateway 的完整渠道配置。
//
// api_key 是解密后的上游凭证：该结构只允许出现在 InternalAuth 保护的
// /api/v1/internal/relay/config 响应中，不得用于任何面向用户或管理端的接口，也不得写日志。
type RelayChannelConfig struct {
	ChannelID       string   `json:"channel_id"`
	ProviderType    string   `json:"provider_type"`
	DisplayName     string   `json:"display_name"`
	BaseURL         string   `json:"base_url"`
	APIKey          string   `json:"api_key"`
	Enabled         bool     `json:"enabled"`
	Models          []string `json:"models"`
	Weight          int      `json:"weight"`
	Priority        int      `json:"priority"`
	GrayPercent     int      `json:"gray_percent"`
	ReadOnly        bool     `json:"read_only"`
	CredentialError string   `json:"credential_error,omitempty"`
}

// RelayConfig 是 relay 配置同步的响应体。Version 是快照指纹，同时作为 ETag。
type RelayConfig struct {
	SchemaVersion int                  `json:"schema_version"`
	Version       string               `json:"version"`
	GeneratedAt   string               `json:"generated_at"`
	Channels      []RelayChannelConfig `json:"channels"`
}

// RelayConfigSchemaVersion 是同步协议版本；relay 拒绝不认识的版本。
const RelayConfigSchemaVersion = 1

// credentialUnavailable 是凭证无法解密时下发的错误类别（不含任何密钥材料）。
const credentialUnavailable = "credential_unavailable"

// BuildRelayConfig 生成当前的 relay 配置快照。
// version 基于凭证「密文」计算，相同配置得到相同版本，且版本值不由明文凭证推导。
func (h *Handler) BuildRelayConfig(c *gin.Context) (*RelayConfig, error) {
	channels, strategyEnabled, err := h.repo.listWithStrategy(c.Request.Context())
	if err != nil {
		return nil, err
	}

	out := make([]RelayChannelConfig, 0, len(channels))
	digest := sha256.New()
	for i, ch := range channels {
		cfg := RelayChannelConfig{
			ChannelID:    ch.ChannelID,
			ProviderType: ch.ProviderType,
			DisplayName:  ch.DisplayName,
			BaseURL:      ch.BaseURL,
			Enabled:      ch.Enabled && strategyEnabled[i],
			Models:       modelIDs(ch.Models),
			Weight:       ch.Weight,
			Priority:     ch.Priority,
			GrayPercent:  ch.GrayPercent,
			ReadOnly:     ch.ReadOnly,
		}
		if ch.EncryptedAPIKey != "" {
			plain, err := h.decrypt(ch.EncryptedAPIKey)
			if err != nil {
				// 下发错误类别而不是跳过渠道：relay 会保留该渠道但不参与路由，便于排查。
				logging.Errorf("relay config: channel %s credential cannot be decrypted", ch.ChannelID)
				cfg.CredentialError = credentialUnavailable
			} else {
				cfg.APIKey = plain
			}
		}
		out = append(out, cfg)

		hashed := cfg
		hashed.APIKey = ch.EncryptedAPIKey
		line, _ := json.Marshal(hashed)
		digest.Write(line)
		digest.Write([]byte{'\n'})
	}

	return &RelayConfig{
		SchemaVersion: RelayConfigSchemaVersion,
		Version:       hex.EncodeToString(digest.Sum(nil)),
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Channels:      out,
	}, nil
}

// InternalRelayConfig 下发 relay 所需的完整渠道配置（含解密后的凭证）。
// 必须挂在 /internal 组下；支持 If-None-Match，未变化时返回 304 不下发凭证。
func (h *Handler) InternalRelayConfig(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	cfg, err := h.BuildRelayConfig(c)
	if err != nil {
		logging.Errorf("internal relay config: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to build relay config"})
		return
	}
	etag := `"` + cfg.Version + `"`
	c.Header("ETag", etag)
	if match := strings.TrimSpace(c.GetHeader("If-None-Match")); match != "" && (match == etag || match == cfg.Version) {
		c.Status(http.StatusNotModified)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

func modelIDs(models []ModelInfo) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ModelID)
	}
	return ids
}
