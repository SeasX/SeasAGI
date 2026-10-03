package mitm

import (
	"path/filepath"
	"testing"
)

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
	if r.Count() != len(defaultRuleDomains) {
		t.Errorf("count: got %d, want %d", r.Count(), len(defaultRuleDomains))
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

func TestRulesPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	storePath := filepath.Join(dir, "mitm_rules.json")

	r := NewRulesWithBase([]string{"api.base.com"})
	r.SetStorePath(storePath)
	r.Add("api.custom.com")
	r.Remove("api.base.com")
	if err := r.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 新实例载入后应保持用户的增删
	r2 := NewRulesWithBase([]string{"api.base.com"})
	r2.SetStorePath(storePath)
	if err := r2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !r2.Match("api.custom.com") {
		t.Error("added domain should persist across restart")
	}
	if r2.Match("api.base.com") {
		t.Error("removed domain should stay removed across restart")
	}
	if r2.Count() != 1 {
		t.Errorf("count after load: got %d, want 1", r2.Count())
	}
}

func TestRulesLoadMissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	r := NewRulesWithBase([]string{"api.base.com"})
	r.SetStorePath(filepath.Join(dir, "does-not-exist.json"))
	if err := r.Load(); err != nil {
		t.Fatalf("Load on missing file should be nil, got %v", err)
	}
	if r.Count() != 1 {
		t.Errorf("count: got %d, want 1", r.Count())
	}
}

func TestRulesSaveNoStorePathIsNoop(t *testing.T) {
	r := NewRules()
	r.Add("api.custom.com")
	if err := r.Save(); err != nil {
		t.Fatalf("Save without store path should be nil, got %v", err)
	}
	if !r.Match("api.custom.com") {
		t.Error("in-memory add should still apply")
	}
}

func TestRulesMatchIsCaseInsensitive(t *testing.T) {
	r := NewRules()
	r.Add("API.Custom.COM")
	if !r.Match("api.custom.com") {
		t.Error("Match should be case-insensitive")
	}
	if !r.Match("  API.Custom.com  ") {
		t.Error("Match should trim surrounding whitespace")
	}
}

func TestDefaultBaseDomainsIncludesSupportedTargetHosts(t *testing.T) {
	base := DefaultBaseDomains()
	set := make(map[string]bool, len(base))
	for _, d := range base {
		if d != normalizeDomain(d) {
			t.Errorf("baseline domain %q is not normalized", d)
		}
		if set[d] {
			t.Errorf("baseline domain %q duplicated", d)
		}
		set[d] = true
	}
	// 所有 viability=supported 目标的 Hosts 必须落在基线内（展示与拦截一致）
	for _, tg := range MergedTargets() {
		if tg.Viability != "supported" {
			continue
		}
		for _, h := range tg.Hosts {
			if !set[normalizeDomain(h)] {
				t.Errorf("supported target host %q missing from baseline domains", h)
			}
		}
	}
}
