package combo

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
)

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

type CandidateProvider struct {
	ChannelID    string `json:"channel_id"`
	Model        string `json:"model"`
	Priority     int    `json:"priority"`
	HealthStatus string `json:"health_status,omitempty"`
}

type ComboStep struct {
	ChannelID             string              `json:"channel_id,omitempty"`
	Model                 string              `json:"model"`
	StepRole              string              `json:"step_role,omitempty"`
	Providers             []CandidateProvider `json:"providers,omitempty"`
	Channels              []string            `json:"channels,omitempty"`
	SelectionPolicy       string              `json:"selection_policy,omitempty"`
	AllowProviderFallback bool                `json:"allow_provider_fallback,omitempty"`
}

type ModelCombo struct {
	ComboID         string         `json:"combo_id"`
	Scope           string         `json:"scope"`
	OwnerID         string         `json:"owner_id"`
	TenantID        string         `json:"tenant_id"`
	LogicalName     string         `json:"logical_name"`
	DisplayName     string         `json:"display_name"`
	Description     string         `json:"description"`
	Tags            string         `json:"tags"`
	Strategy        string         `json:"strategy"`
	StickyUses      int            `json:"sticky_uses"`
	QuickStrategy   string         `json:"quick_strategy,omitempty"`
	TaskProfileJSON string         `json:"-"`
	TaskProfile     map[string]any `json:"task_profile,omitempty"`
	StepsJSON       string         `json:"-"`
	Steps           []ComboStep    `json:"steps,omitempty"`
	Status          string         `json:"status"`
	Source          string         `json:"source"`
	Version         int            `json:"version"`
	CreatedBy       string         `json:"created_by"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

type CreateComboRequest struct {
	LogicalName   string         `json:"logical_name"`
	DisplayName   string         `json:"display_name"`
	Description   string         `json:"description,omitempty"`
	Tags          []string       `json:"tags,omitempty"`
	Strategy      string         `json:"strategy,omitempty"`
	StickyUses    int            `json:"sticky_uses,omitempty"`
	QuickStrategy string         `json:"quick_strategy,omitempty"`
	TaskProfile   map[string]any `json:"task_profile,omitempty"`
	Steps         []ComboStep    `json:"steps"`
	Status        string         `json:"status,omitempty"`
	Source        string         `json:"source,omitempty"`
}

type UpdateComboRequest struct {
	DisplayName   *string         `json:"display_name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	Tags          *[]string       `json:"tags,omitempty"`
	Strategy      *string         `json:"strategy,omitempty"`
	StickyUses    *int            `json:"sticky_uses,omitempty"`
	QuickStrategy *string         `json:"quick_strategy,omitempty"`
	TaskProfile   *map[string]any `json:"task_profile,omitempty"`
	Steps         *[]ComboStep    `json:"steps,omitempty"`
	Status        *string         `json:"status,omitempty"`
}

