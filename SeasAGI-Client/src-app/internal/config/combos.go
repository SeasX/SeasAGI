package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

func (s *Service) ensureBuiltinTemplates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := make(map[string]struct{})
	for _, t := range s.config.ComboTemplates {
		existing[t.Name] = struct{}{}
	}

	builtins := []ModelCombo{
		{
			Name:        "gpt-4o-fallback",
			Description: "GPT-4o 主力 + 备用回退，适合生产环境",
			Tags:        []string{"生产", "高可用", "OpenAI"},
			Steps: []ModelComboStep{
				{Model: "gpt-4o"},
				{Model: "gpt-4o-mini"},
			},
			Models:     []string{"gpt-4o", "gpt-4o-mini"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
		{
			Name:        "claude-opus-4-8-fallback",
			Description: "Claude Opus 4.8 主力 + Sonnet 5 回退，企业级智能",
			Tags:        []string{"生产", "企业", "Anthropic", "前沿"},
			Steps: []ModelComboStep{
				{Model: "claude-opus-4-8"},
				{Model: "claude-sonnet-5"},
			},
			Models:     []string{"claude-opus-4-8", "claude-sonnet-5"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
		{
			Name:        "claude-sonnet-round-robin",
			Description: "Claude Sonnet 5 轮询，适合高并发场景",
			Tags:        []string{"生产", "高并发", "Anthropic"},
			Steps: []ModelComboStep{
				{Model: "claude-sonnet-5"},
			},
			Models:     []string{"claude-sonnet-5"},
			Strategy:   "round_robin",
			StickyUses: 3,
		},
		{
			Name:        "fast-cheap-fallback",
			Description: "轻量模型优先，失败回退到更小模型，适合批量任务",
			Tags:        []string{"批量", "低成本"},
			Steps: []ModelComboStep{
				{Model: "gpt-4o-mini"},
				{Model: "gpt-3.5-turbo"},
			},
			Models:     []string{"gpt-4o-mini", "gpt-3.5-turbo"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
	}

	changed := false
	for _, builtin := range builtins {
		if _, ok := existing[builtin.Name]; !ok {
			s.config.ComboTemplates = append(s.config.ComboTemplates, builtin)
			changed = true
		}
	}
	if changed {
		_ = s.saveLocked()
	}
}

// generateComboID creates a unique combo identifier
func generateComboID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// migrateModelCombos migrates existing combos to the unified schema.
// It fills in missing fields (combo_id, logical_name, display_name, status, source, version).
func (s *Service) migrateModelCombos() {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	for i := range s.config.ModelCombos {
		c := &s.config.ModelCombos[i]

		// Generate combo_id if missing
		if c.ComboID == "" {
			c.ComboID = generateComboID()
			changed = true
		}

		// Set logical_name from name if empty
		if c.LogicalName == "" && c.Name != "" {
			c.LogicalName = strings.ToLower(strings.ReplaceAll(c.Name, " ", "-"))
			changed = true
		}

		// Set display_name from name if empty
		if c.DisplayName == "" && c.Name != "" {
			c.DisplayName = c.Name
			changed = true
		}

		// Set status to active if empty
		if c.Status == "" {
			c.Status = "active"
			changed = true
		}

		// Set source to local if empty
		if c.Source == "" {
			c.Source = "local"
			changed = true
		}

		// Set version to 1 if 0
		if c.Version == 0 {
			c.Version = 1
			changed = true
		}
	}

	if changed {
		_ = s.saveLocked()
	}
}

func (s *Service) SaveModelCombo(combo ModelCombo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	combo.Name = strings.TrimSpace(combo.Name)
	if combo.Name == "" {
		return errors.New("combo name is required")
	}
	combo.Description = strings.TrimSpace(combo.Description)
	combo.Tags = normalizeTags(combo.Tags)
	combo.Strategy = strings.TrimSpace(combo.Strategy)
	if combo.Strategy == "" {
		combo.Strategy = "fallback"
	}
	if combo.StickyUses <= 0 {
		combo.StickyUses = 1
	}
	combo.Steps = normalizeComboSteps(combo.Steps)
	if len(combo.Steps) == 0 && len(combo.Models) > 0 {
		models := normalizeModels(combo.Models)
		combo.Steps = make([]ModelComboStep, 0, len(models))
		for _, model := range models {
			combo.Steps = append(combo.Steps, ModelComboStep{Model: model})
		}
	}
	combo.Models = comboModelsFromSteps(combo.Steps)
	if len(combo.Steps) == 0 {
		return errors.New("combo must contain at least one model")
	}

	for i := range s.config.ModelCombos {
		if s.config.ModelCombos[i].Name == combo.Name {
			s.config.ModelCombos[i] = combo
			return s.saveLocked()
		}
	}

	s.config.ModelCombos = append(s.config.ModelCombos, combo)
	return s.saveLocked()
}

func (s *Service) DeleteModelCombo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.config.ModelCombos[:0]
	for _, combo := range s.config.ModelCombos {
		if combo.Name != name {
			filtered = append(filtered, combo)
		}
	}
	s.config.ModelCombos = filtered
	return s.saveLocked()
}

var modelTierMap = map[string]int{
	"gpt-5":                    11,
	"gpt-4o":                   10,
	"gpt-4-turbo":              10,
	"gpt-4":                    10,
	"claude-4-opus":            12,
	"claude-4-sonnet":          11,
	"claude-opus-4-20250514":   10,
	"claude-opus-4-8":          12,
	"claude-3-opus":            10,
	"claude-fable-5":           13,
	"claude-sonnet-5":          11,
	"claude-haiku-4-5":         5,
	"o3":                       10,
	"claude-3-5-sonnet":        8,
	"o3-mini":                  9,
	"o1":                       9,
	"o1-mini":                  9,
	"deepseek-v4-pro":          9,
	"glm-5.1":                  9,
	"claude-sonnet-4-20250514": 8,
	"gemini-2.5-pro":           8,
	"kimi-2.6":                 9,
	"qwen-max":                 8,
	"mistral-large":            8,
	"mistral-medium-3.5":       8,
	"llama-3.1-405b":           8,
	"llama-4-maverick":         8,
	"llama-4-scout":            7,
	"gpt-5-mini":               8,
	"gpt-4o-mini":              5,
	"claude-3-5-haiku":         5,
	"gemini-2.5-flash":         5,
	"deepseek-v4-flash":        7,
	"glm-5":                    8,
	"kimi-2.5":                 8,
	"deepseek-chat":            7, // Deprecated — use deepseek-v4-pro after 2026-07-24
	"deepseek-reasoner":        7, // Deprecated — use deepseek-v4-flash (thinking) after 2026-07-24
	"deepseek-coder":           6,
	"qwen-plus":                7,
	"llama-3.1-70b":            7,
	"gpt-5-nano":               6,
	"gemini-2.0-flash":         7,
	"gemini-1.5-pro":           9,
	"gpt-3.5-turbo":            3,
	"claude-3-haiku":           3,
	"gemini-1.5-flash":         3,
	"minimax-m2.7":             7,
	"minimax-m3":               9,
}

var modelCostMap = map[string]float64{
	"gpt-5":                    20.0,
	"gpt-4-turbo":              10.0,
	"gpt-4o":                   5.0,
	"gpt-4":                    30.0,
	"claude-4-opus":            80.0,
	"claude-4-sonnet":          18.0,
	"claude-opus-4-20250514":   15.0,
	"claude-opus-4-8":          30.0,
	"claude-3-opus":            15.0,
	"claude-fable-5":           60.0,
	"claude-sonnet-5":          18.0,
	"claude-haiku-4-5":         1.0,
	"o3":                       10.0,
	"o3-mini":                  1.1,
	"o1":                       15.0,
	"o1-mini":                  1.1,
	"claude-sonnet-4-20250514": 3.0,
	"claude-3-5-sonnet":        3.0,
	"deepseek-v4-pro":          2.0,
	"gemini-2.5-pro":           1.25,
	"glm-5.1":                  3.0,
	"kimi-2.6":                 4.0,
	"deepseek-chat":            0.27, // Deprecated — use deepseek-v4-pro after 2026-07-24
	"deepseek-reasoner":        0.40, // Deprecated — use deepseek-v4-flash after 2026-07-24
	"deepseek-coder":           0.14,
	"deepseek-v4-flash":        0.40,
	"glm-5":                    2.0,
	"kimi-2.5":                 2.5,
	"gpt-5-mini":               1.2,
	"gpt-4o-mini":              0.15,
	"claude-3-5-haiku":         0.8,
	"gemini-2.5-flash":         0.075,
	"gpt-5-nano":               0.40,
	"gpt-3.5-turbo":            0.5,
	"claude-3-haiku":           0.25,
	"gemini-1.5-flash":         0.075,
	"minimax-m2.7":             1.5,
	"minimax-m3":               1.5,
	"mistral-medium-3.5":       9.0,
	"llama-4-maverick":         1.1,
	"llama-4-scout":            0.4,
}

func modelTier(model string) int {
	if tier, ok := modelTierMap[model]; ok {
		return tier
	}
	for k, v := range modelTierMap {
		if strings.HasPrefix(model, k) {
			return v
		}
	}
	return 5
}

// modelCost returns a single cost signal (USD per 1M output tokens) for the
// given model. The canonical price table lives in internal/usage so that cost
// accounting and routing price constraints can never diverge; the map below is
// only a legacy fallback for models not present in that table.
func modelCost(model string) float64 {
	if p, ok := usage.LookupPricing(model); ok {
		return p.OutputPricePer1M
	}
	if cost, ok := modelCostMap[model]; ok {
		return cost
	}
	best := 1.0
	bestLen := -1
	for k, v := range modelCostMap {
		if strings.HasPrefix(model, k) && len(k) > bestLen {
			best = v
			bestLen = len(k)
		}
	}
	return best
}

// ModelCost returns the estimated cost per 1K tokens for the given model.
func ModelCost(model string) float64 {
	return modelCost(model)
}

func (s *Service) ApplyComboSortPreset(comboName string, preset string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset != "intelligence" && preset != "speed" && preset != "budget" {
		return fmt.Errorf("unknown sort preset: %s (valid: intelligence, speed, budget)", preset)
	}

	idx := -1
	for i, combo := range s.config.ModelCombos {
		if combo.Name == comboName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("combo not found: %s", comboName)
	}

	steps := s.config.ModelCombos[idx].Steps
	if len(steps) <= 1 {
		return nil
	}

	switch preset {
	case "intelligence":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelTier(steps[i].Model) > modelTier(steps[j].Model)
		})
	case "speed":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelTier(steps[i].Model) < modelTier(steps[j].Model)
		})
	case "budget":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelCost(steps[i].Model) < modelCost(steps[j].Model)
		})
	}

	s.config.ModelCombos[idx].Steps = steps
	s.config.ModelCombos[idx].Models = comboModelsFromSteps(steps)
	return s.saveLocked()
}

