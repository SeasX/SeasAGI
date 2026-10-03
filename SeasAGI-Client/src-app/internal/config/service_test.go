package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// newTestService creates a Service with a temporary config directory for testing.
func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	svc := &Service{
		config: AppConfig{
			ListenPort:       4318,
			RoutingStrategy:  "fallback",
			StickyChannelUse: 2,
		},
		channels: []Channel{},
		path:     filepath.Join(dir, "config.json"),
	}
	svc.ensureOptimizationConfig()
	svc.ensureBuiltinTemplates()
	svc.migrateModelCombos()
	return svc
}

// ---------------------------------------------------------------------------
// modelTierMap tests
// ---------------------------------------------------------------------------

func TestModelTierMap_HasAllExpectedModels(t *testing.T) {
	expectedModels := []string{
		"gpt-5", "gpt-4o", "gpt-4-turbo", "gpt-4",
		"claude-4-opus", "claude-4-sonnet", "claude-opus-4-20250514",
		"claude-opus-4-8", "claude-3-opus", "claude-fable-5",
		"claude-sonnet-5", "claude-haiku-4-5",
		"o3", "o3-mini", "o1", "o1-mini",
		"deepseek-v4-pro", "deepseek-chat", "deepseek-reasoner", "deepseek-coder", "deepseek-v4-flash",
		"glm-5.1", "glm-5",
		"gemini-2.5-pro", "gemini-2.5-flash", "gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash",
		"kimi-2.6", "kimi-2.5",
		"qwen-max", "qwen-plus",
		"mistral-large", "mistral-medium-3.5",
		"llama-3.1-405b", "llama-3.1-70b", "llama-4-maverick", "llama-4-scout",
		"gpt-5-mini", "gpt-5-nano", "gpt-4o-mini", "gpt-3.5-turbo",
		"claude-3-5-sonnet", "claude-3-5-haiku", "claude-3-haiku",
		"minimax-m2.7", "minimax-m3",
	}

	for _, model := range expectedModels {
		_, ok := modelTierMap[model]
		if !ok {
			t.Errorf("modelTierMap missing expected entry: %q", model)
		}
	}
}

func TestModelTierMap_ClaudeFable5Tier13(t *testing.T) {
	if tier, ok := modelTierMap["claude-fable-5"]; !ok {
		t.Error("modelTierMap missing claude-fable-5")
	} else if tier != 13 {
		t.Errorf("claude-fable-5 tier = %d, want 13", tier)
	}
}

func TestModelTierMap_NoNegativeTiers(t *testing.T) {
	for model, tier := range modelTierMap {
		if tier < 0 {
			t.Errorf("modelTierMap has negative tier for %q: %d", model, tier)
		}
	}
}

func TestModelTierMap_NoDuplicateEntries(t *testing.T) {
	// In Go, a map literal with duplicate keys would be a compile error,
	// so we can only verify at runtime that we can iterate without panics
	// and that the count matches expectations (~49 entries).
	seen := make(map[string]int)
	for model, tier := range modelTierMap {
		if prev, ok := seen[model]; ok {
			t.Errorf("modelTierMap has duplicate key %q (prev tier=%d, new tier=%d)", model, prev, tier)
		}
		seen[model] = tier
	}
}

// ---------------------------------------------------------------------------
// modelCostMap tests
// ---------------------------------------------------------------------------

func TestModelCostMap_HasAllExpectedModels(t *testing.T) {
	expectedModels := []string{
		"gpt-5", "gpt-4o", "gpt-4-turbo", "gpt-4",
		"claude-4-opus", "claude-4-sonnet", "claude-opus-4-20250514",
		"claude-opus-4-8", "claude-3-opus", "claude-fable-5",
		"claude-sonnet-5", "claude-haiku-4-5",
		"o3", "o3-mini", "o1", "o1-mini",
		"claude-sonnet-4-20250514", "claude-3-5-sonnet",
		"deepseek-v4-pro", "deepseek-chat", "deepseek-reasoner", "deepseek-coder", "deepseek-v4-flash",
		"gemini-2.5-pro", "gemini-2.5-flash", "gemini-1.5-flash",
		"glm-5.1", "glm-5",
		"kimi-2.6", "kimi-2.5",
		"gpt-5-mini", "gpt-5-nano", "gpt-4o-mini", "gpt-3.5-turbo",
		"claude-3-5-haiku", "claude-3-haiku",
		"minimax-m2.7", "minimax-m3",
		"mistral-medium-3.5",
		"llama-4-maverick", "llama-4-scout",
	}

	for _, model := range expectedModels {
		_, ok := modelCostMap[model]
		if !ok {
			t.Errorf("modelCostMap missing expected entry: %q", model)
		}
	}
}

func TestModelCostMap_ClaudeOpus4Cost80(t *testing.T) {
	if cost, ok := modelCostMap["claude-4-opus"]; !ok {
		t.Error("modelCostMap missing claude-4-opus")
	} else if cost != 80.0 {
		t.Errorf("claude-4-opus cost = %f, want 80.0", cost)
	}
}

func TestModelCostMap_NoNegativeCosts(t *testing.T) {
	for model, cost := range modelCostMap {
		if cost < 0 {
			t.Errorf("modelCostMap has negative cost for %q: %f", model, cost)
		}
	}
}

// ---------------------------------------------------------------------------
// NewService tests
// ---------------------------------------------------------------------------

func TestNewService_CreatesNonNil(t *testing.T) {
	svc := newTestService(t)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.config.ListenPort != 4318 {
		t.Errorf("expected ListenPort 4318, got %d", svc.config.ListenPort)
	}
	if svc.channels == nil {
		t.Error("channels should not be nil")
	}
}

