package mitm

import "testing"

func TestDefaultRules(t *testing.T) {
	r := NewDefaultRules()
	expected := []string{
		"api.openai.com",
		"api.anthropic.com",
		"generativelanguage.googleapis.com",
		"api.deepseek.com",
		"api.x.ai",
		"openrouter.ai",
	}
	for _, d := range expected {
		if !r.Match(d) {
			t.Errorf("default rule %q should match", d)
		}
	}
	if r.Count() != len(expected) {
		t.Errorf("count: got %d, want %d", r.Count(), len(expected))
	}
}

func TestRulesMatchNonDefault(t *testing.T) {
	r := NewDefaultRules()
	if r.Match("google.com") {
		t.Error("google.com should not match default rules")
	}
	if r.Match("github.com") {
		t.Error("github.com should not match default rules")
	}
}

func TestRulesAddRemove(t *testing.T) {
	r := NewRules()

	// Add
	r.Add("api.test.com")
	if !r.Match("api.test.com") {
		t.Error("Add did not make domain match")
	}
	if r.Count() != 1 {
		t.Errorf("count after Add: got %d, want 1", r.Count())
	}

	// Add again (幂等)
	r.Add("api.test.com")
	if r.Count() != 1 {
		t.Errorf("count after duplicate Add: got %d, want 1", r.Count())
	}

	// Remove
	r.Remove("api.test.com")
	if r.Match("api.test.com") {
		t.Error("Remove did not unmatch domain")
	}
	if r.Count() != 0 {
		t.Errorf("count after Remove: got %d, want 0", r.Count())
	}

	// Remove again (幂等)
	r.Remove("api.test.com")
	if r.Count() != 0 {
		t.Errorf("count after duplicate Remove: got %d, want 0", r.Count())
	}
}

func TestRulesList(t *testing.T) {
	r := NewRules()
	r.Add("a.com")
	r.Add("b.com")
	r.Add("c.com")

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List len: got %d, want 3", len(list))
	}

	// 验证返回的是副本（修改不影响内部状态）
	list[0] = "modified.com"
	if r.Match("modified.com") {
		t.Error("List returned internal slice, not a copy")
	}
	if !r.Match("a.com") {
		t.Error("original domain should still match")
	}
}
