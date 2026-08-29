package modelcatalog

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Catalog is the YAML representation of data/model-catalog.yaml.
type Catalog struct {
	Version string       `yaml:"version"`
	Models  []ModelEntry `yaml:"models"`
}

// ModelEntry describes a single model in the catalog.
type ModelEntry struct {
	Name                string            `yaml:"name"`
	DisplayName         string            `yaml:"display_name"`
	Description         string            `yaml:"description"`
	Category            string            `yaml:"category"`
	Family              string            `yaml:"family"`
	Provider            string            `yaml:"provider"`
	Modality            string            `yaml:"modality"`
	ContextWindow       int               `yaml:"context_window"`
	MaxOutputTokens     int               `yaml:"max_output_tokens"`
	InputPriceUSDPer1M  float64           `yaml:"input_price_usd_per_1m"`
	OutputPriceUSDPer1M float64           `yaml:"output_price_usd_per_1m"`
	InputModalities     []string          `yaml:"input_modalities"`
	OutputModalities    []string          `yaml:"output_modalities"`
	Capabilities        []string          `yaml:"capabilities"`
	SupportedParameters []string          `yaml:"supported_parameters"`
	Metadata            map[string]string `yaml:"metadata"`
}

const modelSelectColumns = `model_id, display_name, description, category, family, provider, modality, context_window, max_output_tokens, input_price_usd_per_1m, output_price_usd_per_1m, capabilities, input_modalities, output_modalities, supported_parameters, metadata, status, source, updated_at`

const upsertSQL = `INSERT OR REPLACE INTO model_catalog (
	model_id, display_name, description, category, family, provider, modality,
	context_window, max_output_tokens, input_price_usd_per_1m, output_price_usd_per_1m,
	capabilities, input_modalities, output_modalities, supported_parameters, metadata,
	status, source, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 'catalog', CURRENT_TIMESTAMP)`

type modelRow struct {
	ModelID             string
	DisplayName         string
	Description         string
	Category            string
	Family              string
	Provider            string
	Modality            string
	ContextWindow       int
	MaxOutputTokens     int
	InputPriceUSDPer1M  float64
	OutputPriceUSDPer1M float64
	Capabilities        string
	InputModalities     string
	OutputModalities    string
	SupportedParameters string
	Metadata            string
	Status              string
	Source              string
	UpdatedAt           string
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanModel(s scanner) (modelRow, error) {
	var r modelRow
	err := s.Scan(
		&r.ModelID, &r.DisplayName, &r.Description, &r.Category, &r.Family, &r.Provider, &r.Modality,
		&r.ContextWindow, &r.MaxOutputTokens, &r.InputPriceUSDPer1M, &r.OutputPriceUSDPer1M,
		&r.Capabilities, &r.InputModalities, &r.OutputModalities, &r.SupportedParameters, &r.Metadata,
		&r.Status, &r.Source, &r.UpdatedAt,
	)
	return r, err
}

func (r modelRow) toResponse() gin.H {
	var capabilities []string
	_ = json.Unmarshal([]byte(r.Capabilities), &capabilities)
	var inputModalities []string
	_ = json.Unmarshal([]byte(r.InputModalities), &inputModalities)
	var outputModalities []string
	_ = json.Unmarshal([]byte(r.OutputModalities), &outputModalities)
	var supportedParameters []string
	_ = json.Unmarshal([]byte(r.SupportedParameters), &supportedParameters)
	var metadata map[string]string
	_ = json.Unmarshal([]byte(r.Metadata), &metadata)
	return gin.H{
		"id":                      r.ModelID,
		"name":                    r.ModelID,
		"display_name":            r.DisplayName,
		"description":             r.Description,
		"category":                r.Category,
		"family":                  r.Family,
		"provider":                r.Provider,
		"modality":                r.Modality,
		"context_window":          r.ContextWindow,
		"max_output_tokens":       r.MaxOutputTokens,
		"input_price_usd_per_1m":  r.InputPriceUSDPer1M,
		"output_price_usd_per_1m": r.OutputPriceUSDPer1M,
		"capabilities":            capabilities,
		"input_modalities":        inputModalities,
		"output_modalities":       outputModalities,
		"supported_parameters":    supportedParameters,
		"metadata":                metadata,
		"status":                  r.Status,
		"source":                  r.Source,
		"updated_at":              r.UpdatedAt,
	}
}

// findCatalogFile searches for data/model-catalog.yaml in the current working
// directory and a few parent directories.
func findCatalogFile() (string, error) {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "data", "model-catalog.yaml"))
		dir := cwd
		for i := 0; i < 6; i++ {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
			candidates = append(candidates, filepath.Join(dir, "data", "model-catalog.yaml"))
		}
	}
	candidates = append(candidates, filepath.Join("data", "model-catalog.yaml"))
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("data/model-catalog.yaml not found in candidate paths")
}