// ---------------------------------------------------------------------------
// ensureBuiltinTemplates tests
// ---------------------------------------------------------------------------

func TestEnsureBuiltinTemplates_HasAtLeast4Templates(t *testing.T) {
	svc := newTestService(t)
	if len(svc.config.ComboTemplates) < 4 {
		t.Errorf("expected at least 4 built-in templates, got %d", len(svc.config.ComboTemplates))
	}
}

// ---------------------------------------------------------------------------
// ApplyComboSortPreset tests
// ---------------------------------------------------------------------------

func TestApplyComboSortPreset_Intelligence(t *testing.T) {
	svc := newTestService(t)
	// Add a combo with multiple steps
	combo := ModelCombo{
		Name:     "test-combo-intel",
		Strategy: "fallback",
		Steps: []ModelComboStep{
			{Model: "gpt-4o-mini"},     // tier 5
			{Model: "claude-opus-4-8"}, // tier 12
			{Model: "gpt-4o"},          // tier 10
		},
	}
	if err := svc.SaveModelCombo(combo); err != nil {
		t.Fatalf("SaveModelCombo: %v", err)
	}

	if err := svc.ApplyComboSortPreset("test-combo-intel", "intelligence"); err != nil {
		t.Fatalf("ApplyComboSortPreset: %v", err)
	}

	updated, ok := svc.GetModelCombo("test-combo-intel")
	if !ok {
		t.Fatal("combo not found after sort")
	}
	if len(updated.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(updated.Steps))
	}
	// Intelligence = descending tier: claude-opus-4-8 (12) > gpt-4o (10) > gpt-4o-mini (5)
	expected := []string{"claude-opus-4-8", "gpt-4o", "gpt-4o-mini"}
	for i, step := range updated.Steps {
		if step.Model != expected[i] {
			t.Errorf("step[%d] model = %q, want %q", i, step.Model, expected[i])
		}
	}
}

func TestApplyComboSortPreset_Speed(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "test-combo-speed",
		Strategy: "fallback",
		Steps: []ModelComboStep{
			{Model: "claude-opus-4-8"}, // tier 12
			{Model: "gpt-4o-mini"},     // tier 5
			{Model: "gpt-4o"},          // tier 10
		},
	}
	if err := svc.SaveModelCombo(combo); err != nil {
		t.Fatalf("SaveModelCombo: %v", err)
	}

	if err := svc.ApplyComboSortPreset("test-combo-speed", "speed"); err != nil {
		t.Fatalf("ApplyComboSortPreset: %v", err)
	}

	updated, ok := svc.GetModelCombo("test-combo-speed")
	if !ok {
		t.Fatal("combo not found after sort")
	}
	// Speed = ascending tier: gpt-4o-mini (5) < gpt-4o (10) < claude-opus-4-8 (12)
	expected := []string{"gpt-4o-mini", "gpt-4o", "claude-opus-4-8"}
	for i, step := range updated.Steps {
		if step.Model != expected[i] {
			t.Errorf("step[%d] model = %q, want %q", i, step.Model, expected[i])
		}
	}
}

