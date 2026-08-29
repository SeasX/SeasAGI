package providerresource

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"crypto/rand"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/secure"
	"github.com/gin-gonic/gin"
)

// ProviderResource represents a single API key/region/environment under a channel.
type ProviderResource struct {
	ResourceID      string     `json:"resource_id"`
	ChannelID       string     `json:"channel_id"`
	ResourceName    string     `json:"resource_name"`
	APIKey          string     `json:"api_key,omitempty"`
	EncryptedAPIKey string     `json:"encrypted_api_key,omitempty"`
	Region          string     `json:"region"`
	Environment     string     `json:"environment"`
	Enabled         bool       `json:"enabled"`
	Weight          int        `json:"weight"`
	Priority        int        `json:"priority"`
	RateLimitRPM    int        `json:"rate_limit_rpm"`
	MaxConcurrency  int        `json:"max_concurrency"`
	Healthy         bool       `json:"healthy"`
	StatusCode      int        `json:"status_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	ConsecFailures  int        `json:"consec_failures,omitempty"`
	CooldownUntil   *time.Time `json:"cooldown_until,omitempty"`
	LastCheck       *time.Time `json:"last_check,omitempty"`
	CreatedAt       string     `json:"created_at,omitempty"`
	UpdatedAt       string     `json:"updated_at,omitempty"`
}

// CreateResourceRequest is the payload for creating a new resource.
type CreateResourceRequest struct {
	ResourceName   string `json:"resource_name"`
	APIKey         string `json:"api_key"`
	Region         string `json:"region"`
	Environment    string `json:"environment"`
	Weight         int    `json:"weight"`
	Priority       int    `json:"priority"`
	RateLimitRPM   int    `json:"rate_limit_rpm"`
	MaxConcurrency int    `json:"max_concurrency"`
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ListResources returns all resources for a channel.
func ListResources(c *gin.Context) {
	channelID := c.Param("id")
	rows, err := database.DB.Query(
		`SELECT resource_id, channel_id, resource_name, encrypted_api_key, region, environment,
			enabled, weight, priority, rate_limit_rpm, max_concurrency, healthy, status_code,
			error_message, consec_failures, cooldown_until, last_check, created_at, updated_at
		 FROM provider_resources WHERE channel_id = ? ORDER BY priority ASC, weight DESC`,
		channelID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	resources := make([]ProviderResource, 0)
	for rows.Next() {
		r, err := scanResourceRow(rows)
		if err != nil {
			continue
		}
		resources = append(resources, r)
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": resources})
}

// GetResource returns a single resource by ID.
func GetResource(c *gin.Context) {
	channelID := c.Param("id")
	resourceID := c.Param("resource_id")
	r, err := fetchResource(channelID, resourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}
	c.JSON(http.StatusOK, r)
}

// CreateResource creates a new resource under a channel.
func CreateResource(c *gin.Context) {
	channelID := c.Param("id")
	var req CreateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resourceID := generateID()
	encrypted := ""
	if req.APIKey != "" {
		var err error
		encrypted, err = secure.EncryptString(req.APIKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt api key"})
			return
		}
	}

	environment := req.Environment
	if environment == "" {
		environment = "production"
	}
	weight := req.Weight
	if weight == 0 {
		weight = 100
	}

	_, err := database.DB.Exec(
		`INSERT INTO provider_resources
		 (resource_id, channel_id, resource_name, encrypted_api_key, region, environment,
		  enabled, weight, priority, rate_limit_rpm, max_concurrency, healthy)
		 VALUES (?,?,?,?,?,?,1,?,?,?,0,1)`,
		resourceID, channelID, req.ResourceName, encrypted, req.Region, environment,
		weight, req.Priority, req.RateLimitRPM, req.MaxConcurrency,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	r, _ := fetchResource(channelID, resourceID)
	c.JSON(http.StatusCreated, r)
}

// UpdateResource updates an existing resource.
func UpdateResource(c *gin.Context) {
	channelID := c.Param("id")
	resourceID := c.Param("resource_id")
	existing, err := fetchResource(channelID, resourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}

	var req CreateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encrypted := existing.EncryptedAPIKey
	if req.APIKey != "" {
		encrypted, err = secure.EncryptString(req.APIKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt api key"})
			return
		}
	}

	environment := req.Environment
	if environment == "" {
		environment = existing.Environment
	}
	weight := req.Weight
	if weight == 0 {
		weight = existing.Weight
	}

	_, err = database.DB.Exec(
		`UPDATE provider_resources SET
		 resource_name=?, encrypted_api_key=?, region=?, environment=?, weight=?, priority=?,
		 rate_limit_rpm=?, max_concurrency=?, updated_at=CURRENT_TIMESTAMP
		 WHERE resource_id=? AND channel_id=?`,
		req.ResourceName, encrypted, req.Region, environment, weight, req.Priority,
		req.RateLimitRPM, req.MaxConcurrency, resourceID, channelID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	r, _ := fetchResource(channelID, resourceID)
	c.JSON(http.StatusOK, r)
}

// DeleteResource deletes a resource by ID.
func DeleteResource(c *gin.Context) {
	channelID := c.Param("id")
	resourceID := c.Param("resource_id")
	_, err := database.DB.Exec(
		`DELETE FROM provider_resources WHERE resource_id=? AND channel_id=?`,
		resourceID, channelID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

// ResolveResource picks the best available resource for a channel.
func ResolveResource(c *gin.Context) {
	channelID := c.Param("id")
	row := database.DB.QueryRow(
		`SELECT resource_id, channel_id, resource_name, encrypted_api_key, region, environment,
				enabled, weight, priority, rate_limit_rpm, max_concurrency, healthy, status_code,
				error_message, consec_failures, cooldown_until, last_check
		 FROM provider_resources
		 WHERE channel_id = ? AND enabled = 1 AND healthy = 1
		   AND (cooldown_until IS NULL OR cooldown_until <= datetime('now'))
		 ORDER BY priority ASC, weight DESC
		 LIMIT 1`,
		channelID,
	)

	var r ProviderResource
	var enabledInt, healthyInt int
	var cooldownNull, lastCheckNull sql.NullTime
	if err := row.Scan(
		&r.ResourceID, &r.ChannelID, &r.ResourceName, &r.EncryptedAPIKey, &r.Region, &r.Environment,
		&enabledInt, &r.Weight, &r.Priority, &r.RateLimitRPM, &r.MaxConcurrency,
		&healthyInt, &r.StatusCode, &r.ErrorMessage, &r.ConsecFailures,
		&cooldownNull, &lastCheckNull,
	); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no available resource"})
		return
	}
	r.Enabled = enabledInt == 1
	r.Healthy = healthyInt == 1
	if cooldownNull.Valid {
		r.CooldownUntil = &cooldownNull.Time
	}
	if lastCheckNull.Valid {
		r.LastCheck = &lastCheckNull.Time
	}

	// Decrypt API key for relay use
	if r.EncryptedAPIKey != "" {
		if plain, err := secure.DecryptString(r.EncryptedAPIKey); err == nil {
			r.APIKey = plain
		}
	}

	c.JSON(http.StatusOK, r)
}

// fetchResource retrieves a single resource from the database.
func fetchResource(channelID, resourceID string) (ProviderResource, error) {
	row := database.DB.QueryRow(
		`SELECT resource_id, channel_id, resource_name, encrypted_api_key, region, environment,
				enabled, weight, priority, rate_limit_rpm, max_concurrency, healthy, status_code,
				error_message, consec_failures, cooldown_until, last_check, created_at, updated_at
		 FROM provider_resources WHERE resource_id=? AND channel_id=?`,
		resourceID, channelID,
	)
	return scanResourceRowFull(row)
}

// scanResourceRow scans a resource from sql.Rows.
func scanResourceRow(rows *sql.Rows) (ProviderResource, error) {
	var r ProviderResource
	var enabledInt, healthyInt int
	var cooldownNull, lastCheckNull sql.NullTime
	err := rows.Scan(
		&r.ResourceID, &r.ChannelID, &r.ResourceName, &r.EncryptedAPIKey, &r.Region, &r.Environment,
		&enabledInt, &r.Weight, &r.Priority, &r.RateLimitRPM, &r.MaxConcurrency,
		&healthyInt, &r.StatusCode, &r.ErrorMessage, &r.ConsecFailures,
		&cooldownNull, &lastCheckNull, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return r, fmt.Errorf("scan failed: %w", err)
	}
	r.Enabled = enabledInt == 1
	r.Healthy = healthyInt == 1
	if cooldownNull.Valid {
		r.CooldownUntil = &cooldownNull.Time
	}
	if lastCheckNull.Valid {
		r.LastCheck = &lastCheckNull.Time
	}
	return r, nil
}

// scanResourceRowFull scans a resource from sql.Row (including created_at/updated_at).
func scanResourceRowFull(row *sql.Row) (ProviderResource, error) {
	var r ProviderResource
	var enabledInt, healthyInt int
	var cooldownNull, lastCheckNull sql.NullTime
	err := row.Scan(
		&r.ResourceID, &r.ChannelID, &r.ResourceName, &r.EncryptedAPIKey, &r.Region, &r.Environment,
		&enabledInt, &r.Weight, &r.Priority, &r.RateLimitRPM, &r.MaxConcurrency,
		&healthyInt, &r.StatusCode, &r.ErrorMessage, &r.ConsecFailures,
		&cooldownNull, &lastCheckNull, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return r, fmt.Errorf("resource not found")
	}
	r.Enabled = enabledInt == 1
	r.Healthy = healthyInt == 1
	if cooldownNull.Valid {
		r.CooldownUntil = &cooldownNull.Time
	}
	if lastCheckNull.Valid {
		r.LastCheck = &lastCheckNull.Time
	}
	return r, nil
}
