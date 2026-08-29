package relay

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
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

func ListRelayGateways(c *gin.Context) {
	items, err := listRelayGateways(true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}

func AdminCreateRelayGateway(c *gin.Context) {
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

	_, err := database.DB.Exec(
		`INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled)
		 VALUES (?, ?, ?, ?, ?, 1)`,
		req.GatewayID, req.Name, req.Host, req.Port, req.Region,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": i18n.TFromContext(c, "relay.gatewayCreated")})
}

func AdminUpdateRelayGateway(c *gin.Context) {
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

	var existing RelayGateway
	err := database.DB.QueryRow(
		`SELECT gateway_id, name, host, port, region, enabled, created_at FROM relay_gateways WHERE gateway_id = ?`, gwID,
	).Scan(&existing.GatewayID, &existing.Name, &existing.Host, &existing.Port, &existing.Region, &existing.Enabled, &existing.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "relay.gatewayNotFound")})
		return
	}

	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	host := existing.Host
	if req.Host != nil {
		host = *req.Host
	}
	port := existing.Port
	if req.Port != nil {
		port = *req.Port
	}
	region := existing.Region
	if req.Region != nil {
		region = *req.Region
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	_, err = database.DB.Exec(
		`UPDATE relay_gateways SET name=?, host=?, port=?, region=?, enabled=? WHERE gateway_id=?`,
		name, host, port, region, enabled, gwID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": i18n.TFromContext(c, "relay.gatewayUpdated")})
}

func AdminDeleteRelayGateway(c *gin.Context) {
	gwID := c.Param("id")
	_, err := database.DB.Exec(`DELETE FROM relay_gateways WHERE gateway_id = ?`, gwID)
	if err != nil {
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

func AdminListRelayGateways(c *gin.Context) {
	items, err := listRelayGateways(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": items})
}

func listRelayGateways(enabledOnly bool) ([]RelayGateway, error) {
	query := `SELECT gateway_id, name, host, port, region, enabled, created_at
		 FROM relay_gateways`
	if enabledOnly {
		query += ` WHERE enabled = 1 AND TRIM(host) <> '' AND port > 0`
	}
	query += ` ORDER BY region, name`

	rows, err := database.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelayGateway, 0)
	for rows.Next() {
		var g RelayGateway
		if err := rows.Scan(&g.GatewayID, &g.Name, &g.Host, &g.Port, &g.Region, &g.Enabled, &g.CreatedAt); err == nil {
			items = append(items, g)
		}
	}
	return items, rows.Err()
}