func TestApplyComboSortPreset_Budget(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "test-combo-budget",
		Strategy: "fallback",
		Steps: []ModelComboStep{
			{Model: "claude-4-opus"},   // cost 80.0
			{Model: "gpt-4o-mini"},     // cost 0.15
			{Model: "claude-sonnet-5"}, // cost 18.0
		},
	}
	if err := svc.SaveModelCombo(combo); err != nil {
		t.Fatalf("SaveModelCombo: %v", err)
	}

	if err := svc.ApplyComboSortPreset("test-combo-budget", "budget"); err != nil {
		t.Fatalf("ApplyComboSortPreset: %v", err)
	}

	updated, ok := svc.GetModelCombo("test-combo-budget")
	if !ok {
		t.Fatal("combo not found after sort")
	}
	// Budget = ascending cost: gpt-4o-mini (0.15) < claude-sonnet-5 (18.0) < claude-4-opus (80.0)
	expected := []string{"gpt-4o-mini", "claude-sonnet-5", "claude-4-opus"}
	for i, step := range updated.Steps {
		if step.Model != expected[i] {
			t.Errorf("step[%d] model = %q, want %q", i, step.Model, expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// ResolveChannelsForModel tests
// ---------------------------------------------------------------------------

func TestResolveChannelsForModel_NoChannels(t *testing.T) {
	svc := newTestService(t)
	channels, resolvedModel := svc.ResolveChannelsForModel("gpt-4o")
	if len(channels) != 0 {
		t.Errorf("expected 0 channels, got %d", len(channels))
	}
	if resolvedModel != "gpt-4o" {
		t.Errorf("expected resolved model 'gpt-4o', got %q", resolvedModel)
	}
}

// ---------------------------------------------------------------------------
// UpdateSetting / GetSetting tests
// ---------------------------------------------------------------------------

func TestUpdateSetting_And_GetSetting(t *testing.T) {
	svc := newTestService(t)

	// Update locale
	if err := svc.UpdateSetting("locale", "zh-CN"); err != nil {
		t.Fatalf("UpdateSetting locale: %v", err)
	}
	if got := svc.GetSetting("locale"); got != "zh-CN" {
		t.Errorf("GetSetting locale = %q, want %q", got, "zh-CN")
	}

	// Update auto_update
	if err := svc.UpdateSetting("auto_update", false); err != nil {
		t.Fatalf("UpdateSetting auto_update: %v", err)
	}
	if got := svc.GetSetting("auto_update"); got != "false" {
		t.Errorf("GetSetting auto_update = %q, want %q", got, "false")
	}

	// Update analytics_enabled
	if err := svc.UpdateSetting("analytics_enabled", true); err != nil {
		t.Fatalf("UpdateSetting analytics_enabled: %v", err)
	}
	if got := svc.GetSetting("analytics_enabled"); got != "true" {
		t.Errorf("GetSetting analytics_enabled = %q, want %q", got, "true")
	}

	// Update log_retention_days
	if err := svc.UpdateSetting("log_retention_days", 60); err != nil {
		t.Fatalf("UpdateSetting log_retention_days: %v", err)
	}
	if got := svc.GetSetting("log_retention_days"); got != "60" {
		t.Errorf("GetSetting log_retention_days = %q, want %q", got, "60")
	}
}

func TestGetSetting_ReturnsEmptyForUnknownKey(t *testing.T) {
	svc := newTestService(t)
	if got := svc.GetSetting("nonexistent"); got != "" {
		t.Errorf("expected empty string for unknown key, got %q", got)
	}
}

func TestUpdateSetting_TypeMismatchDoesNotChangeValue(t *testing.T) {
	svc := newTestService(t)

	// Set locale to a known value
	_ = svc.UpdateSetting("locale", "en-US")
	if got := svc.GetSetting("locale"); got != "en-US" {
		t.Fatalf("expected en-US, got %q", got)
	}

	// Try to update with wrong type (int instead of string)
	_ = svc.UpdateSetting("locale", 42)
	if got := svc.GetSetting("locale"); got != "en-US" {
		t.Errorf("locale changed despite type mismatch: %q", got)
	}
}

// ---------------------------------------------------------------------------
// modelTier / modelCost helper tests
// ---------------------------------------------------------------------------

func TestModelTier_UnknownModelReturnsDefault(t *testing.T) {
	tier := modelTier("nonexistent-model-xyz")
	if tier != 5 {
		t.Errorf("unknown model tier = %d, want 5 (default)", tier)
	}
}

func TestModelCost_UnknownModelReturnsDefault(t *testing.T) {
	cost := modelCost("nonexistent-model-xyz")
	if cost != 1.0 {
		t.Errorf("unknown model cost = %f, want 1.0 (default)", cost)
	}
}

// ---------------------------------------------------------------------------
// Edge cases for SaveModelCombo / DeleteModelCombo / GetModelCombo
// ---------------------------------------------------------------------------

func TestSaveModelCombo_EmptyName(t *testing.T) {
	svc := newTestService(t)
	err := svc.SaveModelCombo(ModelCombo{Name: "", Strategy: "fallback"})
	if err == nil {
		t.Error("expected error for empty combo name")
	}
}

func TestSaveModelCombo_NoSteps(t *testing.T) {
	svc := newTestService(t)
	err := svc.SaveModelCombo(ModelCombo{Name: "no-steps", Strategy: "fallback"})
	if err == nil {
		t.Error("expected error for combo with no steps")
	}
}

func TestSaveModelCombo_NoStepsButHasModels(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "models-only",
		Strategy: "fallback",
		Models:   []string{"gpt-4o", "gpt-4o-mini"},
	}
	if err := svc.SaveModelCombo(combo); err != nil {
		t.Fatalf("SaveModelCombo: %v", err)
	}
	saved, ok := svc.GetModelCombo("models-only")
	if !ok {
		t.Fatal("combo not found")
	}
	if len(saved.Steps) != 2 {
		t.Errorf("expected 2 steps derived from Models, got %d", len(saved.Steps))
	}
}

func TestDeleteModelCombo(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "to-delete",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}},
	}
	_ = svc.SaveModelCombo(combo)

	if err := svc.DeleteModelCombo("to-delete"); err != nil {
		t.Fatalf("DeleteModelCombo: %v", err)
	}
	_, ok := svc.GetModelCombo("to-delete")
	if ok {
		t.Error("combo still exists after delete")
	}
}

func TestListModelCombos(t *testing.T) {
	svc := newTestService(t)
	combos := svc.ListModelCombos()
	if len(combos) != 0 {
		t.Errorf("expected 0 combos initially, got %d", len(combos))
	}

	// Add a combo and verify it appears
	combo := ModelCombo{
		Name:     "listed-combo",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}},
	}
	_ = svc.SaveModelCombo(combo)

	combos = svc.ListModelCombos()
	if len(combos) != 1 {
		t.Fatalf("expected 1 combo, got %d", len(combos))
	}
	if combos[0].Name != "listed-combo" {
		t.Errorf("combo name = %q, want %q", combos[0].Name, "listed-combo")
	}
}

// ---------------------------------------------------------------------------
// SaveCustomChannel / DeleteCustomChannel / ReorderChannels tests
// ---------------------------------------------------------------------------

func TestSaveCustomChannel_CreatesChannelID(t *testing.T) {
	svc := newTestService(t)
	id, err := svc.SaveCustomChannel(Channel{
		DisplayName: "test-channel",
		BaseURL:     "https://test.example.com",
		Models:      []string{"gpt-4o"},
	})
	if err != nil {
		t.Fatalf("SaveCustomChannel: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty channel ID")
	}
}

