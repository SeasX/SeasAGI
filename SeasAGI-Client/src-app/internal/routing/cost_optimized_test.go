package routing

import (
	"testing"
)

func TestCostOptimizedSelectsCheapest(t *testing.T) {
	candidates := []CostCandidate{
		{Model: "gpt-4o", ChannelID: "ch1", InputPricePer1M: 2.50, OutputPricePer1M: 10.00, HasPricing: true},
		{Model: "gpt-4o-mini", ChannelID: "ch2", InputPricePer1M: 0.15, OutputPricePer1M: 0.60, HasPricing: true},
		{Model: "claude-3-opus", ChannelID: "ch3", InputPricePer1M: 15.00, OutputPricePer1M: 75.00, HasPricing: true},
	}

	s := NewCostOptimizedStrategy(1000, 500)
	selected, err := s.Select(candidates)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	if selected.Model != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini (cheapest), got %s", selected.Model)
	}
}

func TestCostOptimizedNoPricingData(t *testing.T) {
	candidates := []CostCandidate{
		{Model: "unknown-model", ChannelID: "ch1", HasPricing: false},
		{Model: "gpt-4o-mini", ChannelID: "ch2", InputPricePer1M: 0.15, OutputPricePer1M: 0.60, HasPricing: true},
	}

	s := NewCostOptimizedStrategy(1000, 500)
	selected, err := s.Select(candidates)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	// 有定价数据的应优先于无定价数据的
	if selected.Model != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini (has pricing), got %s", selected.Model)
	}
}

func TestCostOptimizedAllNoPricing(t *testing.T) {
	candidates := []CostCandidate{
		{Model: "model-a", ChannelID: "ch1", HasPricing: false},
		{Model: "model-b", ChannelID: "ch2", HasPricing: false},
	}

	s := NewCostOptimizedStrategy(1000, 500)
	selected, err := s.Select(candidates)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	// 都无定价时应返回第一个
	if selected.Model != "model-a" {
		t.Errorf("expected model-a (first), got %s", selected.Model)
	}
}

func TestCostOptimizedEmpty(t *testing.T) {
	s := NewCostOptimizedStrategy(1000, 500)
	_, err := s.Select([]CostCandidate{})
	if err == nil {
		t.Error("expected error for empty candidates")
	}
}

func TestCostOptimizedEstimateTokens(t *testing.T) {
	// 不同 token 估算应影响选择
	candidates := []CostCandidate{
		{Model: "model-a", ChannelID: "ch1", InputPricePer1M: 1.00, OutputPricePer1M: 1.00, HasPricing: true},
		{Model: "model-b", ChannelID: "ch2", InputPricePer1M: 0.50, OutputPricePer1M: 5.00, HasPricing: true},
	}

	// 低 output 估算 → model-b 更便宜 (0.50*0.001 + 5*0.0001 = 0.001)
	s1 := NewCostOptimizedStrategy(1000, 100)
	selected1, _ := s1.Select(candidates)
	if selected1.Model != "model-b" {
		t.Errorf("expected model-b for low output, got %s", selected1.Model)
	}

	// 高 output 估算 → model-a 更便宜 (1*0.001 + 1*0.01 = 0.011 vs 0.5*0.001 + 5*0.01 = 0.0505)
	s2 := NewCostOptimizedStrategy(1000, 10000)
	selected2, _ := s2.Select(candidates)
	if selected2.Model != "model-a" {
		t.Errorf("expected model-a for high output, got %s", selected2.Model)
	}
}

func TestCostOptimizedWithCombo(t *testing.T) {
	// 模拟 Combo Step 场景：多个步骤候选
	step1Candidates := []CostCandidate{
		{Model: "gpt-4o", ChannelID: "ch1", InputPricePer1M: 2.50, OutputPricePer1M: 10.00, HasPricing: true},
		{Model: "deepseek-chat", ChannelID: "ch2", InputPricePer1M: 0.27, OutputPricePer1M: 1.10, HasPricing: true},
	}

	s := NewCostOptimizedStrategy(2000, 1000)
	selected, err := s.Select(step1Candidates)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	if selected.Model != "deepseek-chat" {
		t.Errorf("expected deepseek-chat (cheaper), got %s", selected.Model)
	}
}