func scanCombo(scanner interface {
	Scan(dest ...interface{}) error
}) (ModelCombo, error) {
	var c ModelCombo
	err := scanner.Scan(
		&c.ComboID, &c.Scope, &c.OwnerID, &c.TenantID,
		&c.LogicalName, &c.DisplayName, &c.Description,
		&c.Tags, &c.Strategy, &c.StickyUses, &c.QuickStrategy, &c.TaskProfileJSON,
		&c.StepsJSON, &c.Status, &c.Source, &c.Version,
		&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func parseSteps(c *ModelCombo) {
	if c.StepsJSON != "" {
		_ = json.Unmarshal([]byte(c.StepsJSON), &c.Steps)
	}
	if c.TaskProfileJSON != "" {
		_ = json.Unmarshal([]byte(c.TaskProfileJSON), &c.TaskProfile)
	}
}

// ListUserCombos returns all combos owned by the current user
func ListUserCombos(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": i18n.TFromContext(c, "combo.requireUserId")})
		return
	}

	rows, err := database.DB.Query(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos
		 WHERE owner_id = ? AND scope = 'user'
		 ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	combos := make([]ModelCombo, 0)
	for rows.Next() {
		mc, err := scanCombo(rows)
		if err != nil {
			continue
		}
		parseSteps(&mc)
		combos = append(combos, mc)
	}

	c.JSON(http.StatusOK, gin.H{"data": combos})
}

// GetUserCombo returns a single combo by ID (owner must match)
func GetUserCombo(c *gin.Context) {
	userID := c.GetString("user_id")
	comboID := c.Param("id")

	var mc ModelCombo
	mc, err := scanCombo(database.DB.QueryRow(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos WHERE combo_id = ? AND owner_id = ?`, comboID, userID,
	))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "combo.notFound")})
		return
	}
	parseSteps(&mc)
	c.JSON(http.StatusOK, gin.H{"data": mc})
}

// CreateUserCombo creates a new user-level combo
func CreateUserCombo(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateComboRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.LogicalName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logical_name is required"})
		return
	}
	if len(req.Steps) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one step is required"})
		return
	}

	comboID := generateID()
	now := time.Now().UTC().Format(time.RFC3339)

	stepsBytes, _ := json.Marshal(req.Steps)
	tagsBytes, _ := json.Marshal(req.Tags)
	taskProfileBytes, _ := json.Marshal(req.TaskProfile)
	if req.Tags == nil {
		tagsBytes = []byte("[]")
	}
	if req.TaskProfile == nil {
		taskProfileBytes = []byte("{}")
	}

	strategy := req.Strategy
	if strategy == "" {
		strategy = "fallback"
	}
	status := req.Status
	if status == "" {
		status = "draft"
	}
	source := req.Source
	if source == "" {
		source = "local"
	}

	_, err := database.DB.Exec(
		`INSERT INTO model_combos (combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		 tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version, created_by, created_at, updated_at)
		 VALUES (?, 'user', ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
		comboID, userID, req.LogicalName, req.DisplayName, req.Description,
		string(tagsBytes), strategy, req.StickyUses, req.QuickStrategy, string(taskProfileBytes), string(stepsBytes),
		status, source, userID, now, now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	mc := ModelCombo{
		ComboID:       comboID,
		Scope:         "user",
		OwnerID:       userID,
		LogicalName:   req.LogicalName,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		Tags:          string(tagsBytes),
		Strategy:      strategy,
		StickyUses:    req.StickyUses,
		QuickStrategy: req.QuickStrategy,
		TaskProfile:   req.TaskProfile,
		Steps:         req.Steps,
		Status:        status,
		Source:        source,
		Version:       1,
		CreatedBy:     userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	c.JSON(http.StatusCreated, gin.H{"data": mc})
}

// UpdateUserCombo updates an existing user combo
func UpdateUserCombo(c *gin.Context) {
	userID := c.GetString("user_id")
	comboID := c.Param("id")

	var existing ModelCombo
	existing, err := scanCombo(database.DB.QueryRow(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos WHERE combo_id = ? AND owner_id = ?`, comboID, userID,
	))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "combo.notFound")})
		return
	}

	var req UpdateComboRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	newVersion := existing.Version + 1

	displayName := existing.DisplayName
	if req.DisplayName != nil {
		displayName = *req.DisplayName
	}
	description := existing.Description
	if req.Description != nil {
		description = *req.Description
	}
	tags := existing.Tags
	if req.Tags != nil {
		b, _ := json.Marshal(req.Tags)
		tags = string(b)
	}
	strategy := existing.Strategy
	if req.Strategy != nil {
		strategy = *req.Strategy
	}
	stickyUses := existing.StickyUses
	if req.StickyUses != nil {
		stickyUses = *req.StickyUses
	}
	quickStrategy := existing.QuickStrategy
	if req.QuickStrategy != nil {
		quickStrategy = *req.QuickStrategy
	}
	taskProfileJSON := existing.TaskProfileJSON
	if req.TaskProfile != nil {
		b, _ := json.Marshal(req.TaskProfile)
		taskProfileJSON = string(b)
	}
	stepsJSON := existing.StepsJSON
	if req.Steps != nil {
		b, _ := json.Marshal(req.Steps)
		stepsJSON = string(b)
	}
	status := existing.Status
	if req.Status != nil {
		status = *req.Status
	}

	_, err = database.DB.Exec(
		`UPDATE model_combos SET display_name=?, description=?, tags=?, strategy=?, sticky_uses=?,
		 quick_strategy=?, task_profile=?, steps_json=?, status=?, version=?, updated_at=? WHERE combo_id=?`,
		displayName, description, tags, strategy, stickyUses,
		quickStrategy, taskProfileJSON, stepsJSON, status, newVersion, now, comboID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"combo_id":       comboID,
			"version":        newVersion,
			"display_name":   displayName,
			"description":    description,
			"status":         status,
			"strategy":       strategy,
			"quick_strategy": quickStrategy,
			"updated_at":     now,
		},
	})
}

// DeleteUserCombo soft-deletes (archives) a user combo
func DeleteUserCombo(c *gin.Context) {
	userID := c.GetString("user_id")
	comboID := c.Param("id")

	result, err := database.DB.Exec(
		`UPDATE model_combos SET status='archived', updated_at=CURRENT_TIMESTAMP WHERE combo_id=? AND owner_id=?`,
		comboID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "combo.notFound")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "combo archived"})
}

// ListUserCombosByAdmin returns all combos for a specific user (admin endpoint)
func ListUserCombosByAdmin(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.TFromContext(c, "combo.requireUserId")})
		return
	}

	rows, err := database.DB.Query(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos
		 WHERE owner_id = ?
		 ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	combos := make([]ModelCombo, 0)
	for rows.Next() {
		mc, err := scanCombo(rows)
		if err != nil {
			continue
		}
		parseSteps(&mc)
		combos = append(combos, mc)
	}

	c.JSON(http.StatusOK, gin.H{"data": combos})
}

// ListTenantCombos lists combos within a tenant scope (platform/official + tenant templates)
func ListTenantCombos(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	scope := c.Query("scope")

	query := `SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
	          tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
	          created_by, created_at, updated_at
	          FROM model_combos WHERE status = 'active'`

	args := make([]interface{}, 0)

	if scope == "platform" || scope == "" {
		query += ` AND (scope = 'platform' OR (scope = 'tenant' AND tenant_id = ?))`
		args = append(args, tenantID)
	} else if scope == "platform" {
		query += ` AND scope = 'platform'`
	} else if scope == "tenant" {
		query += ` AND scope = 'tenant' AND tenant_id = ?`
		args = append(args, tenantID)
	}

	query += ` ORDER BY logical_name ASC`

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	combos := make([]ModelCombo, 0)
	for rows.Next() {
		mc, err := scanCombo(rows)
		if err != nil {
			continue
		}
		parseSteps(&mc)
		combos = append(combos, mc)
	}

	c.JSON(http.StatusOK, gin.H{"data": combos})
}

// ListOfficialTemplates returns platform-scope active combos as templates
func ListOfficialTemplates(c *gin.Context) {
	rows, err := database.DB.Query(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, quick_strategy, task_profile, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos
		 WHERE scope = 'platform' AND status = 'active'
		 ORDER BY logical_name ASC`,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	combos := make([]ModelCombo, 0)
	for rows.Next() {
		mc, err := scanCombo(rows)
		if err != nil {
			continue
		}
		parseSteps(&mc)
		combos = append(combos, mc)
	}

	c.JSON(http.StatusOK, gin.H{"data": combos})
}

// ─── Release Management ─────────────────────────────────────────────

type ComboRelease struct {
	ReleaseID     string `json:"release_id"`
	ComboID       string `json:"combo_id"`
	Version       int    `json:"version"`
	SnapshotJSON  string `json:"snapshot_json"`
	PublishedBy   string `json:"published_by"`
	PublishedAt   string `json:"published_at"`
	ChangeSummary string `json:"change_summary"`
}

// PublishComboRelease snapshot current state and creates a release record
func PublishComboRelease(c *gin.Context) {
	comboID := c.Param("combo_id")
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "admin"
	}

	var existing ModelCombo
	existing, err := scanCombo(database.DB.QueryRow(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos WHERE combo_id = ?`, comboID,
	))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "combo.notFound")})
		return
	}
	parseSteps(&existing)

	snapshot := map[string]any{
		"combo_id":     existing.ComboID,
		"logical_name": existing.LogicalName,
		"display_name": existing.DisplayName,
		"description":  existing.Description,
		"tags":         existing.Tags,
		"strategy":     existing.Strategy,
		"sticky_uses":  existing.StickyUses,
		"steps":        existing.Steps,
		"status":       existing.Status,
	}

	var req struct {
		ChangeSummary string `json:"change_summary"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.ChangeSummary = ""
	}

	snapshotBytes, _ := json.Marshal(snapshot)
	releaseID := generateID()
	now := time.Now().UTC().Format(time.RFC3339)
	newVersion := existing.Version + 1

	_, err = database.DB.Exec(
		`INSERT INTO model_combo_releases (release_id, combo_id, version, snapshot_json, published_by, published_at, change_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		releaseID, comboID, newVersion, string(snapshotBytes), userID, now, req.ChangeSummary,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	_, _ = database.DB.Exec(
		`UPDATE model_combos SET version = ?, updated_at = ? WHERE combo_id = ?`,
		newVersion, now, comboID,
	)

	c.JSON(http.StatusCreated, gin.H{
		"data": ComboRelease{
			ReleaseID:     releaseID,
			ComboID:       comboID,
			Version:       newVersion,
			SnapshotJSON:  string(snapshotBytes),
			PublishedBy:   userID,
			PublishedAt:   now,
			ChangeSummary: req.ChangeSummary,
		},
	})
}

// ListComboReleases returns all releases for a combo
func ListComboReleases(c *gin.Context) {
	comboID := c.Param("combo_id")

	rows, err := database.DB.Query(
		`SELECT release_id, combo_id, version, snapshot_json, published_by, published_at, change_summary
		 FROM model_combo_releases WHERE combo_id = ? ORDER BY version DESC`, comboID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	releases := make([]ComboRelease, 0)
	for rows.Next() {
		var r ComboRelease
		if err := rows.Scan(&r.ReleaseID, &r.ComboID, &r.Version, &r.SnapshotJSON,
			&r.PublishedBy, &r.PublishedAt, &r.ChangeSummary); err != nil {
			continue
		}
		releases = append(releases, r)
	}

	c.JSON(http.StatusOK, gin.H{"data": releases})
}

// RollbackComboRelease restores combo to a previous release version
func RollbackComboRelease(c *gin.Context) {
	comboID := c.Param("combo_id")
	releaseID := c.Param("release_id")

	var release ComboRelease
	err := database.DB.QueryRow(
		`SELECT release_id, combo_id, version, snapshot_json, published_by, published_at, change_summary
		 FROM model_combo_releases WHERE release_id = ? AND combo_id = ?`, releaseID, comboID,
	).Scan(&release.ReleaseID, &release.ComboID, &release.Version, &release.SnapshotJSON,
		&release.PublishedBy, &release.PublishedAt, &release.ChangeSummary)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "release not found"})
		return
	}

	var snapshot map[string]any
	if err := json.Unmarshal([]byte(release.SnapshotJSON), &snapshot); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse release snapshot"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	displayName, _ := snapshot["display_name"].(string)
	description, _ := snapshot["description"].(string)
	tags, _ := snapshot["tags"].(string)
	strategy, _ := snapshot["strategy"].(string)
	stickyUses, _ := snapshot["sticky_uses"].(float64)
	status, _ := snapshot["status"].(string)

	stepsJSON := "[]"
	if stepsRaw, ok := snapshot["steps"]; ok {
		if b, err := json.Marshal(stepsRaw); err == nil {
			stepsJSON = string(b)
		}
	}

	_, err = database.DB.Exec(
		`UPDATE model_combos SET display_name=?, description=?, tags=?, strategy=?, sticky_uses=?,
		 steps_json=?, status=?, version=?, updated_at=? WHERE combo_id=?`,
		displayName, description, tags, strategy, int(stickyUses),
		stepsJSON, status, release.Version, now, comboID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rollback successful",
		"data": gin.H{
			"combo_id": comboID,
			"version":  release.Version,
		},
	})
}

// DeployComboToTenant publishes a combo to specific tenants
func DeployComboToTenant(c *gin.Context) {
	comboID := c.Param("combo_id")
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "admin"
	}

	var req struct {
		TenantIDs []string `json:"tenant_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.TenantIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one tenant_id is required"})
		return
	}

	var sourceCombo ModelCombo
	sourceCombo, err := scanCombo(database.DB.QueryRow(
		`SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
		        tags, strategy, sticky_uses, steps_json, status, source, version,
		        created_by, created_at, updated_at
		 FROM model_combos WHERE combo_id = ?`, comboID,
	))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source combo not found"})
		return
	}
	parseSteps(&sourceCombo)

	now := time.Now().UTC().Format(time.RFC3339)
	deployed := make([]string, 0)
	for _, tenantID := range req.TenantIDs {
		newID := generateID()

		var existingID string
		err := database.DB.QueryRow(
			`SELECT combo_id FROM model_combos WHERE scope = 'tenant' AND tenant_id = ? AND logical_name = ?`,
			tenantID, sourceCombo.LogicalName,
		).Scan(&existingID)

		if err == nil {
			_, _ = database.DB.Exec(
				`UPDATE model_combos SET display_name=?, description=?, tags=?, strategy=?, sticky_uses=?,
				 steps_json=?, version=version+1, updated_at=? WHERE combo_id=?`,
				sourceCombo.DisplayName, sourceCombo.Description, sourceCombo.Tags,
				sourceCombo.Strategy, sourceCombo.StickyUses, sourceCombo.StepsJSON,
				now, existingID,
			)
			deployed = append(deployed, existingID)
		} else {
			_, err = database.DB.Exec(
				`INSERT INTO model_combos (combo_id, scope, owner_id, tenant_id, logical_name, display_name,
				 description, tags, strategy, sticky_uses, steps_json, status, source, version, created_by, created_at, updated_at)
				 VALUES (?, 'tenant', ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 'official', 1, ?, ?, ?)`,
				newID, userID, tenantID, sourceCombo.LogicalName, sourceCombo.DisplayName,
				sourceCombo.Description, sourceCombo.Tags, sourceCombo.Strategy,
				sourceCombo.StickyUses, sourceCombo.StepsJSON, userID, now, now,
			)
			if err != nil {
				continue
			}
			deployed = append(deployed, newID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            fmt.Sprintf("deployed to %d tenants", len(deployed)),
		"deployed_combo_ids": deployed,
	})
}

// ListAllCombosAdmin returns all combos across scopes with optional filters
func ListAllCombosAdmin(c *gin.Context) {
	scope := c.Query("scope")
	s := c.Query("status")

	query := `SELECT combo_id, scope, owner_id, tenant_id, logical_name, display_name, description,
	          tags, strategy, sticky_uses, steps_json, status, source, version,
	          created_by, created_at, updated_at
	          FROM model_combos WHERE 1=1`
	args := make([]interface{}, 0)

	if scope != "" {
		query += ` AND scope = ?`
		args = append(args, scope)
	}
	if s != "" {
		query += ` AND status = ?`
		args = append(args, s)
	}

	query += ` ORDER BY updated_at DESC`

	rows, err := database.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	combos := make([]ModelCombo, 0)
	for rows.Next() {
		mc, err := scanCombo(rows)
		if err != nil {
			continue
		}
		parseSteps(&mc)
		combos = append(combos, mc)
	}

	c.JSON(http.StatusOK, gin.H{"data": combos})
}