func TestDeleteCustomChannel(t *testing.T) {
	svc := newTestService(t)
	id, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "delete-me",
		BaseURL:     "https://delete.example.com",
		Models:      []string{"gpt-4o"},
	})
	if err := svc.DeleteCustomChannel(id); err != nil {
		t.Fatalf("DeleteCustomChannel: %v", err)
	}
	_, ok := svc.GetChannel(id)
	if ok {
		t.Error("channel still exists after delete")
	}
}

func TestReorderChannels(t *testing.T) {
	svc := newTestService(t)
	id1, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "first",
		BaseURL:     "https://first.example.com",
		Models:      []string{"gpt-4o"},
	})
	id2, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "second",
		BaseURL:     "https://second.example.com",
		Models:      []string{"gpt-4o-mini"},
	})
	id3, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "third",
		BaseURL:     "https://third.example.com",
		Models:      []string{"claude-sonnet-5"},
	})

	// Reorder: third, first, second
	if err := svc.ReorderChannels([]string{id3, id1, id2}); err != nil {
		t.Fatalf("ReorderChannels: %v", err)
	}

	channels, _ := svc.ListChannels()
	if len(channels) != 3 {
		t.Fatalf("expected 3 channels, got %d", len(channels))
	}
	if channels[0].ChannelID != id3 {
		t.Errorf("expected first channel to be %q, got %q", id3, channels[0].ChannelID)
	}
	if channels[1].ChannelID != id1 {
		t.Errorf("expected second channel to be %q, got %q", id1, channels[1].ChannelID)
	}
	if channels[2].ChannelID != id2 {
		t.Errorf("expected third channel to be %q, got %q", id2, channels[2].ChannelID)
	}
}

// ---------------------------------------------------------------------------
// migrateModelCombos tests
// ---------------------------------------------------------------------------

func TestMigrateModelCombos_FillsMissingFields(t *testing.T) {
	svc := newTestService(t)
	// Manually add a combo with missing fields
	svc.mu.Lock()
	svc.config.ModelCombos = append(svc.config.ModelCombos, ModelCombo{
		Name: "minimal-combo",
		Steps: []ModelComboStep{
			{Model: "gpt-4o"},
		},
		Strategy: "fallback",
	})
	svc.mu.Unlock()

	svc.migrateModelCombos()

	combo, ok := svc.GetModelCombo("minimal-combo")
	if !ok {
		t.Fatal("combo not found after migration")
	}
	if combo.ComboID == "" {
		t.Error("combo_id should not be empty after migration")
	}
	if combo.LogicalName == "" {
		t.Error("logical_name should not be empty after migration")
	}
	if combo.DisplayName == "" {
		t.Error("display_name should not be empty after migration")
	}
	if combo.Status != "active" {
		t.Errorf("status = %q, want %q", combo.Status, "active")
	}
	if combo.Source != "local" {
		t.Errorf("source = %q, want %q", combo.Source, "local")
	}
	if combo.Version != 1 {
		t.Errorf("version = %d, want 1", combo.Version)
	}
}

// ---------------------------------------------------------------------------
// GetConfig / GetOptimizationConfig tests
// ---------------------------------------------------------------------------

func TestGetConfig_Defaults(t *testing.T) {
	svc := newTestService(t)
	cfg := svc.GetConfig()
	if cfg.RoutingStrategy != "fallback" {
		t.Errorf("RoutingStrategy = %q, want %q", cfg.RoutingStrategy, "fallback")
	}
	if cfg.StickyChannelUse != 2 {
		t.Errorf("StickyChannelUse = %d, want 2", cfg.StickyChannelUse)
	}
}

func TestGetOptimizationConfig_ReturnsDefaultsWhenNil(t *testing.T) {
	svc := newTestService(t)
	// ensureOptimizationConfig already sets it, so set it to nil to test
	svc.mu.Lock()
	svc.config.Optimizations = nil
	svc.mu.Unlock()

	opt := svc.GetOptimizationConfig()
	if opt.Mode != "value_first" {
		t.Errorf("Mode = %q, want %q", opt.Mode, "value_first")
	}
	if !opt.PenaltyEnabled {
		t.Error("PenaltyEnabled should be true")
	}
	if !opt.HealthCheckEnabled {
		t.Error("HealthCheckEnabled should be true")
	}
	if !opt.CooldownEnabled {
		t.Error("CooldownEnabled should be true")
	}
	if !opt.StickyEnabled {
		t.Error("StickyEnabled should be true")
	}
	if opt.DefaultPreset != "budget" {
		t.Errorf("DefaultPreset = %q, want %q", opt.DefaultPreset, "budget")
	}
}

// ---------------------------------------------------------------------------
// ApplyComboSortPreset error cases
// ---------------------------------------------------------------------------

func TestApplyComboSortPreset_InvalidPreset(t *testing.T) {
	svc := newTestService(t)
	err := svc.ApplyComboSortPreset("any-combo", "invalid")
	if err == nil {
		t.Error("expected error for invalid preset")
	}
}

func TestApplyComboSortPreset_ComboNotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.ApplyComboSortPreset("nonexistent", "intelligence")
	if err == nil {
		t.Error("expected error for nonexistent combo")
	}
}

func TestApplyComboSortPreset_SingleStepDoesNothing(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "single-step",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}},
	}
	_ = svc.SaveModelCombo(combo)
	if err := svc.ApplyComboSortPreset("single-step", "intelligence"); err != nil {
		t.Fatalf("ApplyComboSortPreset: %v", err)
	}
	updated, _ := svc.GetModelCombo("single-step")
	if len(updated.Steps) != 1 || updated.Steps[0].Model != "gpt-4o" {
		t.Error("single-step combo should remain unchanged")
	}
}

