package gateway

import (
	"net/http"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
)

func mitmHeader(target, path string) http.Header {
	h := http.Header{}
	h.Set(config.HeaderMITMSource, "1")
	h.Set(config.HeaderMITMTarget, target)
	h.Set(config.HeaderMITMEndpoint, path)
	return h
}

func TestResolveMITMRouteDisabledOrUnmarked(t *testing.T) {
	h := mitmHeader("cursor", "/v1/chat/completions")
	if got := resolveMITMRoute(h, config.MITMRoutingConfig{Enabled: false}, "gpt-4o"); got.Intercepted || got.Matched {
		t.Fatalf("disabled config should not intercept: %+v", got)
	}
	if got := resolveMITMRoute(http.Header{}, config.MITMRoutingConfig{Enabled: true}, "gpt-4o"); got.Intercepted {
		t.Fatalf("missing MITM header should not intercept")
	}
}

func TestResolveMITMRouteDefaultPassthrough(t *testing.T) {
	got := resolveMITMRoute(
		mitmHeader("cursor", "/v1/chat/completions"),
		config.MITMRoutingConfig{Enabled: true, DefaultAction: config.MITMActionPassThrough},
		"gpt-4o",
	)
	if !got.Intercepted {
		t.Fatal("expected intercepted")
	}
	if got.Matched || got.ResolvedTarget != "" {
		t.Fatalf("passthrough must not rewrite: %+v", got)
	}
}

func TestResolveMITMRouteComboAction(t *testing.T) {
	cfg := config.MITMRoutingConfig{
		Enabled:       true,
		DefaultAction: config.MITMActionPassThrough,
		Rules: []config.MITMRoutingRule{
			{ID: "r1", Enabled: true, Priority: 10, TargetID: "cursor", Action: config.MITMActionCombo, ComboName: "Coding-Combo"},
		},
	}
	got := resolveMITMRoute(mitmHeader("cursor", "/v1/chat/completions"), cfg, "gpt-4o")
	if !got.Matched || got.ResolvedTarget != "Coding-Combo" || got.RuleID != "r1" {
		t.Fatalf("unexpected decision: %+v", got)
	}
}

func TestResolveMITMRoutePriority(t *testing.T) {
	cfg := config.MITMRoutingConfig{
		Enabled:       true,
		DefaultAction: config.MITMActionPassThrough,
		Rules: []config.MITMRoutingRule{
			{ID: "low", Enabled: true, Priority: 5, TargetID: "cursor", Action: config.MITMActionCombo, ComboName: "Low"},
			{ID: "high", Enabled: true, Priority: 1, TargetID: "cursor", Action: config.MITMActionCombo, ComboName: "High"},
		},
	}
	got := resolveMITMRoute(mitmHeader("cursor", "/v1/chat/completions"), cfg, "gpt-4o")
	if got.ResolvedTarget != "High" {
		t.Fatalf("lower priority value must win, got %q", got.ResolvedTarget)
	}
}

func TestResolveMITMRouteDisabledRuleSkipped(t *testing.T) {
	cfg := config.MITMRoutingConfig{
		Enabled:       true,
		DefaultAction: config.MITMActionPassThrough,
		Rules: []config.MITMRoutingRule{
			{ID: "off", Enabled: false, Priority: 1, TargetID: "cursor", Action: config.MITMActionCombo, ComboName: "Off"},
		},
	}
	got := resolveMITMRoute(mitmHeader("cursor", "/x"), cfg, "gpt-4o")
	if got.Matched {
		t.Fatalf("disabled rule must be skipped: %+v", got)
	}
}

func TestMITMRuleMatchesDimensions(t *testing.T) {
	rule := config.MITMRoutingRule{
		Enabled:      true,
		TargetID:     "claude-code",
		EndpointPath: "/v1/messages",
		RequestModel: "claude-*",
	}
	if !mitmRuleMatches(rule, "claude-code", "/v1/messages", "claude-sonnet-5") {
		t.Fatal("wildcard model + matching target/path should match")
	}
	if mitmRuleMatches(rule, "cursor", "/v1/messages", "claude-sonnet-5") {
		t.Fatal("target mismatch must not match")
	}
	if mitmRuleMatches(rule, "claude-code", "/v1/other", "claude-sonnet-5") {
		t.Fatal("endpoint prefix mismatch must not match")
	}
	if mitmRuleMatches(rule, "claude-code", "/v1/messages", "gpt-4o") {
		t.Fatal("model mismatch must not match")
	}
}

func TestMITMRuleExactAndWildcardModel(t *testing.T) {
	exact := config.MITMRoutingRule{Enabled: true, RequestModel: "gpt-4o"}
	if !mitmRuleMatches(exact, "any", "/x", "gpt-4o") {
		t.Fatal("exact model should match")
	}
	if mitmRuleMatches(exact, "any", "/x", "gpt-4o-mini") {
		t.Fatal("exact model must not prefix-match")
	}
	wildcard := config.MITMRoutingRule{Enabled: true, RequestModel: "gpt-4*"}
	if !mitmRuleMatches(wildcard, "any", "/x", "gpt-4o") {
		t.Fatal("wildcard should match")
	}
}

