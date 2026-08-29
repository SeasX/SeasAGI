package discovery

import (
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

// ---------------------------------------------------------------------------
// NewService
// ---------------------------------------------------------------------------

func TestNewService(t *testing.T) {
	cfgSvc := config.NewService(config.AppConfig{})
	svc := NewService(cfgSvc)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.configSvc != cfgSvc {
		t.Error("NewService did not store the provided configSvc")
	}
}

// ---------------------------------------------------------------------------
// DiscoveredModel struct fields
// ---------------------------------------------------------------------------

func TestDiscoveredModelFields(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	dm := DiscoveredModel{
		ChannelID:    "ch-test-1",
		ModelID:      "gpt-4o",
		ModelName:    "gpt-4o",
		DiscoveredAt: now,
	}

	if dm.ChannelID != "ch-test-1" {
		t.Errorf("ChannelID = %q, want %q", dm.ChannelID, "ch-test-1")
	}
	if dm.ModelID != "gpt-4o" {
		t.Errorf("ModelID = %q, want %q", dm.ModelID, "gpt-4o")
	}
	if dm.ModelName != "gpt-4o" {
		t.Errorf("ModelName = %q, want %q", dm.ModelName, "gpt-4o")
	}
	if dm.DiscoveredAt != now {
		t.Errorf("DiscoveredAt = %q, want %q", dm.DiscoveredAt, now)
	}
}

// ---------------------------------------------------------------------------
// DiscoveredModel field types
// ---------------------------------------------------------------------------

func TestDiscoveredModelFieldTypes(t *testing.T) {
	dm := DiscoveredModel{}

	// Verify all fields are strings by checking zero values.
	if dm.ChannelID != "" {
		t.Error("ChannelID should be string type (zero value is empty string)")
	}
	if dm.ModelID != "" {
		t.Error("ModelID should be string type (zero value is empty string)")
	}
	if dm.ModelName != "" {
		t.Error("ModelName should be string type (zero value is empty string)")
	}
	if dm.DiscoveredAt != "" {
		t.Error("DiscoveredAt should be string type (zero value is empty string)")
	}
}

// ---------------------------------------------------------------------------
// modelTime helper
// ---------------------------------------------------------------------------

func TestModelTimeFormat(t *testing.T) {
	ts := modelTime()
	_, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t.Errorf("modelTime() returned %q which is not RFC3339: %v", ts, err)
	}
}
