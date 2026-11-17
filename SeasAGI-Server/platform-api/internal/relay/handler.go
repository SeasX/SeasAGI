package relay

import (
	"errors"
	"net/http"

	"github.com/SeasAGI/SeasAGI-Server/shared/i18n"
	"github.com/gin-gonic/gin"
)

type RelayGateway struct {
	GatewayID string `json:"gateway_id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Region    string `json:"region"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// Handler 提供中继网关接口；数据访问经注入的 Repository 完成。
type Handler struct {
	repo *Repository
}

// NewHandler 创建中继网关处理器。
func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// ListRelayGateways 返回已启用且地址有效的中继网关（登录用户可见）。
func (h *Handler) ListRelayGateways(c *gin.Context) {
	items, err := h.repo.List(c.Request.Context(), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}

func (h *Handler) AdminCreateRelayGateway(c *gin.Context) {
	var req struct {
		GatewayID string `json:"gateway_id" binding:"required"`
		Name      string `json:"name" binding:"required"`
		Host      string `json:"host" binding:"required"`
		Port      int    `json:"port"`
		Region    string `json:"region"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 8318
	}

	if err := h.repo.Create(c.Request.Context(), RelayGateway{
		GatewayID: req.GatewayID, Name: req.Name, Host: req.Host, Port: req.Port, Region: req.Region,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": i18n.TFromContext(c, "relay.gatewayCreated")})
}

func (h *Handler) AdminUpdateRelayGateway(c *gin.Context) {
	gwID := c.Param("id")
	var req struct {
		Name    *string `json:"name"`
		Host    *string `json:"host"`
		Port    *int    `json:"port"`
		Region  *string `json:"region"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing, err := h.repo.Get(c.Request.Context(), gwID)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "relay.gatewayNotFound")})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updated := *existing
	if req.Name != nil {
		updated.Name = *req.Name
	}
	if req.Host != nil {
		updated.Host = *req.Host
	}
	if req.Port != nil {
		updated.Port = *req.Port
	}
	if req.Region != nil {
		updated.Region = *req.Region
	}
	if req.Enabled != nil {
		updated.Enabled = *req.Enabled
	}

	if err := h.repo.Update(c.Request.Context(), updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "relay.gatewayUpdated")})
}

func (h *Handler) AdminDeleteRelayGateway(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "relay.gatewayDeleted")})
}

type AdminRelayGatewayRecord struct {
	GatewayID string `json:"gateway_id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Region    string `json:"region"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

// AdminListRelayGateways 返回全部中继网关（含停用与地址无效的记录）。
func (h *Handler) AdminListRelayGateways(c *gin.Context) {
	items, err := h.repo.List(c.Request.Context(), false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}
