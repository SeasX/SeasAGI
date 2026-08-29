package user

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
)

type OptimizationConfig struct {
	Mode               string `json:"mode"`
	PenaltyEnabled     bool   `json:"penalty_enabled"`
	PenaltyDecaySec    int    `json:"penalty_decay_sec"`
	HealthCheckEnabled bool   `json:"health_check_enabled"`
	HealthCheckSec     int    `json:"health_check_sec"`
	HealthMaxFailures  int    `json:"health_max_failures"`
	CooldownEnabled    bool   `json:"cooldown_enabled"`
	CooldownSec        int    `json:"cooldown_sec"`
	StickyEnabled      bool   `json:"sticky_enabled"`
	StickyTTLSec       int    `json:"sticky_ttl_sec"`
	PresetEnabled      bool   `json:"preset_enabled"`
	DefaultPreset      string `json:"default_preset"`
}

func GetOptimizationConfig(c *gin.Context) {
	userID := c.GetString("user_id")

	var raw string
	err := database.DB.QueryRow(
		`SELECT optimization_config FROM users WHERE user_id = ?`,
		userID,
	).Scan(&raw)
	if err != nil || raw == "" {
		c.JSON(http.StatusOK, map[string]any{"config": nil})
		return
	}

	var cfg OptimizationConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		c.JSON(http.StatusOK, map[string]any{"config": nil})
		return
	}

	c.JSON(http.StatusOK, map[string]any{"config": cfg})
}

func SetOptimizationConfig(c *gin.Context) {
	userID := c.GetString("user_id")

	var body struct {
		Config OptimizationConfig `json:"config"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	raw, err := json.Marshal(body.Config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "marshal failed"})
		return
	}

	_, err = database.DB.Exec(
		`UPDATE users SET optimization_config = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?`,
		string(raw), userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