func (s *Service) ListModelCombos() []ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModelCombo, 0, len(s.config.ModelCombos))
	for _, combo := range s.config.ModelCombos {
		combo = normalizeComboForRead(combo)
		result = append(result, combo)
	}
	return result
}

func (s *Service) GetModelCombo(name string) (ModelCombo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, combo := range s.config.ModelCombos {
		if combo.Name == name {
			combo = normalizeComboForRead(combo)
			return combo, true
		}
	}
	return ModelCombo{}, false
}

// FindComboByModel finds a combo that contains the given model in its steps
func (s *Service) FindComboByModel(model string) *ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.config.ModelCombos {
		for _, step := range s.config.ModelCombos[i].Steps {
			if step.Model == model {
				c := s.config.ModelCombos[i]
				c = normalizeComboForRead(c)
				return &c
			}
		}
		for _, m := range s.config.ModelCombos[i].Models {
			if m == model {
				c := s.config.ModelCombos[i]
				c = normalizeComboForRead(c)
				return &c
			}
		}
	}
	return nil
}

func (s *Service) SaveComboTemplate(template ModelCombo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	template.Name = strings.TrimSpace(template.Name)
	if template.Name == "" {
		return errors.New("template name is required")
	}
	template.Description = strings.TrimSpace(template.Description)
	template.Tags = normalizeTags(template.Tags)
	template.Strategy = strings.TrimSpace(template.Strategy)
	if template.Strategy == "" {
		template.Strategy = "fallback"
	}
	if template.StickyUses <= 0 {
		template.StickyUses = 1
	}
	template.Steps = normalizeComboSteps(template.Steps)
	if len(template.Steps) == 0 && len(template.Models) > 0 {
		models := normalizeModels(template.Models)
		template.Steps = make([]ModelComboStep, 0, len(models))
		for _, model := range models {
			template.Steps = append(template.Steps, ModelComboStep{Model: model})
		}
	}
	template.Models = comboModelsFromSteps(template.Steps)
	if len(template.Steps) == 0 {
		return errors.New("template must contain at least one step")
	}

	for i := range s.config.ComboTemplates {
		if s.config.ComboTemplates[i].Name == template.Name {
			s.config.ComboTemplates[i] = template
			return s.saveLocked()
		}
	}

	s.config.ComboTemplates = append(s.config.ComboTemplates, template)
	return s.saveLocked()
}

