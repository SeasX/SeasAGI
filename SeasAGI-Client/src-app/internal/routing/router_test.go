package routing

import (
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

func TestNewResolver(t *testing.T) {
	r := NewResolver(nil)
	if r == nil {
		t.Fatal("NewResolver returned nil")
	}
	if r.state == nil {
		t.Error("Resolver.state is nil")
	}
	if r.sessions == nil {
		t.Error("Resolver.sessions is nil")
	}
}

func TestRoutingError_ImplementsError(t *testing.T) {
	var err error = &RoutingError{Type: "test", Message: "an error occurred"}
	if err.Error() != "an error occurred" {
		t.Errorf("expected 'an error occurred', got %q", err.Error())
	}
}

func TestRoutingError_HasCorrectFields(t *testing.T) {
	re := &RoutingError{
		Type:    "routing_error",
		Message: "no enabled channel configured",
	}
	if re.Type != "routing_error" {
		t.Errorf("expected Type 'routing_error', got %q", re.Type)
	}
	if re.Message != "no enabled channel configured" {
		t.Errorf("expected Message 'no enabled channel configured', got %q", re.Message)
	}
}

func TestPlanStep_HasCorrectFields(t *testing.T) {
	ch := config.Channel{
		ChannelID:    "test-channel",
		ChannelType:  "custom",
		ProviderType: "openai",
		Enabled:      true,
	}
	step := PlanStep{
		Channel:       ch,
		UpstreamModel: "gpt-4o",
		StepRole:      "primary",
	}
	if step.Channel.ChannelID != "test-channel" {
		t.Errorf("expected ChannelID 'test-channel', got %q", step.Channel.ChannelID)
	}
	if step.UpstreamModel != "gpt-4o" {
		t.Errorf("expected UpstreamModel 'gpt-4o', got %q", step.UpstreamModel)
	}
	if step.StepRole != "primary" {
		t.Errorf("expected StepRole 'primary', got %q", step.StepRole)
	}
}

func TestFilterCandidatesByConstraints_NoFilters(t *testing.T) {
	r := &Resolver{}
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1"}, UpstreamModel: "gpt-4o"},
		{Channel: config.Channel{ChannelID: "ch2"}, UpstreamModel: "gpt-4o-mini"},
	}
	result := r.FilterCandidatesByConstraints(candidates, nil)
	if len(result) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(result))
	}

	result = r.FilterCandidatesByConstraints(candidates, map[string]any{})
	if len(result) != 2 {
		t.Errorf("expected 2 candidates with empty constraints, got %d", len(result))
	}
}

func TestFilterCandidatesByConstraints_MaxPrice(t *testing.T) {
	r := &Resolver{}
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1"}, UpstreamModel: "gpt-4o-mini"}, // cost 0.15
		{Channel: config.Channel{ChannelID: "ch2"}, UpstreamModel: "gpt-4o"},      // cost 5.0
		{Channel: config.Channel{ChannelID: "ch3"}, UpstreamModel: "gpt-4-turbo"}, // cost 10.0
	}

	constraints := map[string]any{"max_price": float64(3.0)}
	result := r.FilterCandidatesByConstraints(candidates, constraints)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate after max_price filter, got %d", len(result))
	}
	if result[0].UpstreamModel != "gpt-4o-mini" {
		t.Errorf("expected remaining candidate 'gpt-4o-mini', got %q", result[0].UpstreamModel)
	}
}

func TestFilterCandidatesByConstraints_MaxLatency(t *testing.T) {
	r := &Resolver{}
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1", ChannelType: "custom"}, UpstreamModel: "gpt-4o-mini"},     // fast (flash/mini/haiku/nano) -> 600
		{Channel: config.Channel{ChannelID: "ch2", ChannelType: "platform"}, UpstreamModel: "gpt-4o"},        // medium (sonnet/4o/pro) -> 1400
		{Channel: config.Channel{ChannelID: "ch3", ChannelType: "platform"}, UpstreamModel: "claude-3-opus"}, // slow (opus/reasoner) -> 2200
	}

	constraints := map[string]any{"max_latency_ms": float64(1000)}
	result := r.FilterCandidatesByConstraints(candidates, constraints)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate after max_latency filter, got %d", len(result))
	}
	if result[0].UpstreamModel != "gpt-4o-mini" {
		t.Errorf("expected remaining candidate 'gpt-4o-mini', got %q", result[0].UpstreamModel)
	}
}

func TestFilterCandidatesByConstraints_DataPolicy(t *testing.T) {
	r := &Resolver{}
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1", ChannelType: "custom"}, UpstreamModel: "gpt-4o"},
		{Channel: config.Channel{ChannelID: "ch2", ChannelType: "platform"}, UpstreamModel: "gpt-4o"},
	}

	// local_only should keep only custom channels
	result := r.FilterCandidatesByConstraints(candidates, map[string]any{"data_policy": "local_only"})
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate for local_only, got %d", len(result))
	}
	if result[0].Channel.ChannelType != "custom" {
		t.Errorf("expected custom channel type, got %q", result[0].Channel.ChannelType)
	}

	// cloud_only should keep only platform channels
	result = r.FilterCandidatesByConstraints(candidates, map[string]any{"data_policy": "cloud_only"})
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate for cloud_only, got %d", len(result))
	}
	if result[0].Channel.ChannelType != "platform" {
		t.Errorf("expected platform channel type, got %q", result[0].Channel.ChannelType)
	}

	// any should keep all
	result = r.FilterCandidatesByConstraints(candidates, map[string]any{"data_policy": "any"})
	if len(result) != 2 {
		t.Errorf("expected 2 candidates for 'any' data_policy, got %d", len(result))
	}
}

