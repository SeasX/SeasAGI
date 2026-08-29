package device

import (
	"encoding/json"
	"testing"
)

func TestDevice_StructFields(t *testing.T) {
	d := Device{
		DeviceID:   "dev_123",
		UserID:     "user_456",
		DeviceName: "My MacBook",
		Platform:   "macos",
		BoundAt:    "2024-03-15T10:00:00Z",
	}

	if d.DeviceID != "dev_123" {
		t.Errorf("expected DeviceID field to be 'dev_123', got %q", d.DeviceID)
	}
	if d.UserID != "user_456" {
		t.Errorf("expected UserID field to be 'user_456', got %q", d.UserID)
	}
	if d.DeviceName != "My MacBook" {
		t.Errorf("expected DeviceName field to be 'My MacBook', got %q", d.DeviceName)
	}
	if d.Platform != "macos" {
		t.Errorf("expected Platform field to be 'macos', got %q", d.Platform)
	}
	if d.BoundAt != "2024-03-15T10:00:00Z" {
		t.Errorf("expected BoundAt field to be '2024-03-15T10:00:00Z', got %q", d.BoundAt)
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("failed to marshal Device: %v", err)
	}
	var unmarshalled Device
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal Device: %v", err)
	}
	if unmarshalled.DeviceID != d.DeviceID {
		t.Errorf("JSON round-trip failed for DeviceID: expected %q, got %q", d.DeviceID, unmarshalled.DeviceID)
	}
	if unmarshalled.UserID != d.UserID {
		t.Errorf("JSON round-trip failed for UserID: expected %q, got %q", d.UserID, unmarshalled.UserID)
	}
	if unmarshalled.DeviceName != d.DeviceName {
		t.Errorf("JSON round-trip failed for DeviceName: expected %q, got %q", d.DeviceName, unmarshalled.DeviceName)
	}
	if unmarshalled.Platform != d.Platform {
		t.Errorf("JSON round-trip failed for Platform: expected %q, got %q", d.Platform, unmarshalled.Platform)
	}
	if unmarshalled.BoundAt != d.BoundAt {
		t.Errorf("JSON round-trip failed for BoundAt: expected %q, got %q", d.BoundAt, unmarshalled.BoundAt)
	}
}

func TestBindRequest_StructFields(t *testing.T) {
	req := BindRequest{
		DeviceName: "iPhone 15",
		Platform:   "ios",
	}

	if req.DeviceName != "iPhone 15" {
		t.Errorf("expected DeviceName field to be 'iPhone 15', got %q", req.DeviceName)
	}
	if req.Platform != "ios" {
		t.Errorf("expected Platform field to be 'ios', got %q", req.Platform)
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal BindRequest: %v", err)
	}
	var unmarshalled BindRequest
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal BindRequest: %v", err)
	}
	if unmarshalled.DeviceName != req.DeviceName {
		t.Errorf("JSON round-trip failed for DeviceName: expected %q, got %q", req.DeviceName, unmarshalled.DeviceName)
	}
	if unmarshalled.Platform != req.Platform {
		t.Errorf("JSON round-trip failed for Platform: expected %q, got %q", req.Platform, unmarshalled.Platform)
	}
}

func TestDevice_JSONTags(t *testing.T) {
	d := Device{
		DeviceID:   "dev_789",
		UserID:     "user_012",
		DeviceName: "Pixel 9",
		Platform:   "android",
		BoundAt:    "2024-05-20T08:30:00Z",
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("failed to marshal Device: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal into map: %v", err)
	}

	expectedKeys := []string{"device_id", "user_id", "device_name", "platform", "bound_at"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON key %q not found in marshalled output", key)
		}
	}

	// Verify the exact values
	if raw["device_id"] != "dev_789" {
		t.Errorf("expected JSON device_id to be 'dev_789', got %v", raw["device_id"])
	}
	if raw["user_id"] != "user_012" {
		t.Errorf("expected JSON user_id to be 'user_012', got %v", raw["user_id"])
	}
	if raw["device_name"] != "Pixel 9" {
		t.Errorf("expected JSON device_name to be 'Pixel 9', got %v", raw["device_name"])
	}
	if raw["platform"] != "android" {
		t.Errorf("expected JSON platform to be 'android', got %v", raw["platform"])
	}
	if raw["bound_at"] != "2024-05-20T08:30:00Z" {
		t.Errorf("expected JSON bound_at to be '2024-05-20T08:30:00Z', got %v", raw["bound_at"])
	}
}