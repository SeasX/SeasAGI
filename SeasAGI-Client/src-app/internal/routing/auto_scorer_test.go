package routing

import "testing"

func TestAutoScorerDefault(t *testing.T) {
	scorer := NewScorer(AutoDefault)
	stats := RuntimeStats{
		Healthy:        true,
		QuotaRemaining: 0.8,
		CostPer1M:      2.5,
		LatencyMs:      2000,
		SuccessRate:    0.95,
		LastUsedMs:     60000,
		CacheHitRate:   0.3,
		PenaltyScore:   1,
		ThroughputRPM:  60,
		ContextWindow:  128000,
		ToolSupport:    true,
		Available:      true,
	}
	score := scorer.Score(stats)
	if score <= 0 {
		t.Errorf("expected positive score, got %.1f", score)
	}
	if score > 100 {
		t.Errorf("expected score <= 100, got %.1f", score)
	}
}

func TestAutoScorerFast(t *testing.T) {
	scorer := NewScorer(AutoFast)

	fastStats := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 5.0,
		LatencyMs: 500, SuccessRate: 0.9, Available: true,
		ToolSupport: true, ContextWindow: 128000, PenaltyScore: 0,
	}
	slowStats := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 5.0,
		LatencyMs: 5000, SuccessRate: 0.9, Available: true,
		ToolSupport: true, ContextWindow: 128000, PenaltyScore: 0,
	}

	fastScore := scorer.Score(fastStats)
	slowScore := scorer.Score(slowStats)
	if fastScore <= slowScore {
		t.Errorf("expected fast > slow, got fast=%.1f slow=%.1f", fastScore, slowScore)
	}
}

func TestAutoScorerCheap(t *testing.T) {
	scorer := NewScorer(AutoCheap)

	cheapStats := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 0.5,
		LatencyMs: 3000, SuccessRate: 0.9, Available: true,
		ToolSupport: true, ContextWindow: 128000, PenaltyScore: 0,
	}
	expensiveStats := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 15.0,
		LatencyMs: 3000, SuccessRate: 0.9, Available: true,
		ToolSupport: true, ContextWindow: 128000, PenaltyScore: 0,
	}

	cheapScore := scorer.Score(cheapStats)
	expensiveScore := scorer.Score(expensiveStats)
	if cheapScore <= expensiveScore {
		t.Errorf("expected cheap > expensive, got cheap=%.1f expensive=%.1f", cheapScore, expensiveScore)
	}
}

func TestAutoScorerCoding(t *testing.T) {
	scorer := NewScorer(AutoCoding)

	withTools := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 5.0,
		LatencyMs: 2000, SuccessRate: 0.95, Available: true,
		ToolSupport: true, ContextWindow: 128000, PenaltyScore: 0,
	}
	withoutTools := RuntimeStats{
		Healthy: true, QuotaRemaining: 0.5, CostPer1M: 5.0,
		LatencyMs: 2000, SuccessRate: 0.95, Available: true,
		ToolSupport: false, ContextWindow: 128000, PenaltyScore: 0,
	}

	toolsScore := scorer.Score(withTools)
	noToolsScore := scorer.Score(withoutTools)
	if toolsScore <= noToolsScore {
		t.Errorf("expected tools > no-tools, got tools=%.1f no-tools=%.1f", toolsScore, noToolsScore)
	}
}

func TestAutoScorerHealthPenalty(t *testing.T) {
	scorer := NewScorer(AutoDefault)

	healthy := RuntimeStats{Healthy: true, Available: true, SuccessRate: 0.9}
	unhealthy := RuntimeStats{Healthy: false, Available: true, SuccessRate: 0.9}

	healthyScore := scorer.Score(healthy)
	unhealthyScore := scorer.Score(unhealthy)
	if unhealthyScore > 0 {
		t.Errorf("expected 0 for unhealthy, got %.1f", unhealthyScore)
	}
	if healthyScore <= 0 {
		t.Errorf("expected positive for healthy, got %.1f", healthyScore)
	}
}

func TestAutoScorerPenaltyImpact(t *testing.T) {
	scorer := NewScorer(AutoDefault)

	lowPenalty := RuntimeStats{
		Healthy: true, Available: true, SuccessRate: 0.9,
		PenaltyScore: 0, QuotaRemaining: 0.5, CostPer1M: 2.5,
	}
	highPenalty := RuntimeStats{
		Healthy: true, Available: true, SuccessRate: 0.9,
		PenaltyScore: 8, QuotaRemaining: 0.5, CostPer1M: 2.5,
	}

	lowScore := scorer.Score(lowPenalty)
	highScore := scorer.Score(highPenalty)
	if highScore >= lowScore {
		t.Errorf("expected low penalty > high penalty, got low=%.1f high=%.1f", lowScore, highScore)
	}
}

func TestAutoStrategySelect(t *testing.T) {
	strategy := NewAutoStrategy(AutoDefault)
	candidates := []AutoCandidate{
		{Model: "model-a", ChannelID: "ch1", Stats: RuntimeStats{
			Healthy: true, Available: true, SuccessRate: 0.8,
			CostPer1M: 5.0, LatencyMs: 3000, PenaltyScore: 3,
		}},
		{Model: "model-b", ChannelID: "ch2", Stats: RuntimeStats{
			Healthy: true, Available: true, SuccessRate: 0.95,
			CostPer1M: 1.0, LatencyMs: 1000, PenaltyScore: 0,
		}},
	}

	selected, err := strategy.Select(candidates)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if selected.Model != "model-b" {
		t.Errorf("expected model-b (better stats), got %s", selected.Model)
	}
}

func TestAutoStrategyEmpty(t *testing.T) {
	strategy := NewAutoStrategy(AutoDefault)
	_, err := strategy.Select([]AutoCandidate{})
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

func TestAutoScorerUnavailable(t *testing.T) {
	scorer := NewScorer(AutoDefault)
	stats := RuntimeStats{Healthy: true, Available: false}
	if score := scorer.Score(stats); score != 0 {
		t.Errorf("expected 0 for unavailable, got %.1f", score)
	}
}