func TestResolveMITMRouteDefaultCombo(t *testing.T) {
	cfg := config.MITMRoutingConfig{Enabled: true, DefaultAction: config.MITMActionCombo, DefaultCombo: "Fallback-Combo"}
	got := resolveMITMRoute(mitmHeader("unknown", "/x"), cfg, "gpt-4o")
	if !got.Matched || got.ResolvedTarget != "Fallback-Combo" || got.RuleID != "" {
		t.Fatalf("unexpected default decision: %+v", got)
	}
}

func TestResolveMITMRouteModelActionAndConstraints(t *testing.T) {
	cfg := config.MITMRoutingConfig{
		Enabled:       true,
		DefaultAction: config.MITMActionPassThrough,
		Rules: []config.MITMRoutingRule{
			{
				ID: "r", Enabled: true, TargetID: "*", RequestModel: "gpt-4*",
				Action:      config.MITMActionModel,
				Model:       "cheap-model",
				Constraints: map[string]any{"max_price": 1.0},
			},
		},
	}
	got := resolveMITMRoute(mitmHeader("cursor", "/x"), cfg, "gpt-4o")
	if got.ResolvedTarget != "cheap-model" || !got.Matched {
		t.Fatalf("unexpected decision: %+v", got)
	}
	if got.Constraints["max_price"] != 1.0 {
		t.Fatalf("constraints must be carried: %+v", got.Constraints)
	}
}

func TestSortedMITMRulesEnterpriseLast(t *testing.T) {
	rules := []config.MITMRoutingRule{
		{ID: "ent", Enabled: true, Priority: 1, Source: config.MITMRuleSourceEnterprise},
		{ID: "local", Enabled: true, Priority: 50, Source: config.MITMRuleSourceLocal},
	}
	got := sortedMITMRules(rules)
	if len(got) != 2 || got[0].ID != "local" || got[1].ID != "ent" {
		t.Fatalf("enterprise rules must be sorted last regardless of priority: %+v", got)
	}
}

func TestResolveMITMRouteHeaderMismatch(t *testing.T) {
	h := mitmHeader("cursor", "/v1/chat/completions")
	h.Set(config.HeaderMITMHost, "api.anthropic.com") // 实际属于 claude-code
	got := resolveMITMRoute(h, config.MITMRoutingConfig{Enabled: true, DefaultAction: config.MITMActionPassThrough}, "gpt-4o")
	if !got.HeaderMismatch {
		t.Fatalf("expected header mismatch, got %+v", got)
	}
}

func TestResolveMITMRouteHeaderConsistent(t *testing.T) {
	h := mitmHeader("claude-code", "/v1/messages")
	h.Set(config.HeaderMITMHost, "api.anthropic.com")
	got := resolveMITMRoute(h, config.MITMRoutingConfig{Enabled: true, DefaultAction: config.MITMActionPassThrough}, "claude-sonnet-5")
	if got.HeaderMismatch {
		t.Fatalf("consistent headers must not be flagged: %+v", got)
	}
}

func TestSplitMarketGrantSteps(t *testing.T) {
	plan := []routing.PlanStep{
		{Channel: config.Channel{ChannelID: config.MarketGrantChannelPrefix + "g1", Source: "market_grant"}},
		{Channel: config.Channel{ChannelID: "ch1"}},
		{Channel: config.Channel{ChannelID: config.MarketGrantChannelPrefix + "g2", Source: "market_grant"}},
	}
	grants, rest := splitMarketGrantSteps(plan)
	if len(grants) != 2 || len(rest) != 1 {
		t.Fatalf("unexpected split: grants=%d rest=%d", len(grants), len(rest))
	}
	if rest[0].Channel.ChannelID != "ch1" {
		t.Fatalf("non-grant step misplaced: %+v", rest[0])
	}
	if got := grantIDFromChannelID(config.MarketGrantChannelPrefix + "g1"); got != "g1" {
		t.Fatalf("grant id extraction failed: %q", got)
	}
}

func TestMarketFirstForCombo(t *testing.T) {
	off := false
	on := true
	cases := []struct {
		name  string
		combo config.ModelCombo
		ok    bool
		want  bool
	}{
		{"combo missing defaults on", config.ModelCombo{}, false, true},
		{"unset defaults on", config.ModelCombo{Name: "c"}, true, true},
		{"explicit on", config.ModelCombo{Name: "c", MarketPriority: &on}, true, true},
		{"explicit off", config.ModelCombo{Name: "c", MarketPriority: &off}, true, false},
	}
	for _, tc := range cases {
		if got := marketFirstForCombo(tc.combo, tc.ok); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
