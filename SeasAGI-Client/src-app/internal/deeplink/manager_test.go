package deeplink

import (
	"encoding/json"
	"testing"
)

func TestParseImportProvider(t *testing.T) {
	m := NewManager()
	raw := BuildImportProviderURL("my provider", "openai", "https://api.example.com/v1", "sk-123")
	action, err := m.Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if action.Type != ActionImportProvider {
		t.Fatalf("expected type %q, got %q", ActionImportProvider, action.Type)
	}
	want := map[string]string{
		"name":     "my provider",
		"type":     "openai",
		"base_url": "https://api.example.com/v1",
		"api_key":  "sk-123",
	}
	for k, v := range want {
		if action.Params[k] != v {
			t.Errorf("param %q: expected %q, got %q", k, v, action.Params[k])
		}
	}
}

func TestParseRejectsUnsupportedScheme(t *testing.T) {
	m := NewManager()
	if _, err := m.Parse("https://import-provider?a=1"); err == nil {
		t.Fatal("expected error for non-seasagi scheme")
	}
}

func TestParseInvalidURL(t *testing.T) {
	m := NewManager()
	if _, err := m.Parse("://bad"); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestParseCapturesFragment(t *testing.T) {
	m := NewManager()
	action, err := m.Parse("seasagi://open-channel#team=acme")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if action.Type != ActionOpenChannel {
		t.Fatalf("expected type %q, got %q", ActionOpenChannel, action.Type)
	}
	if action.Params["_fragment"] != "team=acme" {
		t.Fatalf("expected fragment captured, got %q", action.Params["_fragment"])
	}
}

func TestBuildImportMCPURLWithArgs(t *testing.T) {
	m := NewManager()
	args := []string{"--flag", "value with space"}
	raw := BuildImportMCPURL("mcp", "npx", "stdio", args)
	action, err := m.Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if action.Type != ActionImportMCP {
		t.Fatalf("expected type %q, got %q", ActionImportMCP, action.Type)
	}
	if action.Params["name"] != "mcp" || action.Params["command"] != "npx" || action.Params["transport"] != "stdio" {
		t.Fatalf("unexpected params: %+v", action.Params)
	}
	var decoded []string
	if err := json.Unmarshal([]byte(action.Params["args"]), &decoded); err != nil {
		t.Fatalf("args not valid JSON: %v", err)
	}
	if len(decoded) != 2 || decoded[0] != args[0] || decoded[1] != args[1] {
		t.Fatalf("expected args %v, got %v", args, decoded)
	}
}

func TestBuildImportPromptAndSkillURL(t *testing.T) {
	m := NewManager()
	prompt, err := m.Parse(BuildImportPromptURL("p", "claude"))
	if err != nil {
		t.Fatalf("prompt parse error: %v", err)
	}
	if prompt.Type != ActionImportPrompt || prompt.Params["app"] != "claude" {
		t.Fatalf("unexpected prompt action: %+v", prompt)
	}
	skill, err := m.Parse(BuildImportSkillURL("s", "https://github.com/x/y"))
	if err != nil {
		t.Fatalf("skill parse error: %v", err)
	}
	if skill.Type != ActionImportSkill || skill.Params["repo"] != "https://github.com/x/y" {
		t.Fatalf("unexpected skill action: %+v", skill)
	}
}

func TestHandleDispatchesToRegisteredHandler(t *testing.T) {
	m := NewManager()
	var got DeepLinkAction
	m.RegisterHandler(ActionImportProvider, func(a DeepLinkAction) error {
		got = a
		return nil
	})
	if err := m.Handle(BuildImportProviderURL("n", "t", "u", "k")); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if got.Type != ActionImportProvider || got.Params["name"] != "n" {
		t.Fatalf("handler received wrong action: %+v", got)
	}
}

func TestHandleWithoutHandler(t *testing.T) {
	m := NewManager()
	if err := m.Handle("seasagi://open-channel"); err == nil {
		t.Fatal("expected error when no handler registered")
	}
}

func TestEnqueueAndProcessPending(t *testing.T) {
	m := NewManager()
	count := 0
	m.RegisterHandler(ActionOpenChannel, func(DeepLinkAction) error {
		count++
		return nil
	})
	if err := m.Enqueue("seasagi://open-channel?a=1"); err != nil {
		t.Fatalf("Enqueue error: %v", err)
	}
	m.ProcessPending()
	if count != 1 {
		t.Fatalf("expected handler called once, got %d", count)
	}
	// Second drain is a no-op.
	m.ProcessPending()
	if count != 1 {
		t.Fatalf("expected no additional calls, got %d", count)
	}
}

func TestEnqueueOverflow(t *testing.T) {
	m := NewManager()
	for i := 0; i < 16; i++ {
		if err := m.Enqueue("seasagi://open-channel"); err != nil {
			t.Fatalf("Enqueue %d unexpectedly failed: %v", i, err)
		}
	}
	if err := m.Enqueue("seasagi://open-channel"); err == nil {
		t.Fatal("expected queue full error on 17th enqueue")
	}
}

func TestEnqueueInvalidURL(t *testing.T) {
	m := NewManager()
	if err := m.Enqueue("https://x"); err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}
