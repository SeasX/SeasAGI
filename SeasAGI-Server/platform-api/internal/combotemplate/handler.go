package combotemplate

import (
	"encoding/json"
	"net/http"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
	"github.com/gin-gonic/gin"
)

type Step struct {
	ChannelID string `json:"channel_id,omitempty"`
	Model     string `json:"model"`
}

type OfficialTemplate struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Steps       []Step   `json:"steps"`
	Models      []string `json:"models"`
	Strategy    string   `json:"strategy"`
	StickyUses  int      `json:"sticky_uses"`
	Source      string   `json:"source,omitempty"`
}

// ListOfficialTemplates returns platform-level combo templates.
// Priority: model_combos (scope=platform) > built-in default templates.
func ListOfficialTemplates(c *gin.Context) {
	// Try reading from model_combos main table first
	platformCombos, err := queryPlatformCombos()
	if err == nil && len(platformCombos) > 0 {
		c.JSON(http.StatusOK, gin.H{"object": "list", "data": platformCombos})
		return
	}

	// Fallback: built-in templates using default models.
	lang := i18n.GetLangFromRequest(c)
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   buildOfficialTemplates(nil, lang),
	})
}

// queryPlatformCombos reads platform-scope combos from the model_combos table.
func queryPlatformCombos() ([]OfficialTemplate, error) {
	rows, err := database.DB.Query(
		`SELECT logical_name, display_name, description, tags, strategy, sticky_uses, steps_json
		 FROM model_combos
		 WHERE scope = 'platform' AND status = 'active'
		 ORDER BY logical_name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []OfficialTemplate
	for rows.Next() {
		var logicalName, displayName, description, tagsStr, strategy, stepsJSON string
		var stickyUses int
		if err := rows.Scan(&logicalName, &displayName, &description, &tagsStr, &strategy, &stickyUses, &stepsJSON); err != nil {
			continue
		}

		var steps []Step
		if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
			continue
		}

		var tags []string
		if err := json.Unmarshal([]byte(tagsStr), &tags); err != nil {
			tags = nil
		}

		models := make([]string, 0, len(steps))
		for _, s := range steps {
			if s.Model != "" {
				models = append(models, s.Model)
			}
		}

		templates = append(templates, OfficialTemplate{
			Name:        logicalName,
			Description: description,
			Tags:        tags,
			Steps:       steps,
			Models:      models,
			Strategy:    strategy,
			StickyUses:  stickyUses,
			Source:      "platform",
		})
	}

	if len(templates) == 0 {
		return nil, nil
	}
	return templates, nil
}

func buildOfficialTemplates(models []string, lang string) []OfficialTemplate {
	uniqueModels := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		if _, ok := seen[model]; ok || model == "" {
			continue
		}
		seen[model] = struct{}{}
		uniqueModels = append(uniqueModels, model)
	}
	if len(uniqueModels) == 0 {
		uniqueModels = []string{"gpt-4o-mini"}
	}

	primary := uniqueModels[0]
	fallback := primary
	if len(uniqueModels) > 1 {
		fallback = uniqueModels[1]
	}
	cheapest := uniqueModels[len(uniqueModels)-1]

	templates := []OfficialTemplate{
		{
			Name:        "official-balanced-fallback",
			Description: i18n.T(lang, "combotemplate.defaultFallback"),
			Tags:        splitTags(lang, "combotemplate.defaultFallbackTags"),
			Steps: []Step{
				{Model: primary},
				{Model: fallback},
			},
			Models:     dedupeModels([]string{primary, fallback}),
			Strategy:   "fallback",
			StickyUses: 1,
			Source:     "fallback_dynamic",
		},
		{
			Name:        "official-efficient-first",
			Description: i18n.T(lang, "combotemplate.defaultCostEffective"),
			Tags:        splitTags(lang, "combotemplate.defaultCostEffectiveTags"),
			Steps: []Step{
				{Model: cheapest},
				{Model: primary},
			},
			Models:     dedupeModels([]string{cheapest, primary}),
			Strategy:   "fallback",
			StickyUses: 1,
			Source:     "fallback_dynamic",
		},
		{
			Name:        "official-burst-round-robin",
			Description: i18n.T(lang, "combotemplate.defaultPolling"),
			Tags:        splitTags(lang, "combotemplate.defaultPollingTags"),
			Steps: []Step{
				{Model: primary},
			},
			Models:     []string{primary},
			Strategy:   "round_robin",
			StickyUses: 3,
			Source:     "fallback_dynamic",
		},
	}

	return templates
}

func splitTags(lang, key string) []string {
	value := i18n.T(lang, key)
	if value == "" || value == key {
		return []string{"General"}
	}
	var tags []string
	_ = json.Unmarshal([]byte("["+value+"]"), &tags)
	return tags
}

func dedupeModels(models []string) []string {
	result := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		if _, ok := seen[model]; ok || model == "" {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result
}
