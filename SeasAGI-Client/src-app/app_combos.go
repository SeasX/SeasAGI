package main

import (
	"encoding/json"
	"fmt"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/optimizer"
)

// numberToInt 将 JSON 反序列化后的数值（float64/int/json.Number）安全转为 int。
func numberToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	}
	return 0, false
}

func (a *App) GetDefaultComboName() string {
	return a.configSvc.GetDefaultComboName()
}

func (a *App) SetDefaultComboName(name string) error {
	a.configSvc.SetDefaultComboName(name)
	return nil
}

func (a *App) GetComboByName(name string) config.ModelCombo {
	c, ok := a.configSvc.GetModelCombo(name)
	if !ok {
		return config.ModelCombo{}
	}
	return c
}

func (a *App) ListModelCombos() []config.ModelCombo {
	return a.configSvc.ListModelCombos()
}

func (a *App) SaveModelCombo(combo config.ModelCombo) error {
	return a.configSvc.SaveModelCombo(combo)
}

func (a *App) DeleteModelCombo(name string) error {
	return a.configSvc.DeleteModelCombo(name)
}

func (a *App) ApplyComboSortPreset(comboName string, preset string) error {
	return a.configSvc.ApplyComboSortPreset(comboName, preset)
}

func (a *App) ApplyRecommendation(toModel string, preset string) error {
	if toModel == "" {
		return fmt.Errorf("recommended model is empty")
	}
	presetName := preset
	if presetName == "" {
		presetName = "budget"
	}
	combo := config.ModelCombo{
		Name:        fmt.Sprintf("推荐方案 - %s", toModel),
		Description: fmt.Sprintf("由智能优化引擎推荐，目标模型：%s，排序预设：%s", toModel, presetName),
		Steps:       []config.ModelComboStep{{ChannelID: "", Model: toModel}},
		Strategy:    "fallback",
		StickyUses:  1,
	}
	err := a.configSvc.SaveModelCombo(combo)
	if err != nil {
		return fmt.Errorf("failed to apply recommendation: %w", err)
	}
	return a.configSvc.ApplyComboSortPreset(combo.Name, presetName)
}

// PreviewComboOptimization returns a detailed preview of an optimization recommendation
// without applying it. Returns the recommendation details, estimated impact, and current vs new step comparison.
func (a *App) PreviewComboOptimization(mode string, taskType string) map[string]any {
	if mode == "" {
		mode = "value_first"
	}
	if taskType == "" {
		taskType = optimizer.TaskGeneralChat
	}
	plan := a.optimizerSvc.GetOptimizationPlan(mode, taskType)
	if plan == nil || len(plan.Recommendations) == 0 {
		return map[string]any{
			"has_recommendations": false,
			"message":             "当前暂无优化建议",
		}
	}

	previews := make([]map[string]any, 0, len(plan.Recommendations))
	for _, rec := range plan.Recommendations {
		currentCombo := a.configSvc.FindComboByModel(rec.FromModel)
		currentSteps := make([]map[string]string, 0)
		if currentCombo != nil {
			for _, step := range currentCombo.Steps {
				currentSteps = append(currentSteps, map[string]string{
					"model":   step.Model,
					"channel": step.ChannelID,
				})
			}
		}

		previews = append(previews, map[string]any{
			"from_model":     rec.FromModel,
			"to_model":       rec.ToModel,
			"model_tag":      rec.ModelTag,
			"channel_id":     rec.ChannelID,
			"channel_name":   rec.ChannelName,
			"savings_usd":    rec.SavingsUSD,
			"quality_diff":   rec.QualityDiff,
			"avg_latency_ms": rec.AvgLatencyMs,
			"error_rate":     rec.ErrorRate,
			"reason":         rec.Reason,
			"current_steps":  currentSteps,
			"proposed_steps": []map[string]string{
				{"model": rec.ToModel, "channel": rec.ChannelID},
			},
		})
	}

	return map[string]any{
		"has_recommendations": true,
		"mode":                plan.Mode,
		"strategy":            plan.Strategy,
		"monthly_savings":     plan.MonthlySavings,
		"recommendations":     previews,
	}
}