// ---------------------------------------------------------------------------
// ResolveChannelsForModel with channels
// ---------------------------------------------------------------------------

func TestResolveChannelsForModel_WithExactMatch(t *testing.T) {
	svc := newTestService(t)
	ch := Channel{
		DisplayName: "exact-channel",
		BaseURL:     "https://exact.example.com",
		Enabled:     true,
		Models:      []string{"gpt-4o"},
	}
	_, _ = svc.SaveCustomChannel(ch)

	channels, model := svc.ResolveChannelsForModel("gpt-4o")
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	if channels[0].DisplayName != "exact-channel" {
		t.Errorf("channel name = %q, want %q", channels[0].DisplayName, "exact-channel")
	}
	if model != "gpt-4o" {
		t.Errorf("resolved model = %q, want %q", model, "gpt-4o")
	}
}

func TestResolveChannelsForModel_DisabledChannelExcluded(t *testing.T) {
	svc := newTestService(t)
	ch := Channel{
		DisplayName: "disabled-channel",
		BaseURL:     "https://disabled.example.com",
		Enabled:     false,
		Models:      []string{"gpt-4o"},
	}
	_, _ = svc.SaveCustomChannel(ch)

	channels, _ := svc.ResolveChannelsForModel("gpt-4o")
	if len(channels) != 0 {
		t.Errorf("expected 0 channels (disabled), got %d", len(channels))
	}
}

// ---------------------------------------------------------------------------
// ResolvePreferredChannel tests
// ---------------------------------------------------------------------------

func TestResolvePreferredChannel_NoChannels(t *testing.T) {
	svc := newTestService(t)
	_, ok := svc.ResolvePreferredChannel()
	if ok {
		t.Error("expected no preferred channel when no channels exist")
	}
}

func TestResolvePreferredChannel_DefaultChannel(t *testing.T) {
	svc := newTestService(t)
	id, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "default-channel",
		BaseURL:     "https://default.example.com",
		Enabled:     true,
		Models:      []string{"gpt-4o"},
	})

	svc.mu.Lock()
	svc.config.DefaultChannelID = id
	svc.mu.Unlock()

	ch, ok := svc.ResolvePreferredChannel()
	if !ok {
		t.Fatal("expected a preferred channel")
	}
	if ch.ChannelID != id {
		t.Errorf("channel ID = %q, want %q", ch.ChannelID, id)
	}
}

// ---------------------------------------------------------------------------
// SaveComboTemplate / DeleteComboTemplate / ListComboTemplates tests
// ---------------------------------------------------------------------------

func TestSaveAndDeleteComboTemplate(t *testing.T) {
	svc := newTestService(t)
	tmpl := ModelCombo{
		Name:     "test-template",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}},
	}
	if err := svc.SaveComboTemplate(tmpl); err != nil {
		t.Fatalf("SaveComboTemplate: %v", err)
	}

	templates := svc.ListComboTemplates()
	found := false
	for _, tpl := range templates {
		if tpl.Name == "test-template" {
			found = true
			break
		}
	}
	if !found {
		t.Error("template not found after save")
	}

	if err := svc.DeleteComboTemplate("test-template"); err != nil {
		t.Fatalf("DeleteComboTemplate: %v", err)
	}
	templates = svc.ListComboTemplates()
	for _, tpl := range templates {
		if tpl.Name == "test-template" {
			t.Error("template still exists after delete")
			break
		}
	}
}

// ---------------------------------------------------------------------------
// RenameComboTemplate tests
// ---------------------------------------------------------------------------

func TestRenameComboTemplate(t *testing.T) {
	svc := newTestService(t)
	tmpl := ModelCombo{
		Name:     "old-name",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}},
	}
	_ = svc.SaveComboTemplate(tmpl)

	if err := svc.RenameComboTemplate("old-name", "new-name"); err != nil {
		t.Fatalf("RenameComboTemplate: %v", err)
	}

	templates := svc.ListComboTemplates()
	foundOld, foundNew := false, false
	for _, tpl := range templates {
		if tpl.Name == "old-name" {
			foundOld = true
		}
		if tpl.Name == "new-name" {
			foundNew = true
		}
	}
	if foundOld {
		t.Error("old template name still exists after rename")
	}
	if !foundNew {
		t.Error("new template name not found after rename")
	}
}

// ---------------------------------------------------------------------------
// UpdateRoutingSettings tests
// ---------------------------------------------------------------------------

func TestUpdateRoutingSettings(t *testing.T) {
	svc := newTestService(t)
	if err := svc.UpdateRoutingSettings("round_robin", 5); err != nil {
		t.Fatalf("UpdateRoutingSettings: %v", err)
	}
	cfg := svc.GetConfig()
	if cfg.RoutingStrategy != "round_robin" {
		t.Errorf("RoutingStrategy = %q, want %q", cfg.RoutingStrategy, "round_robin")
	}
	if cfg.StickyChannelUse != 5 {
		t.Errorf("StickyChannelUse = %d, want 5", cfg.StickyChannelUse)
	}
}

func TestUpdateRoutingSettings_EmptyStrategyDefaults(t *testing.T) {
	svc := newTestService(t)
	_ = svc.UpdateRoutingSettings("", 0)
	cfg := svc.GetConfig()
	if cfg.RoutingStrategy != "fallback" {
		t.Errorf("empty strategy should default to fallback, got %q", cfg.RoutingStrategy)
	}
	if cfg.StickyChannelUse != 2 {
		t.Errorf("0 sticky uses should default to 2, got %d", cfg.StickyChannelUse)
	}
}