func upsertModel(m ModelEntry) error {
	capabilitiesJSON, err := json.Marshal(m.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}
	inputModalitiesJSON, err := json.Marshal(m.InputModalities)
	if err != nil {
		return fmt.Errorf("marshal input_modalities: %w", err)
	}
	outputModalitiesJSON, err := json.Marshal(m.OutputModalities)
	if err != nil {
		return fmt.Errorf("marshal output_modalities: %w", err)
	}
	supportedParametersJSON, err := json.Marshal(m.SupportedParameters)
	if err != nil {
		return fmt.Errorf("marshal supported_parameters: %w", err)
	}
	metadataJSON, err := json.Marshal(m.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = database.DB.Exec(upsertSQL,
		m.Name, m.DisplayName, m.Description, m.Category, m.Family, m.Provider, m.Modality,
		m.ContextWindow, m.MaxOutputTokens, m.InputPriceUSDPer1M, m.OutputPriceUSDPer1M,
		string(capabilitiesJSON), string(inputModalitiesJSON), string(outputModalitiesJSON),
		string(supportedParametersJSON), string(metadataJSON),
	)
	return err
}

// LoadAndSyncCatalog locates, parses and upserts the model catalog into the database.
func LoadAndSyncCatalog() error {
	path, err := findCatalogFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read catalog file: %w", err)
	}
	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return fmt.Errorf("parse catalog yaml: %w", err)
	}
	synced := 0
	for _, m := range catalog.Models {
		if m.Name == "" {
			continue
		}
		if err := upsertModel(m); err != nil {
			logging.Errorf("modelcatalog: failed to upsert model %s: %v", m.Name, err)
			continue
		}
		synced++
	}
	logging.Infof("modelcatalog: synced %d/%d models from %s", synced, len(catalog.Models), path)
	return nil
}

// ListModels returns all models stored in the catalog.
func ListModels(c *gin.Context) {
	query := `SELECT ` + modelSelectColumns + ` FROM model_catalog ORDER BY model_id`
	rows, err := database.DB.Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query models"})
		return
	}
	defer rows.Close()
	models := make([]gin.H, 0)
	for rows.Next() {
		r, err := scanModel(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to scan model"})
			return
		}
		models = append(models, r.toResponse())
	}
	if err := rows.Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read models"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "list", "data": models})
}

// GetModel returns a single model by name.
func GetModel(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model name is required"})
		return
	}
	query := `SELECT ` + modelSelectColumns + ` FROM model_catalog WHERE model_id = ?`
	r, err := scanModel(database.DB.QueryRow(query, name))
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "model not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query model"})
		return
	}
	c.JSON(http.StatusOK, r.toResponse())
}

// SyncCatalog (admin) reloads and syncs the model catalog from disk.
func SyncCatalog(c *gin.Context) {
	if err := LoadAndSyncCatalog(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("sync failed: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"object": "sync", "status": "ok"})
}
