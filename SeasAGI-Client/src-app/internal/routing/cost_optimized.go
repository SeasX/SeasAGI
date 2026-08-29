package routing

import (
	"sort"
)

// CostCandidate cost-optimized 策略候选
type CostCandidate struct {
	Model         string
	ChannelID     string
	InputPricePer1M  float64 // 每百万 token 输入价格 (USD)
	OutputPricePer1M float64 // 每百万 token 输出价格 (USD)
	HasPricing    bool
}

// CostOptimizedStrategy 从候选列表中选择 $/请求 最低的目标
type CostOptimizedStrategy struct {
	estimatedInputTokens  int
	estimatedOutputTokens int
}

// NewCostOptimizedStrategy 创建 cost-optimized 策略
// estimatedInputTokens/outputTokens 用于估算单次请求成本
func NewCostOptimizedStrategy(estimatedInputTokens, estimatedOutputTokens int) *CostOptimizedStrategy {
	if estimatedInputTokens <= 0 {
		estimatedInputTokens = 1000
	}
	if estimatedOutputTokens <= 0 {
		estimatedOutputTokens = 500
	}
	return &CostOptimizedStrategy{
		estimatedInputTokens:  estimatedInputTokens,
		estimatedOutputTokens: estimatedOutputTokens,
	}
}

// Select 从候选列表中选择成本最低的
func (s *CostOptimizedStrategy) Select(candidates []CostCandidate) (CostCandidate, error) {
	if len(candidates) == 0 {
		return CostCandidate{}, &RoutingError{Type: "no_candidates", Message: "no candidates available"}
	}

	// 计算每个候选的估算成本并排序
	scored := make([]scoredCandidate, len(candidates))
	for i, c := range candidates {
		cost := s.estimateCost(c)
		scored[i] = scoredCandidate{candidate: c, score: cost}
	}

	sort.Slice(scored, func(i, j int) bool {
		// 有定价的优先，无定价的排最后
		if !scored[i].candidate.HasPricing && scored[j].candidate.HasPricing {
			return false
		}
		if scored[i].candidate.HasPricing && !scored[j].candidate.HasPricing {
			return true
		}
		// 都有定价或都无定价 → 按成本排序
		return scored[i].score < scored[j].score
	})

	return scored[0].candidate, nil
}

// estimateCost 估算单次请求成本 (USD)
func (s *CostOptimizedStrategy) estimateCost(c CostCandidate) float64 {
	if !c.HasPricing {
		return 999999 // 无定价数据排最后
	}
	inputCost := float64(s.estimatedInputTokens) / 1_000_000 * c.InputPricePer1M
	outputCost := float64(s.estimatedOutputTokens) / 1_000_000 * c.OutputPricePer1M
	return inputCost + outputCost
}

type scoredCandidate struct {
	candidate CostCandidate
	score     float64
}