func (s *Service) DeleteComboTemplate(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.config.ComboTemplates[:0]
	for _, template := range s.config.ComboTemplates {
		if template.Name != name {
			filtered = append(filtered, template)
		}
	}
	s.config.ComboTemplates = filtered
	return s.saveLocked()
}

func (s *Service) RenameComboTemplate(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return errors.New("template names are required")
	}
	for i := range s.config.ComboTemplates {
		if s.config.ComboTemplates[i].Name == oldName {
			s.config.ComboTemplates[i].Name = newName
			return s.saveLocked()
		}
	}
	return errors.New("template not found")
}

func (s *Service) ListComboTemplates() []ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModelCombo, 0, len(s.config.ComboTemplates))
	for _, template := range s.config.ComboTemplates {
		result = append(result, normalizeComboForRead(template))
	}
	return result
}

func normalizeComboSteps(steps []ModelComboStep) []ModelComboStep {
	result := make([]ModelComboStep, 0, len(steps))
	seen := map[string]struct{}{}
	for _, step := range steps {
		step.ChannelID = strings.TrimSpace(step.ChannelID)
		step.Model = strings.TrimSpace(step.Model)
		if step.Model == "" {
			continue
		}
		key := step.ChannelID + "::" + step.Model
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, step)
	}
	return result
}

func comboModelsFromSteps(steps []ModelComboStep) []string {
	models := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Model != "" {
			models = append(models, step.Model)
		}
	}
	return models
}

func normalizeComboForRead(combo ModelCombo) ModelCombo {
	combo.Steps = normalizeComboSteps(combo.Steps)
	combo.Description = strings.TrimSpace(combo.Description)
	combo.Tags = normalizeTags(combo.Tags)
	if len(combo.Steps) == 0 && len(combo.Models) > 0 {
		models := normalizeModels(combo.Models)
		for _, model := range models {
			combo.Steps = append(combo.Steps, ModelComboStep{Model: model})
		}
	}
	combo.Models = comboModelsFromSteps(combo.Steps)
	return combo
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}
