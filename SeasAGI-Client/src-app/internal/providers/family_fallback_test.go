package providers

import "testing"

func TestFamilyFallbackClaude(t *testing.T) {
	m := NewFamilyFallbackMap()
	fallbacks := m.GetFallbacks("claude-3-opus")
	if len(fallbacks) == 0 {
		t.Fatal("expected fallbacks for claude-3-opus")
	}
	if fallbacks[0] != "claude-3-5-sonnet" {
		t.Errorf("expected first fallback claude-3-5-sonnet, got %s", fallbacks[0])
	}
}

func TestFamilyFallbackGPT(t *testing.T) {
	m := NewFamilyFallbackMap()
	fallbacks := m.GetFallbacks("gpt-4")
	if len(fallbacks) == 0 {
		t.Fatal("expected fallbacks for gpt-4")
	}
	if fallbacks[0] != "gpt-4-turbo" {
		t.Errorf("expected first fallback gpt-4-turbo, got %s", fallbacks[0])
	}
}

func TestFamilyFallbackGemini(t *testing.T) {
	m := NewFamilyFallbackMap()
	fallbacks := m.GetFallbacks("gemini-1.5-pro")
	if len(fallbacks) == 0 {
		t.Fatal("expected fallbacks for gemini-1.5-pro")
	}
	if fallbacks[0] != "gemini-1.5-flash" {
		t.Errorf("expected first fallback gemini-1.5-flash, got %s", fallbacks[0])
	}
}

func TestFamilyFallbackCaseInsensitive(t *testing.T) {
	m := NewFamilyFallbackMap()
	fallbacks := m.GetFallbacks("GPT-4O")
	if len(fallbacks) == 0 {
		t.Fatal("expected fallbacks for GPT-4O (case insensitive)")
	}
}

func TestFamilyFallbackUnknown(t *testing.T) {
	m := NewFamilyFallbackMap()
	fallbacks := m.GetFallbacks("unknown-model")
	if fallbacks != nil {
		t.Errorf("expected nil for unknown model, got %v", fallbacks)
	}
}

func TestFamilyFallbackFindClosestMatch(t *testing.T) {
	m := NewFamilyFallbackMap()
	available := []string{"claude-3-5-sonnet", "gpt-4o", "gemini-1.5-flash"}
	match := m.FindClosestMatch("claude-3-opus", available)
	if match != "claude-3-5-sonnet" {
		t.Errorf("expected claude-3-5-sonnet, got %s", match)
	}
}

func TestFamilyFallbackFindClosestMatchNone(t *testing.T) {
	m := NewFamilyFallbackMap()
	available := []string{"gpt-4o", "gemini-1.5-flash"}
	match := m.FindClosestMatch("claude-3-opus", available)
	if match != "" {
		t.Errorf("expected empty match, got %s", match)
	}
}

func TestFamilyFallbackAddCustom(t *testing.T) {
	m := NewFamilyFallbackMap()
	m.AddFamily("custom-model", []string{"fallback-1", "fallback-2"})
	fallbacks := m.GetFallbacks("custom-model")
	if len(fallbacks) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d", len(fallbacks))
	}
	if fallbacks[0] != "fallback-1" {
		t.Errorf("expected fallback-1, got %s", fallbacks[0])
	}
}

func TestFamilyFallbackHasFamily(t *testing.T) {
	m := NewFamilyFallbackMap()
	if !m.HasFamily("gpt-4") {
		t.Error("expected HasFamily true for gpt-4")
	}
	if m.HasFamily("unknown") {
		t.Error("expected HasFamily false for unknown")
	}
}