// ApplyComboOptimization applies a specific optimization recommendation as a new combo.
// If update_existing is true, it updates the existing combo for the from_model instead of creating a new one.
func (a *App) ApplyComboOptimization(toModel string, preset string, updateExisting bool) error {
	if toModel == "" {
		return fmt.Errorf("target model is required")
	}
	presetName := preset
	if presetName == "" {
		presetName = "budget"
	}

	// Try to find existing combo for the from-model if updateExisting is true
	plan := a.optimizerSvc.GetOptimizationPlan("value_first", optimizer.TaskGeneralChat)
	var fromModel string
	if plan != nil {
		for _, rec := range plan.Recommendations {
			if rec.ToModel == toModel {
				fromModel = rec.FromModel
				break
			}
		}
	}

	if updateExisting && fromModel != "" {
		existingCombo := a.configSvc.FindComboByModel(fromModel)
		if existingCombo != nil {
			existingCombo.Steps = []config.ModelComboStep{
				{ChannelID: "", Model: toModel},
			}
			existingCombo.Description = fmt.Sprintf("由智能优化自动更新，新目标模型：%s", toModel)
			if err := a.configSvc.SaveModelCombo(*existingCombo); err != nil {
				return fmt.Errorf("failed to update combo: %w", err)
			}
			return a.configSvc.ApplyComboSortPreset(existingCombo.Name, presetName)
		}
	}

	// Create new combo
	comboName := fmt.Sprintf("优化方案 - %s", toModel)
	combo := config.ModelCombo{
		Name:        comboName,
		Description: fmt.Sprintf("由智能优化引擎生成，目标模型：%s，排序预设：%s", toModel, presetName),
		Steps:       []config.ModelComboStep{{ChannelID: "", Model: toModel}},
		Strategy:    "fallback",
		StickyUses:  1,
	}
	if err := a.configSvc.SaveModelCombo(combo); err != nil {
		return fmt.Errorf("failed to save combo: %w", err)
	}
	return a.configSvc.ApplyComboSortPreset(comboName, presetName)
}

func (a *App) SyncOptimizationConfigToCloud() error {
	cfg := a.configSvc.GetOptimizationConfig()
	raw, err := json.Marshal(map[string]any{
		"config": map[string]any{
			"mode":                 cfg.Mode,
			"penalty_enabled":      cfg.PenaltyEnabled,
			"penalty_decay_sec":    cfg.PenaltyDecaySec,
			"health_check_enabled": cfg.HealthCheckEnabled,
			"health_check_sec":     cfg.HealthCheckSec,
			"health_max_failures":  cfg.HealthMaxFailures,
			"cooldown_enabled":     cfg.CooldownEnabled,
			"cooldown_sec":         cfg.CooldownSec,
			"sticky_enabled":       cfg.StickyEnabled,
			"sticky_ttl_sec":       cfg.StickyTTLSec,
			"preset_enabled":       cfg.PresetEnabled,
			"default_preset":       cfg.DefaultPreset,
		},
	})
	if err != nil {
		return err
	}
	return a.authSvc.PushCloudOptimizationConfig(string(raw))
}