// ---------------------------------------------------------------------------
// SetDefaultModel / GetDefaultComboName / SetDefaultComboName tests
// ---------------------------------------------------------------------------

func TestSetDefaultModel(t *testing.T) {
	svc := newTestService(t)
	svc.SetDefaultModel("gpt-4o", "ch-123")
	cfg := svc.GetConfig()
	if cfg.DefaultModel != "gpt-4o" {
		t.Errorf("DefaultModel = %q, want %q", cfg.DefaultModel, "gpt-4o")
	}
	if cfg.DefaultChannelID != "ch-123" {
		t.Errorf("DefaultChannelID = %q, want %q", cfg.DefaultChannelID, "ch-123")
	}
}

func TestDefaultComboName(t *testing.T) {
	svc := newTestService(t)
	svc.SetDefaultComboName("my-combo")
	if got := svc.GetDefaultComboName(); got != "my-combo" {
		t.Errorf("GetDefaultComboName = %q, want %q", got, "my-combo")
	}
}

// ---------------------------------------------------------------------------
// SetOptimizationConfig tests
// ---------------------------------------------------------------------------

func TestSetOptimizationConfig(t *testing.T) {
	svc := newTestService(t)
	opt := OptimizationConfig{
		Mode:               "speed_first",
		PenaltyEnabled:     false,
		PenaltyDecaySec:    60,
		HealthCheckEnabled: false,
		HealthCheckSec:     600,
		HealthMaxFailures:  5,
		CooldownEnabled:    false,
		CooldownSec:        300,
		StickyEnabled:      false,
		StickyTTLSec:       3600,
		PresetEnabled:      false,
		DefaultPreset:      "intelligence",
	}
	if err := svc.SetOptimizationConfig(opt); err != nil {
		t.Fatalf("SetOptimizationConfig: %v", err)
	}
	got := svc.GetOptimizationConfig()
	if got.Mode != "speed_first" {
		t.Errorf("Mode = %q, want %q", got.Mode, "speed_first")
	}
	if got.DefaultPreset != "intelligence" {
		t.Errorf("DefaultPreset = %q, want %q", got.DefaultPreset, "intelligence")
	}
}

func TestSetOptimizationConfig_EmptyModeDefaults(t *testing.T) {
	svc := newTestService(t)
	opt := OptimizationConfig{}
	_ = svc.SetOptimizationConfig(opt)
	got := svc.GetOptimizationConfig()
	if got.Mode != "value_first" {
		t.Errorf("empty Mode should default to value_first, got %q", got.Mode)
	}
}

// ---------------------------------------------------------------------------
// Platform channel tests
// ---------------------------------------------------------------------------

func TestUpsertAndClearPlatformChannels(t *testing.T) {
	svc := newTestService(t)
	platformChs := []Channel{
		{
			ChannelID:   "plat-1",
			DisplayName: "platform-1",
			BaseURL:     "https://plat1.example.com",
			Enabled:     true,
			Models:      []string{"gpt-4o"},
		},
		{
			ChannelID:   "plat-2",
			DisplayName: "platform-2",
			BaseURL:     "https://plat2.example.com",
			Enabled:     true,
			Models:      []string{"claude-sonnet-5"},
		},
	}
	if err := svc.UpsertPlatformChannels(platformChs); err != nil {
		t.Fatalf("UpsertPlatformChannels: %v", err)
	}

	channels, _ := svc.ListChannels()
	if len(channels) != 2 {
		t.Fatalf("expected 2 platform channels, got %d", len(channels))
	}

	if err := svc.ClearPlatformChannels(); err != nil {
		t.Fatalf("ClearPlatformChannels: %v", err)
	}

	channels, _ = svc.ListChannels()
	if len(channels) != 0 {
		t.Errorf("expected 0 channels after clear, got %d", len(channels))
	}
}

// ---------------------------------------------------------------------------
// UpdateChannelHealth / UpdateChannelModels tests
// ---------------------------------------------------------------------------

func TestUpdateChannelHealth(t *testing.T) {
	svc := newTestService(t)
	id, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "health-test",
		BaseURL:     "https://health.example.com",
		Enabled:     true,
		Models:      []string{"gpt-4o"},
	})
	if err := svc.UpdateChannelHealth(id, "unhealthy"); err != nil {
		t.Fatalf("UpdateChannelHealth: %v", err)
	}
	ch, ok := svc.GetChannel(id)
	if !ok {
		t.Fatal("channel not found")
	}
	if ch.HealthStatus != "unhealthy" {
		t.Errorf("HealthStatus = %q, want %q", ch.HealthStatus, "unhealthy")
	}
}

func TestUpdateChannelModels(t *testing.T) {
	svc := newTestService(t)
	id, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "models-test",
		BaseURL:     "https://models.example.com",
		Enabled:     true,
		Models:      []string{"gpt-4o"},
	})
	if err := svc.UpdateChannelModels(id, []string{"claude-sonnet-5", "gpt-4o-mini"}); err != nil {
		t.Fatalf("UpdateChannelModels: %v", err)
	}
	ch, ok := svc.GetChannel(id)
	if !ok {
		t.Fatal("channel not found")
	}
	if len(ch.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(ch.Models))
	}
	if ch.Models[0] != "claude-sonnet-5" {
		t.Errorf("Models[0] = %q, want %q", ch.Models[0], "claude-sonnet-5")
	}
}

// ---------------------------------------------------------------------------
// FirstEnabledChannel tests
// ---------------------------------------------------------------------------

