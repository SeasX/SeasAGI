package presets

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestNewService(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
	if svc.presets == nil {
		t.Fatal("NewService() presets is nil")
	}
}

func TestCount(t *testing.T) {
	svc := NewService()
	got := svc.Count()
	// Verify the service has at least some presets loaded
	if got == 0 {
		t.Fatal("Count() returned 0, expected non-zero")
	}
}

func TestListPresets(t *testing.T) {
	svc := NewService()
	presets := svc.ListPresets()
	if len(presets) == 0 {
		t.Fatal("ListPresets() returned empty slice")
	}
	if len(presets) != svc.Count() {
		t.Errorf("ListPresets() returned %d presets, Count() = %d", len(presets), svc.Count())
	}
}

func TestListByCategory_Official(t *testing.T) {
	svc := NewService()
	presets := svc.ListByCategory("official")
	if len(presets) == 0 {
		t.Error("ListByCategory('official') returned empty, expected non-empty")
	}
}

func TestListByCategory_Local(t *testing.T) {
	svc := NewService()
	presets := svc.ListByCategory("local")
	if len(presets) == 0 {
		t.Error("ListByCategory('local') returned empty, expected non-empty")
	}
}

func TestListByCategory_Relay(t *testing.T) {
	svc := NewService()
	presets := svc.ListByCategory("relay")
	if len(presets) == 0 {
		t.Error("ListByCategory('relay') returned empty, expected non-empty")
	}
}

func TestListByCategory_Unknown(t *testing.T) {
	svc := NewService()
	presets := svc.ListByCategory("nonexistent-category")
	if len(presets) != 0 {
		t.Errorf("ListByCategory('nonexistent-category') returned %d presets, expected 0", len(presets))
	}
}

func TestListByRegion_US(t *testing.T) {
	svc := NewService()
	presets := svc.ListByRegion("us")
	if len(presets) == 0 {
		t.Error("ListByRegion('us') returned empty, expected non-empty")
	}
}

func TestListByRegion_CN(t *testing.T) {
	svc := NewService()
	presets := svc.ListByRegion("cn")
	if len(presets) == 0 {
		t.Error("ListByRegion('cn') returned empty, expected non-empty")
	}
}

func TestListByRegion_Local(t *testing.T) {
	svc := NewService()
	presets := svc.ListByRegion("local")
	if len(presets) == 0 {
		t.Error("ListByRegion('local') returned empty, expected non-empty")
	}
}

func TestListByRegion_Unknown(t *testing.T) {
	svc := NewService()
	presets := svc.ListByRegion("nonexistent-region")
	if len(presets) != 0 {
		t.Errorf("ListByRegion('nonexistent-region') returned %d presets, expected 0", len(presets))
	}
}

func TestGetByName_Known(t *testing.T) {
	svc := NewService()
	preset := svc.GetByName("openai-official")
	if preset == nil {
		t.Fatal("GetByName('openai-official') returned nil, expected a preset")
	}
	if preset.Name != "openai-official" {
		t.Errorf("GetByName('openai-official').Name = %q, want 'openai-official'", preset.Name)
	}
	if preset.ProviderType != "openai" {
		t.Errorf("GetByName('openai-official').ProviderType = %q, want 'openai'", preset.ProviderType)
	}
	if preset.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("GetByName('openai-official').BaseURL = %q, want 'https://api.openai.com/v1'", preset.BaseURL)
	}
	if preset.Region != "us" {
		t.Errorf("GetByName('openai-official').Region = %q, want 'us'", preset.Region)
	}
	if preset.Category != "official" {
		t.Errorf("GetByName('openai-official').Category = %q, want 'official'", preset.Category)
	}
}

func TestGetByName_Unknown(t *testing.T) {
	svc := NewService()
	preset := svc.GetByName("nonexistent-preset")
	if preset != nil {
		t.Errorf("GetByName('nonexistent-preset') returned %+v, expected nil", preset)
	}
}