func TestFilterCandidatesByConstraints_MultipleFilters(t *testing.T) {
	r := &Resolver{}
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1", ChannelType: "custom"}, UpstreamModel: "gpt-4o-mini"},     // cost 0.15, fast
		{Channel: config.Channel{ChannelID: "ch2", ChannelType: "platform"}, UpstreamModel: "gpt-4o"},        // cost 5.0, medium
		{Channel: config.Channel{ChannelID: "ch3", ChannelType: "platform"}, UpstreamModel: "claude-3-opus"}, // cost 15.0, slow
	}

	constraints := map[string]any{
		"max_price":      float64(4.0),
		"max_latency_ms": float64(1000),
		"data_policy":    "local_only",
	}
	result := r.FilterCandidatesByConstraints(candidates, constraints)
	if len(result) != 1 {
		t.Fatalf("expected 1 candidate after multiple filters, got %d", len(result))
	}
	if result[0].UpstreamModel != "gpt-4o-mini" {
		t.Errorf("expected remaining candidate 'gpt-4o-mini', got %q", result[0].UpstreamModel)
	}
	if result[0].Channel.ChannelType != "custom" {
		t.Errorf("expected custom channel type, got %q", result[0].Channel.ChannelType)
	}
}

func TestFilterCandidatesByConstraints_NoCandidates(t *testing.T) {
	r := &Resolver{}
	// All candidates filtered out should return nil
	candidates := []PlanStep{
		{Channel: config.Channel{ChannelID: "ch1", ChannelType: "platform"}, UpstreamModel: "gpt-4o"},
	}
	constraints := map[string]any{
		"data_policy": "local_only",
	}
	result := r.FilterCandidatesByConstraints(candidates, constraints)
	if result != nil {
		t.Errorf("expected nil when all candidates filtered out, got %v", result)
	}
}

func TestEstimatePlanStepLatencyMs_Fast(t *testing.T) {
	fastModels := []string{
		"gpt-4o-mini",      // contains "mini"
		"gemini-2.5-flash", // contains "flash"
		"claude-3-haiku",   // contains "haiku"
		"gpt-5-nano",       // contains "nano"
	}

	for _, model := range fastModels {
		step := PlanStep{
			UpstreamModel: model,
			Channel:       config.Channel{ChannelType: "platform"},
		}
		latency := estimatePlanStepLatencyMs(step)
		if latency != 600 {
			t.Errorf("expected latency 600 for fast model %q, got %v", model, latency)
		}
	}
}

func TestEstimatePlanStepLatencyMs_Medium(t *testing.T) {
	mediumModels := []string{
		"claude-sonnet-5", // contains "sonnet"
		"gpt-4o",          // contains "4o"
		"gpt-4-turbo",     // contains "gpt-4"
	}

	for _, model := range mediumModels {
		step := PlanStep{
			UpstreamModel: model,
			Channel:       config.Channel{ChannelType: "custom"},
		}
		latency := estimatePlanStepLatencyMs(step)
		if latency != 1400 {
			t.Errorf("expected latency 1400 for medium model %q, got %v", model, latency)
		}
	}
}

func TestEstimatePlanStepLatencyMs_Slow(t *testing.T) {
	slowModels := []string{
		"claude-3-opus",     // contains "opus"
		"deepseek-reasoner", // contains "reasoner"
	}

	for _, model := range slowModels {
		step := PlanStep{
			UpstreamModel: model,
			Channel:       config.Channel{ChannelType: "custom"},
		}
		latency := estimatePlanStepLatencyMs(step)
		if latency != 2200 {
			t.Errorf("expected latency 2200 for slow model %q, got %v", model, latency)
		}
	}
}

func TestEstimatePlanStepLatencyMs_UnknownModel(t *testing.T) {
	// Unknown model should fall back to channel-type heuristic
	step := PlanStep{
		UpstreamModel: "some-unknown-model",
		Channel:       config.Channel{ChannelType: "custom"},
	}
	latency := estimatePlanStepLatencyMs(step)
	if latency != 500 {
		t.Errorf("expected latency 500 for unknown model on custom channel, got %v", latency)
	}

	step2 := PlanStep{
		UpstreamModel: "another-unknown-model",
		Channel:       config.Channel{ChannelType: "platform"},
	}
	latency2 := estimatePlanStepLatencyMs(step2)
	if latency2 != 1500 {
		t.Errorf("expected latency 1500 for unknown model on platform channel, got %v", latency2)
	}
}