func TestFirstEnabledChannel(t *testing.T) {
	svc := newTestService(t)
	id1, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "disabled-ch",
		BaseURL:     "https://disabled.example.com",
		Enabled:     false,
		Models:      []string{"gpt-4o"},
	})
	id2, _ := svc.SaveCustomChannel(Channel{
		DisplayName: "enabled-ch",
		BaseURL:     "https://enabled.example.com",
		Enabled:     true,
		Models:      []string{"gpt-4o-mini"},
	})
	_ = id1 // unused, just ensures order

	ch, ok := svc.FirstEnabledChannel("")
	if !ok {
		t.Fatal("expected an enabled channel")
	}
	if ch.ChannelID != id2 {
		t.Errorf("expected first enabled channel ID = %q, got %q", id2, ch.ChannelID)
	}
}

func TestFirstEnabledChannel_NoEnabled(t *testing.T) {
	svc := newTestService(t)
	_, _ = svc.SaveCustomChannel(Channel{
		DisplayName: "disabled",
		BaseURL:     "https://disabled.example.com",
		Enabled:     false,
		Models:      []string{"gpt-4o"},
	})
	_, ok := svc.FirstEnabledChannel("")
	if ok {
		t.Error("expected no enabled channel")
	}
}

// ---------------------------------------------------------------------------
// SetAutoLaunch / SetAnalyticsEnabled / PlatformAPI tests
// ---------------------------------------------------------------------------

func TestSetAutoLaunch(t *testing.T) {
	svc := newTestService(t)
	_ = svc.SetAutoLaunch(true)
	cfg := svc.GetConfig()
	if !cfg.AutoLaunch {
		t.Error("AutoLaunch should be true")
	}
}

func TestSetAnalyticsEnabled(t *testing.T) {
	svc := newTestService(t)
	svc.SetAnalyticsEnabled(true)
	cfg := svc.GetConfig()
	if !cfg.AnalyticsEnabled {
		t.Error("AnalyticsEnabled should be true")
	}
}

func TestPlatformAPIBaseURL(t *testing.T) {
	svc := newTestService(t)
	if got := svc.GetPlatformAPIBaseURL(); got != "https://seasagi.seasx.ai/api/v1" {
		t.Errorf("default PlatformAPIBaseURL = %q, want %q", got, "https://seasagi.seasx.ai/api/v1")
	}
	_ = svc.SetPlatformAPIBaseURL("https://api.example.com")
	if got := svc.GetPlatformAPIBaseURL(); got != "https://api.example.com" {
		t.Errorf("PlatformAPIBaseURL = %q, want %q", got, "https://api.example.com")
	}
}

// ---------------------------------------------------------------------------
// FindComboByModel tests
// ---------------------------------------------------------------------------

func TestFindComboByModel(t *testing.T) {
	svc := newTestService(t)
	combo := ModelCombo{
		Name:     "findable-combo",
		Strategy: "fallback",
		Steps:    []ModelComboStep{{Model: "gpt-4o"}, {Model: "claude-sonnet-5"}},
	}
	_ = svc.SaveModelCombo(combo)

	found := svc.FindComboByModel("gpt-4o")
	if found == nil {
		t.Fatal("combo not found by model gpt-4o")
	}
	if found.Name != "findable-combo" {
		t.Errorf("found combo name = %q, want %q", found.Name, "findable-combo")
	}

	found = svc.FindComboByModel("claude-sonnet-5")
	if found == nil {
		t.Fatal("combo not found by model claude-sonnet-5")
	}
}

func TestFindComboByModel_NotFound(t *testing.T) {
	svc := newTestService(t)
	found := svc.FindComboByModel("nonexistent-model")
	if found != nil {
		t.Error("expected nil for nonexistent model")
	}
}

// ---------------------------------------------------------------------------
// SecurityConfig tests
// ---------------------------------------------------------------------------

func TestEnsureSecurityConfigDefaults(t *testing.T) {
	svc := &Service{
		config:   AppConfig{},
		channels: []Channel{},
		path:     filepath.Join(t.TempDir(), "config.json"),
	}
	svc.ensureSecurityConfig()

	if svc.config.Security == nil {
		t.Fatal("security config not initialized")
	}
	got := svc.GetSecurityConfig()
	if got.PromptInjectionAction != PromptInjectionLog {
		t.Errorf("PromptInjectionAction = %q, want %q", got.PromptInjectionAction, PromptInjectionLog)
	}
	if !got.ErrorSanitizeEnabled {
		t.Error("ErrorSanitizeEnabled should default to true")
	}
	if got.PIIMaskingEnabled {
		t.Error("PIIMaskingEnabled should default to false")
	}
}

func TestEnsureSecurityConfigFillsMissingAction(t *testing.T) {
	svc := &Service{
		config:   AppConfig{Security: &SecurityConfig{PIIMaskingEnabled: true}},
		channels: []Channel{},
		path:     filepath.Join(t.TempDir(), "config.json"),
	}
	svc.ensureSecurityConfig()

	if got := svc.GetSecurityConfig(); got.PromptInjectionAction != PromptInjectionLog {
		t.Errorf("PromptInjectionAction = %q, want %q", got.PromptInjectionAction, PromptInjectionLog)
	}
}

func TestGetSecurityConfigNilReturnsDefault(t *testing.T) {
	svc := &Service{config: AppConfig{}}
	if got := svc.GetSecurityConfig(); got != defaultSecurityConfig() {
		t.Errorf("nil security config should return defaults, got %+v", got)
	}
}

