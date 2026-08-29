package policy

import (
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func TestChannelSnapshot_Fields(t *testing.T) {
	s := ChannelSnapshot{
		ChannelID:    "test-channel",
		ProviderType: "test-provider",
		BaseURL:      "https://example.com",
		Enabled:      true,
		Models:       []string{"gpt-4", "gpt-3.5"},
		SortOrder:    1,
	}

	if s.ChannelID != "test-channel" {
		t.Errorf("ChannelID = %q, want %q", s.ChannelID, "test-channel")
	}
	if s.ProviderType != "test-provider" {
		t.Errorf("ProviderType = %q, want %q", s.ProviderType, "test-provider")
	}
	if s.BaseURL != "https://example.com" {
		t.Errorf("BaseURL = %q, want %q", s.BaseURL, "https://example.com")
	}
	if s.Enabled != true {
		t.Error("Enabled = false, want true")
	}
	if len(s.Models) != 2 || s.Models[0] != "gpt-4" || s.Models[1] != "gpt-3.5" {
		t.Errorf("Models = %v, want [gpt-4 gpt-3.5]", s.Models)
	}
	if s.SortOrder != 1 {
		t.Errorf("SortOrder = %d, want 1", s.SortOrder)
	}
}

func TestChannelSnapshot_JSONTags(t *testing.T) {
	// Verify json tags via reflection
	typ := reflect.TypeOf(ChannelSnapshot{})
	expectedTags := map[string]string{
		"ChannelID":    `json:"channel_id"`,
		"ProviderType": `json:"provider_type"`,
		"BaseURL":      `json:"base_url"`,
		"Enabled":      `json:"enabled"`,
		"Models":       `json:"models"`,
		"SortOrder":    `json:"sort_order"`,
	}

	for i := range typ.NumField() {
		field := typ.Field(i)
		expected, ok := expectedTags[field.Name]
		if !ok {
			t.Errorf("unexpected field %q", field.Name)
			continue
		}
		if got := string(field.Tag); got != expected {
			t.Errorf("field %q: tag = %q, want %q", field.Name, got, expected)
		}
	}

	// Verify JSON round-trip produces the same values
	original := ChannelSnapshot{
		ChannelID:    "ch-001",
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com",
		Enabled:      true,
		Models:       []string{"gpt-4"},
		SortOrder:    2,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ChannelSnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("JSON round-trip: got %+v, want %+v", decoded, original)
	}
}

func TestGetChannels_NilBeforeInit(t *testing.T) {
	// Ensure globalCache is nil
	globalCache = nil

	channels := GetChannels()
	if channels != nil {
		t.Errorf("GetChannels() = %v, want nil", channels)
	}
}

func TestGetChannels_ThreadSafe(t *testing.T) {
	// Reset and initialize cache
	globalCache = nil
	InitCache()

	// Populate channels directly
	globalCache.mu.Lock()
	globalCache.channels = []ChannelSnapshot{
		{ChannelID: "ch-1", ProviderType: "openai", BaseURL: "https://api.openai.com", Enabled: true, Models: []string{"gpt-4"}, SortOrder: 1},
		{ChannelID: "ch-2", ProviderType: "anthropic", BaseURL: "https://api.anthropic.com", Enabled: true, Models: []string{"claude-3"}, SortOrder: 2},
	}
	globalCache.mu.Unlock()

	var wg sync.WaitGroup
	numGoroutines := 100

	for range numGoroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			channels := GetChannels()
			if channels == nil {
				return
			}
			// Read all fields to exercise the read lock
			for _, ch := range channels {
				_, _ = ch.ChannelID, ch.ProviderType
			}
		}()
	}

	wg.Wait()

	// Verify data remains intact after concurrent reads
	channels := GetChannels()
	if len(channels) != 2 {
		t.Errorf("GetChannels() returned %d channels, want 2", len(channels))
	}
	if channels[0].ChannelID != "ch-1" {
		t.Errorf("channels[0].ChannelID = %q, want %q", channels[0].ChannelID, "ch-1")
	}
}
