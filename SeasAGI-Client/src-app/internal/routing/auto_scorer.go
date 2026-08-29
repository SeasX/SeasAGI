package routing

// ScoringFactor 评分因子
type ScoringFactor string

const (
	FactorHealth        ScoringFactor = "health"
	FactorQuota         ScoringFactor = "quota"
	FactorCost          ScoringFactor = "cost"
	FactorLatency       ScoringFactor = "latency"
	FactorSuccessRate   ScoringFactor = "success_rate"
	FactorFreshness     ScoringFactor = "freshness"
	FactorCacheHitRate  ScoringFactor = "cache_hit_rate"
	FactorPenalty       ScoringFactor = "penalty"
	FactorThroughput    ScoringFactor = "throughput"
	FactorContextWindow ScoringFactor = "context_window"
	FactorToolSupport   ScoringFactor = "tool_support"
	FactorAvailability  ScoringFactor = "availability"
)

// AutoVariant auto 策略变体
type AutoVariant string

const (
	AutoDefault AutoVariant = "auto"
	AutoCoding  AutoVariant = "auto/coding"
	AutoFast    AutoVariant = "auto/fast"
	AutoCheap   AutoVariant = "auto/cheap"
	AutoOffline AutoVariant = "auto/offline"
	AutoSmart   AutoVariant = "auto/smart"
)

// RuntimeStats 运行时统计
type RuntimeStats struct {
	Healthy        bool
	QuotaRemaining float64 // 0-1
	CostPer1M      float64
	LatencyMs      float64
	SuccessRate    float64 // 0-1
	LastUsedMs     int64   // 距上次使用的毫秒数
	CacheHitRate   float64 // 0-1
	PenaltyScore   float64 // 0-10
	ThroughputRPM  float64
	ContextWindow  int
	ToolSupport    bool
	Available      bool
}

// Scorer 12 因子评分引擎
type Scorer struct {
	weights map[ScoringFactor]float64
}

// NewScorer 创建评分器
func NewScorer(variant AutoVariant) *Scorer {
	return &Scorer{weights: defaultWeights(variant)}
}

// Score 对候选评分（0-100，越高越好）
func (s *Scorer) Score(stats RuntimeStats) float64 {
	if !stats.Available || !stats.Healthy {
		return 0
	}

	total := 0.0
	for factor, weight := range s.weights {
		score := s.scoreFactor(factor, stats)
		total += score * weight
	}
	return total
}

// scoreFactor 对单个因子评分（0-100）
func (s *Scorer) scoreFactor(factor ScoringFactor, stats RuntimeStats) float64 {
	switch factor {
	case FactorHealth:
		if stats.Healthy { return 100 }
		return 0
	case FactorQuota:
		return stats.QuotaRemaining * 100
	case FactorCost:
		if stats.CostPer1M <= 0 { return 50 }
		if stats.CostPer1M >= 20 { return 0 }
		return (1 - stats.CostPer1M/20) * 100
	case FactorLatency:
		if stats.LatencyMs <= 0 { return 50 }
		if stats.LatencyMs >= 10000 { return 0 }
		return (1 - stats.LatencyMs/10000) * 100
	case FactorSuccessRate:
		return stats.SuccessRate * 100
	case FactorFreshness:
		if stats.LastUsedMs <= 0 { return 100 }
		if stats.LastUsedMs >= 3600000 { return 0 }
		return (1 - float64(stats.LastUsedMs)/3600000) * 100
	case FactorCacheHitRate:
		return stats.CacheHitRate * 100
	case FactorPenalty:
		return (1 - stats.PenaltyScore/10) * 100
	case FactorThroughput:
		if stats.ThroughputRPM <= 0 { return 50 }
		if stats.ThroughputRPM >= 100 { return 100 }
		return stats.ThroughputRPM
	case FactorContextWindow:
		if stats.ContextWindow <= 0 { return 50 }
		if stats.ContextWindow >= 200000 { return 100 }
		return float64(stats.ContextWindow) / 200000 * 100
	case FactorToolSupport:
		if stats.ToolSupport { return 100 }
		return 0
	case FactorAvailability:
		if stats.Available { return 100 }
		return 0
	default:
		return 50
	}
}

// defaultWeights 返回变体对应的因子权重
func defaultWeights(variant AutoVariant) map[ScoringFactor]float64 {
	base := map[ScoringFactor]float64{
		FactorHealth:        0.15,
		FactorQuota:         0.08,
		FactorCost:          0.08,
		FactorLatency:       0.08,
		FactorSuccessRate:   0.12,
		FactorFreshness:     0.05,
		FactorCacheHitRate:  0.05,
		FactorPenalty:       0.10,
		FactorThroughput:    0.05,
		FactorContextWindow: 0.05,
		FactorToolSupport:   0.05,
		FactorAvailability:  0.14,
	}

	switch variant {
	case AutoCoding:
		base[FactorToolSupport] = 0.15
		base[FactorSuccessRate] = 0.15
		base[FactorCost] = 0.03
		base[FactorLatency] = 0.05
	case AutoFast:
		base[FactorLatency] = 0.20
		base[FactorCost] = 0.03
		base[FactorThroughput] = 0.10
	case AutoCheap:
		base[FactorCost] = 0.25
		base[FactorLatency] = 0.03
		base[FactorToolSupport] = 0.02
	case AutoOffline:
		base[FactorQuota] = 0.20
		base[FactorCost] = 0.03
		base[FactorFreshness] = 0.02
	case AutoSmart:
		base[FactorSuccessRate] = 0.15
		base[FactorToolSupport] = 0.10
	}

	return base
}

// AutoStrategy auto 评分路由策略
type AutoStrategy struct {
	scorer *Scorer
}

// NewAutoStrategy 创建 auto 策略
func NewAutoStrategy(variant AutoVariant) *AutoStrategy {
	return &AutoStrategy{scorer: NewScorer(variant)}
}

// AutoCandidate auto 策略候选
type AutoCandidate struct {
	Model     string
	ChannelID string
	Stats     RuntimeStats
}

// Select 从候选列表中选择评分最高的
func (s *AutoStrategy) Select(candidates []AutoCandidate) (AutoCandidate, error) {
	if len(candidates) == 0 {
		return AutoCandidate{}, &RoutingError{Type: "no_candidates", Message: "no candidates available"}
	}

	bestScore := -1.0
	bestIdx := 0
	for i, c := range candidates {
		score := s.scorer.Score(c.Stats)
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return candidates[bestIdx], nil
}
