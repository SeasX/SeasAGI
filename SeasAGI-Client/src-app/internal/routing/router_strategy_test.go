package routing

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

// ---------------------------------------------------------------------------
// NormalizeStrategyPref：UI/云端别名 → 路由打分标准形（修复策略拼写断层）
// ---------------------------------------------------------------------------

func TestNormalizeStrategyPref(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"cost_first", "cost-first"},
		{"COST_FIRST", "cost-first"},
		{" speed_first ", "speed-first"},
		{"stable_first", "stability-first"},
		{"stable-first", "stability-first"},
		{"stability-first", "stability-first"},
		{"quality_first", "quality-first"},
		{"quality-first", "quality-first"},
		{"balanced", "balanced"},
		{"tools_first", "tools-first"},
		{"cloud-only-alias", "cloud-only-alias"}, // 未知值规范化后原样返回（视为均衡）
	}
	for _, tc := range cases {
		if got := NormalizeStrategyPref(tc.in); got != tc.want {
			t.Errorf("NormalizeStrategyPref(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// UI 实际产出的别名必须真实影响路由排序（回归：此前 cost_first/stable_first/
// speed_first 因拼写断层从不生效）
// ---------------------------------------------------------------------------

func TestApplyIntentRouting_UIStrategyAliases(t *testing.T) {
	r := &Resolver{}

	// cost_first（UI 下划线拼写）+ code_logic：DeepSeek 应排到 GPT-4o 之前（+35+15 vs 0）
	steps := stepsFor("gpt-4o", "deepseek-chat")
	ordered, err := r.applyIntentRouting(steps, &IntentContext{
		Scenario: "code_logic", RequiredIQ: "balanced", SecurityLevel: "public",
	}, "cost_first")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "deepseek") {
		t.Errorf("cost_first: first = %q, want deepseek first", ordered[0].UpstreamModel)
	}

	// stable_first：research_audit 无场景加成，稳定性 +15 让 Sonnet 超过 mini
	steps = stepsFor("gpt-4o-mini", "claude-3.5-sonnet")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "research_audit", SecurityLevel: "public",
	}, "stable_first")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "sonnet") {
		t.Errorf("stable_first: first = %q, want sonnet first", ordered[0].UpstreamModel)
	}

	// quality_first：与稳定优先同等对待，Sonnet 应排到 mini 之前
	steps = stepsFor("gpt-4o-mini", "claude-3.5-sonnet")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "research_audit", SecurityLevel: "public",
	}, "quality_first")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "sonnet") {
		t.Errorf("quality_first: first = %q, want sonnet first", ordered[0].UpstreamModel)
	}

	// speed_first：Flash 的 +15 应超过无加成的 Sonnet
	steps = stepsFor("claude-3.5-sonnet", "gemini-2.0-flash")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "research_audit", SecurityLevel: "public",
	}, "speed_first")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "flash") {
		t.Errorf("speed_first: first = %q, want flash first", ordered[0].UpstreamModel)
	}

	// balanced：无策略加成，保持原始顺序
	steps = stepsFor("command-r", "mistral-large")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "research_audit", SecurityLevel: "public",
	}, "balanced")
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].UpstreamModel != "command-r" || ordered[1].UpstreamModel != "mistral-large" {
		t.Errorf("balanced: order = [%s, %s], want original order", ordered[0].UpstreamModel, ordered[1].UpstreamModel)
	}
}

// ---------------------------------------------------------------------------
// Combo 端到端：combo.QuickStrategy（UI 持久化值）驱动 ResolveChatPlan 排序
// ---------------------------------------------------------------------------

func newComboTestService(t *testing.T) *config.Service {
	t.Helper()
	svc := config.NewServiceWithPath(config.AppConfig{RoutingStrategy: "fallback"}, filepath.Join(t.TempDir(), "config.json"))
	if _, err := svc.SaveCustomChannel(config.Channel{
		ChannelID: "ch-openai", ProviderType: "openai", DisplayName: "OpenAI",
		BaseURL: "http://openai.test", Enabled: true, Models: []string{"gpt-4o", "gpt-4o-mini"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveCustomChannel(config.Channel{
		ChannelID: "ch-deepseek", ProviderType: "deepseek", DisplayName: "DeepSeek",
		BaseURL: "http://deepseek.test", Enabled: true, Models: []string{"deepseek-chat"},
	}); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestResolveChatPlan_ComboQuickStrategyCostFirst(t *testing.T) {
	svc := newComboTestService(t)
	if err := svc.SaveModelCombo(config.ModelCombo{
		Name: "combo-cost", Strategy: "fallback", StickyUses: 1, QuickStrategy: "cost_first",
		Steps: []config.ModelComboStep{
			{Model: "gpt-4o"},
			{Model: "deepseek-chat"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(svc)
	intent := &IntentContext{Scenario: "code_logic", RequiredIQ: "balanced", SecurityLevel: "public"}
	planName, steps, err := r.ResolveChatPlan("combo-cost", "chat", intent)
	if err != nil {
		t.Fatal(err)
	}
	if planName != "combo-cost" {
		t.Fatalf("planName = %q, want combo-cost", planName)
	}
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if !strings.Contains(steps[0].UpstreamModel, "deepseek") {
		t.Errorf("first step = %q, want deepseek (cost_first via combo.QuickStrategy)", steps[0].UpstreamModel)
	}
	if steps[1].UpstreamModel != "gpt-4o" {
		t.Errorf("second step = %q, want gpt-4o", steps[1].UpstreamModel)
	}
}

func TestResolveChatPlan_ComboWithoutQuickStrategyKeepsOrder(t *testing.T) {
	svc := newComboTestService(t)
	if err := svc.SaveModelCombo(config.ModelCombo{
		Name: "combo-plain", Strategy: "fallback", StickyUses: 1,
		Steps: []config.ModelComboStep{
			{Model: "gpt-4o"},
			{Model: "deepseek-chat"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(svc)
	intent := &IntentContext{Scenario: "code_logic", RequiredIQ: "balanced", SecurityLevel: "public"}
	_, steps, err := r.ResolveChatPlan("combo-plain", "chat", intent)
	if err != nil {
		t.Fatal(err)
	}
	// 均衡（无快速策略）：code_logic 场景无加成差异，保持主→备原序
	if steps[0].UpstreamModel != "gpt-4o" {
		t.Errorf("first step = %q, want gpt-4o (original order)", steps[0].UpstreamModel)
	}
}

func TestResolveChatPlan_ComboQualityFirstAlias(t *testing.T) {
	svc := newComboTestService(t)
	if _, err := svc.SaveCustomChannel(config.Channel{
		ChannelID: "ch-anthropic", ProviderType: "anthropic", DisplayName: "Anthropic",
		BaseURL: "http://anthropic.test", Enabled: true, Models: []string{"claude-3.5-sonnet"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveModelCombo(config.ModelCombo{
		Name: "combo-quality", Strategy: "fallback", StickyUses: 1, QuickStrategy: "quality_first",
		Steps: []config.ModelComboStep{
			{Model: "gpt-4o-mini"},
			{Model: "claude-3.5-sonnet"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	r := NewResolver(svc)
	intent := &IntentContext{Scenario: "research_audit", RequiredIQ: "balanced", SecurityLevel: "public"}
	_, steps, err := r.ResolveChatPlan("combo-quality", "chat", intent)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(steps[0].UpstreamModel, "sonnet") {
		t.Errorf("first step = %q, want sonnet (quality_first via combo.QuickStrategy)", steps[0].UpstreamModel)
	}
}