func (a *App) SyncOptimizationConfigFromCloud() map[string]any {
	raw, err := a.authSvc.FetchCloudOptimizationConfig()
	if err != nil || raw == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	cfgMap, ok := parsed["config"].(map[string]any)
	if !ok {
		return nil
	}

	local := a.configSvc.GetOptimizationConfig()
	if v, ok := cfgMap["mode"]; ok {
		if s, ok := v.(string); ok && s != "" {
			local.Mode = s
		}
	}
	if v, ok := cfgMap["penalty_enabled"]; ok {
		local.PenaltyEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["penalty_decay_sec"]; ok {
		local.PenaltyDecaySec, _ = toInt(v)
	}
	if v, ok := cfgMap["health_check_enabled"]; ok {
		local.HealthCheckEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["health_check_sec"]; ok {
		local.HealthCheckSec, _ = toInt(v)
	}
	if v, ok := cfgMap["health_max_failures"]; ok {
		local.HealthMaxFailures, _ = toInt(v)
	}
	if v, ok := cfgMap["cooldown_enabled"]; ok {
		local.CooldownEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["cooldown_sec"]; ok {
		local.CooldownSec, _ = toInt(v)
	}
	if v, ok := cfgMap["sticky_enabled"]; ok {
		local.StickyEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["sticky_ttl_sec"]; ok {
		local.StickyTTLSec, _ = toInt(v)
	}
	if v, ok := cfgMap["preset_enabled"]; ok {
		local.PresetEnabled, _ = v.(bool)
	}
	if v, ok := cfgMap["default_preset"]; ok {
		if s, ok := v.(string); ok && s != "" {
			local.DefaultPreset = s
		}
	}
	_ = a.configSvc.SetOptimizationConfig(local)

	return map[string]any{
		"mode":                 local.Mode,
		"penalty_enabled":      local.PenaltyEnabled,
		"penalty_decay_sec":    local.PenaltyDecaySec,
		"health_check_enabled": local.HealthCheckEnabled,
		"health_check_sec":     local.HealthCheckSec,
		"health_max_failures":  local.HealthMaxFailures,
		"cooldown_enabled":     local.CooldownEnabled,
		"cooldown_sec":         local.CooldownSec,
		"sticky_enabled":       local.StickyEnabled,
		"sticky_ttl_sec":       local.StickyTTLSec,
		"preset_enabled":       local.PresetEnabled,
		"default_preset":       local.DefaultPreset,
	}
}

func (a *App) FetchModelStats() []map[string]any {
	entries, err := a.authSvc.FetchCloudModelStats()
	if err != nil {
		return nil
	}
	stats := make([]optimizer.ModelStatsInfo, 0, len(entries))
	result := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		stats = append(stats, optimizer.ModelStatsInfo{
			Model:         e.Model,
			TotalRequests: e.TotalRequests,
			TotalErrors:   e.TotalErrors,
			AvgLatencyMs:  e.AvgLatencyMs,
			ErrorRate:     e.ErrorRate,
		})
		result = append(result, map[string]any{
			"model":          e.Model,
			"total_requests": e.TotalRequests,
			"total_errors":   e.TotalErrors,
			"avg_latency_ms": e.AvgLatencyMs,
			"error_rate":     e.ErrorRate,
		})
	}
	a.optimizerSvc.SetCloudModelStats(stats)
	return result
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func (a *App) ListComboTemplates() []config.ModelCombo {
	return a.configSvc.ListComboTemplates()
}

func (a *App) SaveComboTemplate(template config.ModelCombo) error {
	return a.configSvc.SaveComboTemplate(template)
}

func (a *App) DeleteComboTemplate(name string) error {
	return a.configSvc.DeleteComboTemplate(name)
}

func (a *App) RenameComboTemplate(oldName, newName string) error {
	return a.configSvc.RenameComboTemplate(oldName, newName)
}

func (a *App) GetRecommendedCombos() ([]map[string]any, error) {
	combos, err := a.authSvc.FetchRecommendedCombos()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(combos))
	for _, combo := range combos {
		result = append(result, map[string]any{
			"name":        combo.Name,
			"models":      combo.Models,
			"description": combo.Description,
			"strategy":    combo.Strategy,
		})
	}
	return result, nil
}

func (a *App) GetOfficialComboTemplates() ([]map[string]any, error) {
	templates, err := a.authSvc.FetchOfficialComboTemplates(a.ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(templates))
	for _, tmpl := range templates {
		steps := make([]map[string]any, 0, len(tmpl.Steps))
		for _, step := range tmpl.Steps {
			steps = append(steps, map[string]any{
				"channel_id": step.ChannelID,
				"model":      step.Model,
			})
		}
		result = append(result, map[string]any{
			"name":        tmpl.Name,
			"description": tmpl.Description,
			"tags":        tmpl.Tags,
			"steps":       steps,
			"models":      tmpl.Models,
			"strategy":    tmpl.Strategy,
			"sticky_uses": tmpl.StickyUses,
		})
	}
	return result, nil
}

func (a *App) GetOptimizationPlan(mode string, taskType string) map[string]any {
	if mode == "" {
		mode = "value_first"
	}
	if taskType == "" {
		taskType = optimizer.TaskGeneralChat
	}
	plan := a.optimizerSvc.GetOptimizationPlan(mode, taskType)
	if plan == nil {
		return nil
	}
	recommendations := make([]map[string]any, 0, len(plan.Recommendations))
	for _, rec := range plan.Recommendations {
		recommendations = append(recommendations, map[string]any{
			"from_model":   rec.FromModel,
			"to_model":     rec.ToModel,
			"model_tag":    rec.ModelTag,
			"channel_id":   rec.ChannelID,
			"channel_name": rec.ChannelName,
			"savings_usd":  rec.SavingsUSD,
			"quality_diff": rec.QualityDiff,
			"reason":       rec.Reason,
		})
	}
	return map[string]any{
		"recommendations": recommendations,
		"monthly_savings": plan.MonthlySavings,
		"strategy":        plan.Strategy,
		"mode":            plan.Mode,
		"task_type":       plan.TaskType,
	}
}

func (a *App) GetQuickStrategies() []map[string]any {
	return []map[string]any{
		{
			"alias":        "quality_first",
			"display_name": "质量优先",
			"description":  "优先分配高智商模型（Claude/GPT-4 系），适合复杂推理与代码任务",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"anthropic", "openai", "google"},
				"fallback_order":     []string{"anthropic", "openai", "google"},
				"min_success_rate":   0.95,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
		{
			"alias":        "cost_first",
			"display_name": "性价比优先",
			"description":  "优先选择成本更低的模型与提供商组合",
			"task_profile": map[string]any{
				"task_type":            optimizer.TaskGeneralChat,
				"priority_providers":   []string{"openrouter", "google", "openai"},
				"fallback_order":       []string{"openrouter", "google", "openai"},
				"max_cost_per_request": 0.02,
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "balanced",
			"display_name": "均衡推荐",
			"description":  "质量与成本并重，按意图场景在中高端与轻量模型间动态取舍",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
				"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "stable_first",
			"display_name": "稳定优先",
			"description":  "优先选择稳定性高、回退链清晰的模型组合",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"openai", "anthropic", "google"},
				"fallback_order":     []string{"openai", "anthropic", "google"},
				"min_success_rate":   0.98,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
		{
			"alias":        "speed_first",
			"display_name": "速度优先",
			"description":  "优先选择低延迟模型，适合交互式场景",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskGeneralChat,
				"priority_providers": []string{"google", "openai", "anthropic"},
				"fallback_order":     []string{"google", "openai", "anthropic"},
				"max_latency_ms":     1500,
			},
			"combo_constraints": map[string]any{
				"min_steps":          1,
				"max_steps":          2,
				"allowed_strategies": []string{"fallback", "round_robin"},
			},
		},
		{
			"alias":        "tools_first",
			"display_name": "工具优先",
			"description":  "优先选择工具调用成功率和结构化输出稳定性更高的模型",
			"task_profile": map[string]any{
				"task_type":          optimizer.TaskToolCalling,
				"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
				"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
				"min_success_rate":   0.97,
			},
			"combo_constraints": map[string]any{
				"min_steps":          2,
				"max_steps":          3,
				"allowed_strategies": []string{"fallback"},
			},
		},
	}
}

func (a *App) GetTaskProfiles() []map[string]any {
	return []map[string]any{
		{
			"task_type":          optimizer.TaskGeneralChat,
			"priority_providers": []string{"openai", "anthropic", "google"},
			"fallback_order":     []string{"openai", "anthropic", "google"},
			"min_success_rate":   0.95,
		},
		{
			"task_type":          optimizer.TaskToolCalling,
			"priority_providers": []string{"openai", "anthropic", "google", "openrouter"},
			"fallback_order":     []string{"openai", "anthropic", "google", "openrouter"},
			"min_success_rate":   0.97,
		},
		{
			"task_type":          optimizer.TaskStructured,
			"priority_providers": []string{"openai", "anthropic", "google"},
			"fallback_order":     []string{"openai", "google", "anthropic"},
			"min_success_rate":   0.96,
		},
		{
			"task_type":          optimizer.TaskLongContext,
			"priority_providers": []string{"google", "anthropic", "openai"},
			"fallback_order":     []string{"google", "anthropic", "openai"},
			"max_latency_ms":     4000,
			"min_success_rate":   0.94,
		},
		{
			"task_type":          optimizer.TaskVision,
			"priority_providers": []string{"openai", "google", "anthropic"},
			"fallback_order":     []string{"openai", "google", "anthropic"},
			"min_success_rate":   0.95,
		},
	}
}

func (a *App) GetOptimizationConfig() map[string]any {
	cfg := a.configSvc.GetOptimizationConfig()
	return map[string]any{
		"mode":                 cfg.Mode,
		"penalty_enabled":      cfg.PenaltyEnabled,
		"penalty_decay_sec":    cfg.PenaltyDecaySec,
		"health_check_enabled": cfg.HealthCheckEnabled,
		"health_check_sec":     cfg.HealthCheckSec,
		"health_max_failures":  cfg.HealthMaxFailures,
		"cooldown_enabled":     cfg.CooldownEnabled,
		"cooldown_sec":         cfg.CooldownSec,
		"sticky_enabled":       cfg.StickyEnabled,
		"sticky_ttl_sec":       cfg.StickyTTLSec,
		"preset_enabled":       cfg.PresetEnabled,
		"default_preset":       cfg.DefaultPreset,
	}
}

func (a *App) SetOptimizationConfig(cfg map[string]any) error {
	parsed := config.OptimizationConfig{}
	if v, ok := cfg["mode"]; ok {
		parsed.Mode, _ = v.(string)
	}
	if v, ok := cfg["penalty_enabled"]; ok {
		parsed.PenaltyEnabled, _ = v.(bool)
	}
	if v, ok := cfg["penalty_decay_sec"]; ok {
		parsed.PenaltyDecaySec, _ = v.(int)
	}
	if v, ok := cfg["health_check_enabled"]; ok {
		parsed.HealthCheckEnabled, _ = v.(bool)
	}
	if v, ok := cfg["health_check_sec"]; ok {
		parsed.HealthCheckSec, _ = v.(int)
	}
	if v, ok := cfg["health_max_failures"]; ok {
		parsed.HealthMaxFailures, _ = v.(int)
	}
	if v, ok := cfg["cooldown_enabled"]; ok {
		parsed.CooldownEnabled, _ = v.(bool)
	}
	if v, ok := cfg["cooldown_sec"]; ok {
		parsed.CooldownSec, _ = v.(int)
	}
	if v, ok := cfg["sticky_enabled"]; ok {
		parsed.StickyEnabled, _ = v.(bool)
	}
	if v, ok := cfg["sticky_ttl_sec"]; ok {
		parsed.StickyTTLSec, _ = v.(int)
	}
	if v, ok := cfg["preset_enabled"]; ok {
		parsed.PresetEnabled, _ = v.(bool)
	}
	if v, ok := cfg["default_preset"]; ok {
		parsed.DefaultPreset, _ = v.(string)
	}
	return a.configSvc.SetOptimizationConfig(parsed)
}
