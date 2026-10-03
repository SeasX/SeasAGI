package routing

import (
	"strings"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func msg(text string) []map[string]interface{} {
	return []map[string]interface{}{
		{"role": "user", "content": text},
	}
}

func TestDetectIntent_Scenarios(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		scenario string
		iq       string
	}{
		{"image_gen_en", "Please draw a cat in oil painting style", "image_gen", "balanced"},
		{"image_gen_zh", "帮我画一只赛博朋克风格的猫", "image_gen", "balanced"},
		{"code_logic_high_iq", "Write a function in Go to build a high-performance parallel cache, with thorough tests", "code_logic", "high"},
		{"code_logic_simple", "refactor this loop", "code_logic", "balanced"},
		{"research_audit", "Summarize this paper and analyze this methodology", "research_audit", "balanced"},
		{"creative", "Write a poem about the sea", "creative", "balanced"},
		{"casual_short", "hello there", "casual", "low"},
		{"data_extract", "Extract the following table and convert to csv", "data_extract", "balanced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intent := DetectIntent(msg(tc.text))
			if intent.Scenario != tc.scenario {
				t.Errorf("scenario = %q, want %q", intent.Scenario, tc.scenario)
			}
			if intent.RequiredIQ != tc.iq {
				t.Errorf("required_iq = %q, want %q", intent.RequiredIQ, tc.iq)
			}
			if intent.SecurityLevel != "public" {
				t.Errorf("security_level = %q, want public", intent.SecurityLevel)
			}
		})
	}
}

func TestDetectIntent_Sensitive(t *testing.T) {
	intent := DetectIntent(msg("Here is my api key sk-123, please rotate it"))
	if intent.SecurityLevel != "sensitive" {
		t.Errorf("security_level = %q, want sensitive", intent.SecurityLevel)
	}
	intent = DetectIntent(msg("数据库密码是 hunter2，帮我检查配置"))
	if intent.SecurityLevel != "sensitive" {
		t.Errorf("security_level = %q, want sensitive (zh)", intent.SecurityLevel)
	}
}

func TestDetectIntent_Tags(t *testing.T) {
	intent := DetectIntent(msg("Prove this equation by induction"))
	found := false
	for _, tag := range intent.Tags {
		if tag == "math" {
			found = true
		}
	}
	if !found {
		t.Errorf("tags = %v, want math", intent.Tags)
	}
}

func TestDetectIntent_TaskTypePassthrough(t *testing.T) {
	// 意图检测须沿用既有任务类型启发式
	if got := DetectIntent(msg("hello")).TaskType; got != "chat" {
		t.Errorf("task_type = %q, want chat", got)
	}
	vision := []map[string]interface{}{
		{"role": "user", "content": []interface{}{
			map[string]interface{}{"type": "text", "text": "what is this"},
			map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "http://x"}},
		}},
	}
	if got := DetectIntent(vision).TaskType; got != "vision" {
		t.Errorf("task_type = %q, want vision", got)
	}
	tools := []map[string]interface{}{
		{"role": "assistant", "content": "", "tool_calls": []interface{}{map[string]interface{}{"id": "1"}}},
		{"role": "user", "content": "continue"},
	}
	if got := DetectIntent(tools).TaskType; got != "tools" {
		t.Errorf("task_type = %q, want tools", got)
	}
}

func stepsFor(models ...string) []PlanStep {
	steps := make([]PlanStep, 0, len(models))
	for _, m := range models {
		steps = append(steps, PlanStep{
			Channel:       config.Channel{ChannelID: "ch-" + m, ChannelType: "custom", Enabled: true},
			UpstreamModel: m,
		})
	}
	return steps
}

func TestApplyIntentRouting_ReorderByScenario(t *testing.T) {
	r := &Resolver{}

	// code_logic + high IQ：Sonnet 应排在 Flash 之前
	steps := stepsFor("gemini-1.5-flash", "claude-3.5-sonnet")
	ordered, err := r.applyIntentRouting(steps, &IntentContext{
		Scenario: "code_logic", RequiredIQ: "high", SecurityLevel: "public",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "sonnet") {
		t.Errorf("first = %q, want sonnet first", ordered[0].UpstreamModel)
	}

	// image_gen：dall-e-3 应排在 gpt-4o 之前
	steps = stepsFor("gpt-4o", "dall-e-3")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "image_gen", SecurityLevel: "public",
	}, "quality-first")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "dall-e") {
		t.Errorf("first = %q, want dall-e first", ordered[0].UpstreamModel)
	}

	// casual：轻量模型应排前
	steps = stepsFor("gpt-4o", "gpt-4o-mini")
	ordered, err = r.applyIntentRouting(steps, &IntentContext{
		Scenario: "casual", SecurityLevel: "public",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "mini") {
		t.Errorf("first = %q, want mini first", ordered[0].UpstreamModel)
	}
}

func TestApplyIntentRouting_MathTagPrefersReasoner(t *testing.T) {
	r := &Resolver{}
	steps := stepsFor("qwen2.5-72b", "deepseek-r1-reasoner")
	ordered, err := r.applyIntentRouting(steps, &IntentContext{
		Scenario: "research_audit", SecurityLevel: "public", Tags: []string{"math"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ordered[0].UpstreamModel, "reasoner") {
		t.Errorf("first = %q, want reasoner first", ordered[0].UpstreamModel)
	}
}

func TestApplyIntentRouting_SensitiveForcesLocalOnly(t *testing.T) {
	r := &Resolver{}
	mixed := []PlanStep{
		{Channel: config.Channel{ChannelID: "cloud", ChannelType: "platform", Enabled: true}, UpstreamModel: "gpt-4o"},
		{Channel: config.Channel{ChannelID: "local", ChannelType: "custom", Enabled: true}, UpstreamModel: "qwen2.5-72b"},
	}
	ordered, err := r.applyIntentRouting(mixed, &IntentContext{Scenario: "casual", SecurityLevel: "sensitive"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 1 || ordered[0].Channel.ChannelID != "local" {
		t.Fatalf("ordered = %+v, want only local channel", ordered)
	}

	// 全云渠道 + 敏感意图 → 直接报错，不允许敏感数据出云
	cloudOnly := []PlanStep{
		{Channel: config.Channel{ChannelID: "cloud", ChannelType: "platform", Enabled: true}, UpstreamModel: "gpt-4o"},
	}
	if _, err := r.applyIntentRouting(cloudOnly, &IntentContext{SecurityLevel: "sensitive"}, ""); err == nil {
		t.Error("expected routing error when no local-only channel exists for sensitive intent")
	}
}

func TestApplyIntentRouting_NilIntentNoop(t *testing.T) {
	r := &Resolver{}
	steps := stepsFor("b", "a")
	ordered, err := r.applyIntentRouting(steps, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].UpstreamModel != "b" || len(ordered) != 2 {
		t.Errorf("nil intent should keep original order, got %v", ordered)
	}
}
