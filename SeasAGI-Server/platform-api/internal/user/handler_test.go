package user

import (
	"encoding/json"
	"testing"
)

func TestUserProfile_StructFields(t *testing.T) {
	p := UserProfile{
		UserID:    "user_123",
		Email:     "user@example.com",
		CreatedAt: "2024-01-01T00:00:00Z",
		UpdatedAt: "2024-06-01T00:00:00Z",
	}

	if p.UserID != "user_123" {
		t.Errorf("expected UserID field to be 'user_123', got %q", p.UserID)
	}
	if p.Email != "user@example.com" {
		t.Errorf("expected Email field to be 'user@example.com', got %q", p.Email)
	}
	if p.CreatedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("expected CreatedAt field to be '2024-01-01T00:00:00Z', got %q", p.CreatedAt)
	}
	if p.UpdatedAt != "2024-06-01T00:00:00Z" {
		t.Errorf("expected UpdatedAt field to be '2024-06-01T00:00:00Z', got %q", p.UpdatedAt)
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal UserProfile: %v", err)
	}
	var unmarshalled UserProfile
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal UserProfile: %v", err)
	}
	if unmarshalled.UserID != p.UserID {
		t.Errorf("JSON round-trip failed for UserID: expected %q, got %q", p.UserID, unmarshalled.UserID)
	}
	if unmarshalled.Email != p.Email {
		t.Errorf("JSON round-trip failed for Email: expected %q, got %q", p.Email, unmarshalled.Email)
	}
	if unmarshalled.CreatedAt != p.CreatedAt {
		t.Errorf("JSON round-trip failed for CreatedAt: expected %q, got %q", p.CreatedAt, unmarshalled.CreatedAt)
	}
	if unmarshalled.UpdatedAt != p.UpdatedAt {
		t.Errorf("JSON round-trip failed for UpdatedAt: expected %q, got %q", p.UpdatedAt, unmarshalled.UpdatedAt)
	}
}

func TestUserProfile_JSONTags(t *testing.T) {
	p := UserProfile{
		UserID:    "user_001",
		Email:     "json@example.com",
		CreatedAt: "2024-01-15T10:30:00Z",
		UpdatedAt: "2024-07-20T12:00:00Z",
	}

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal UserProfile: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal into map: %v", err)
	}

	expectedKeys := []string{"user_id", "email", "created_at", "updated_at"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q not found in marshalled output", key)
		}
	}

	// Verify the exact values
	if raw["user_id"] != "user_001" {
		t.Errorf("expected JSON user_id to be 'user_001', got %v", raw["user_id"])
	}
	if raw["email"] != "json@example.com" {
		t.Errorf("expected JSON email to be 'json@example.com', got %v", raw["email"])
	}
	if raw["created_at"] != "2024-01-15T10:30:00Z" {
		t.Errorf("expected JSON created_at to be '2024-01-15T10:30:00Z', got %v", raw["created_at"])
	}
	if raw["updated_at"] != "2024-07-20T12:00:00Z" {
		t.Errorf("expected JSON updated_at to be '2024-07-20T12:00:00Z', got %v", raw["updated_at"])
	}
}