func TestSetSecurityConfigPersists(t *testing.T) {
	svc := &Service{
		config:   AppConfig{},
		channels: []Channel{},
		path:     filepath.Join(t.TempDir(), "config.json"),
	}
	if err := svc.SetSecurityConfig(SecurityConfig{PIIMaskingEnabled: true}); err != nil {
		t.Fatalf("SetSecurityConfig: %v", err)
	}
	got := svc.GetSecurityConfig()
	if !got.PIIMaskingEnabled {
		t.Error("PIIMaskingEnabled not persisted")
	}
	if got.PromptInjectionAction != PromptInjectionLog {
		t.Errorf("PromptInjectionAction = %q, want default %q", got.PromptInjectionAction, PromptInjectionLog)
	}
}

// ---------------------------------------------------------------------------
// RTK/Caveman 启用链路：默认值、持久化、开关守卫
// ---------------------------------------------------------------------------

// TestLoadOrDefaultFreshInstallDefaultsRTK 保证新鲜安装（无配置文件）时后端
// RTK 默认开启，与前端 SettingsPage 的 `rtk_enabled ?? true` 兜底一致；
// 否则 UI 显示开启而网关实际关闭，启用开关第一次点击不生效。
func TestLoadOrDefaultFreshInstallDefaultsRTK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := LoadOrDefault()
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}
	if !cfg.RTKEnabled {
		t.Error("fresh install should default RTKEnabled to true (must match frontend `?? true`)")
	}
	if cfg.RTKMaxOutputChars != 8000 {
		t.Errorf("fresh install RTKMaxOutputChars = %d, want 8000", cfg.RTKMaxOutputChars)
	}
	if cfg.CavemanEnabled {
		t.Error("fresh install should default CavemanEnabled to false (must match frontend `?? false`)")
	}
	if cfg.CavemanStyle != "concise" {
		t.Errorf("fresh install CavemanStyle = %q, want concise", cfg.CavemanStyle)
	}

	// 默认配置应已落盘，重启后一致。
	var state persistedState
	if err := json.Unmarshal(mustReadFile(t, defaultConfigPath()), &state); err != nil {
		t.Fatalf("unmarshal persisted config: %v", err)
	}
	if !state.Config.RTKEnabled {
		t.Error("persisted config should contain rtk_enabled=true")
	}
}

// TestLoadOrDefaultRespectsExistingRTKConfig 保证已有配置文件不被默认值覆盖：
// 显式关闭 RTK 的老用户升级后保持关闭。
func TestLoadOrDefaultRespectsExistingRTKConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	path := defaultConfigPath()
	existing := []byte(`{"config":{"rtk_enabled":false,"rtk_max_output_chars":123,"caveman_enabled":true,"caveman_style":"terse"}}`)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, existing, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadOrDefault()
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}
	if cfg.RTKEnabled {
		t.Error("existing rtk_enabled=false must not be overwritten by defaults")
	}
	if cfg.RTKMaxOutputChars != 123 {
		t.Errorf("RTKMaxOutputChars = %d, want persisted 123", cfg.RTKMaxOutputChars)
	}
	if !cfg.CavemanEnabled || cfg.CavemanStyle != "terse" {
		t.Errorf("Caveman config = (%v, %q), want persisted (true, terse)", cfg.CavemanEnabled, cfg.CavemanStyle)
	}
}

// TestSetRTKAndCavemanConfigTogglePersist 覆盖绑定层 SetRTKSettings 落到
// config 服务的语义：maxChars=0 / 空风格不覆盖既有值，且重启后读回一致。
func TestSetRTKAndCavemanConfigTogglePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	svc := NewServiceWithPath(AppConfig{}, path)

	// maxOutputChars<=0 应被忽略，不覆盖既有值。
	if err := svc.SetRTKConfig(true, 0); err != nil {
		t.Fatalf("SetRTKConfig(true, 0): %v", err)
	}
	if got := svc.GetConfig().RTKMaxOutputChars; got != 0 {
		t.Errorf("RTKMaxOutputChars = %d after zero pass, want unchanged 0", got)
	}
	if err := svc.SetRTKConfig(true, 2500); err != nil {
		t.Fatalf("SetRTKConfig(true, 2500): %v", err)
	}

	// 空风格应被忽略；随后切换开关不丢风格。
	if err := svc.SetCavemanConfig(true, ""); err != nil {
		t.Fatalf("SetCavemanConfig(true, \"\"): %v", err)
	}
	if got := svc.GetConfig().CavemanStyle; got != "" {
		t.Errorf("CavemanStyle = %q after empty pass, want unchanged \"\"", got)
	}
	if err := svc.SetCavemanConfig(true, "terse"); err != nil {
		t.Fatalf("SetCavemanConfig(true, terse): %v", err)
	}
	// 关闭 Caveman 不应清空已选风格（重新开启时无需重选）。
	if err := svc.SetCavemanConfig(false, ""); err != nil {
		t.Fatalf("SetCavemanConfig(false, \"\"): %v", err)
	}

	// 模拟重启：从磁盘重建，验证持久化往返。
	reloaded := NewServiceWithPath(AppConfig{}, path)
	got := reloaded.GetConfig()
	if !got.RTKEnabled || got.RTKMaxOutputChars != 2500 {
		t.Errorf("reloaded RTK = (%v, %d), want (true, 2500)", got.RTKEnabled, got.RTKMaxOutputChars)
	}
	if got.CavemanEnabled || got.CavemanStyle != "terse" {
		t.Errorf("reloaded Caveman = (%v, %q), want (false, terse)", got.CavemanEnabled, got.CavemanStyle)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
