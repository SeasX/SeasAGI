package mitm

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStatusJSONRoundTrip(t *testing.T) {
	original := Status{
		State:       StateRunning,
		ProxyPort:   8080,
		CAInstalled: true,
		RulesCount:  6,
		SystemProxy: true,
		LastError:   "",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Status
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestStatusJSONLastErrorOmit(t *testing.T) {
	s := Status{State: StateError, LastError: ""}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// last_error 为空时应被 omitempty 省略
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if _, ok := m["last_error"]; ok {
		t.Error("last_error should be omitted when empty")
	}
}

func TestInterceptEntryJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	original := InterceptEntry{
		Time:        ts,
		Method:      "POST",
		Host:        "api.openai.com",
		Path:        "/v1/chat/completions",
		Status:      200,
		Intercepted: true,
		DurationMs:  123.45,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded InterceptEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Time.Equal(ts) {
		t.Errorf("time mismatch: got %v, want %v", decoded.Time, ts)
	}
	if decoded.Method != original.Method || decoded.Host != original.Host ||
		decoded.Path != original.Path || decoded.Status != original.Status ||
		decoded.Intercepted != original.Intercepted || decoded.DurationMs != original.DurationMs {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, original)
	}
}

func TestStateStringValues(t *testing.T) {
	cases := map[State]string{
		StateStopped:  "stopped",
		StateStarting: "starting",
		StateRunning:  "running",
		StateStopping: "stopping",
		StateError:    "error",
	}
	for state, expected := range cases {
		if string(state) != expected {
			t.Errorf("State %s: got %q, want %q", state, string(state), expected)
		}
	}
}