func TestImportCustom_AddsPreset(t *testing.T) {
	svc := NewService()
	initialCount := svc.Count()

	customPreset := ProviderPreset{
		Name:         "custom-test-provider",
		ProviderType: "openai",
		BaseURL:      "https://custom.example.com/v1",
		DisplayName:  "Custom Test",
		Region:       "us",
		Category:     "official",
		Models:       []string{"test-model"},
		ExtraConfig:  map[string]string{"key": "value"},
	}
	data, err := json.Marshal(customPreset)
	if err != nil {
		t.Fatalf("Failed to marshal custom preset: %v", err)
	}

	err = svc.ImportCustom(data)
	if err != nil {
		t.Fatalf("ImportCustom() returned error: %v", err)
	}

	newCount := svc.Count()
	if newCount != initialCount+1 {
		t.Errorf("Count after ImportCustom = %d, want %d (initial %d + 1)", newCount, initialCount+1, initialCount)
	}

	imported := svc.GetByName("custom-test-provider")
	if imported == nil {
		t.Fatal("GetByName('custom-test-provider') returned nil after import")
	}
	if imported.BaseURL != "https://custom.example.com/v1" {
		t.Errorf("Imported preset BaseURL = %q, want 'https://custom.example.com/v1'", imported.BaseURL)
	}
	if imported.DisplayName != "Custom Test" {
		t.Errorf("Imported preset DisplayName = %q, want 'Custom Test'", imported.DisplayName)
	}
}

func TestImportCustom_UpdatesExisting(t *testing.T) {
	svc := NewService()
	initialCount := svc.Count()

	updatedPreset := ProviderPreset{
		Name:         "openai-official",
		ProviderType: "openai",
		BaseURL:      "https://updated.example.com/v1",
		DisplayName:  "Updated OpenAI",
		Region:       "us",
		Category:     "official",
	}
	data, err := json.Marshal(updatedPreset)
	if err != nil {
		t.Fatalf("Failed to marshal updated preset: %v", err)
	}

	err = svc.ImportCustom(data)
	if err != nil {
		t.Fatalf("ImportCustom() returned error: %v", err)
	}

	newCount := svc.Count()
	if newCount != initialCount {
		t.Errorf("Count after updating existing preset = %d, want %d (unchanged)", newCount, initialCount)
	}

	imported := svc.GetByName("openai-official")
	if imported == nil {
		t.Fatal("GetByName('openai-official') returned nil after update")
	}
	if imported.BaseURL != "https://updated.example.com/v1" {
		t.Errorf("Updated preset BaseURL = %q, want 'https://updated.example.com/v1'", imported.BaseURL)
	}
	if imported.DisplayName != "Updated OpenAI" {
		t.Errorf("Updated preset DisplayName = %q, want 'Updated OpenAI'", imported.DisplayName)
	}
}

func TestImportCustom_InvalidJSON(t *testing.T) {
	svc := NewService()
	err := svc.ImportCustom([]byte("invalid json"))
	if err == nil {
		t.Error("ImportCustom with invalid JSON should return error, got nil")
	}
}

func TestConcurrentReadSafety(t *testing.T) {
	svc := NewService()
	var wg sync.WaitGroup

	// Launch concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.Count()
			_ = svc.ListPresets()
			_ = svc.ListByCategory("official")
			_ = svc.ListByRegion("us")
			_ = svc.GetByName("openai-official")
		}()
	}

	// Launch concurrent writer
	wg.Add(1)
	go func() {
		defer wg.Done()
		preset := ProviderPreset{
			Name:         "concurrent-test",
			ProviderType: "openai",
			BaseURL:      "https://concurrent.example.com/v1",
			DisplayName:  "Concurrent Test",
			Region:       "us",
			Category:     "official",
		}
		data, _ := json.Marshal(preset)
		_ = svc.ImportCustom(data)
	}()

	wg.Wait()
	// If we get here without race condition or deadlock, the test passes
	preset := svc.GetByName("concurrent-test")
	if preset == nil {
		t.Error("GetByName('concurrent-test') returned nil after concurrent import")
	}
}