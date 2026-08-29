package optimizer

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

type Recommendation struct {
	Type         string  `json:"type"`
	FromModel    string  `json:"from_model"`
	ToModel      string  `json:"to_model"`
	ModelTag     string  `json:"model_tag"`
	ChannelID    string  `json:"channel_id"`
	ChannelName  string  `json:"channel_name"`
	SavingsUSD   float64 `json:"savings_usd"`
	QualityDiff  string  `json:"quality_diff"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
	Reason       string  `json:"reason"`
}

type OptimizationPlan struct {
	Recommendations []Recommendation `json:"recommendations"`
	MonthlySavings  float64          `json:"monthly_savings"`
	Strategy        string           `json:"strategy"`
	Mode            string           `json:"mode"`
	TaskType        string           `json:"task_type"`
}

const (
	ModeQualityFirst = "quality_first"
	ModeValueFirst   = "value_first"
	ModeAutoStrategy = "auto_strategy"
	TaskGeneralChat  = "general_chat"
	TaskToolCalling  = "tool_calling"
	TaskStructured   = "structured_output"
	TaskLongContext  = "long_context"
	TaskVision       = "vision"
	TaskBatchLowCost = "batch_low_cost"
)

type candidate struct {
	name  string
	price ModelPriceInfo
}

type Service struct {
	mu         sync.RWMutex
	configSvc  *config.Service
	usageSvc   *usage.Service
	pricing    map[string]ModelPriceInfo
	modelStats map[string]ModelStatsInfo
}

type ModelPriceInfo struct {
	InputPricePer1M  float64
	OutputPricePer1M float64
	Quality          int
	AvgLatencyMs     float64
	ErrorRate        float64
}

type ModelStatsInfo struct {
	Model         string
	TotalRequests int
	TotalErrors   int
	AvgLatencyMs  float64
	ErrorRate     float64
}

func NewService(configSvc *config.Service, usageSvc *usage.Service) *Service {
	svc := &Service{
		configSvc:  configSvc,
		usageSvc:   usageSvc,
		pricing:    initModelPricing(),
		modelStats: make(map[string]ModelStatsInfo),
	}
	return svc
}

var modelTagMap = map[string]string{
	"gpt-4o":                   "closed",
	"gpt-4o-mini":              "closed",
	"gpt-4-turbo":              "closed",
	"gpt-5":                    "closed",
	"gpt-5-mini":               "closed",
	"gpt-5-nano":               "closed",
	"claude-4-sonnet":          "closed",
	"claude-4-opus":            "closed",
	"claude-3-5-sonnet":        "closed",
	"claude-3-haiku":           "closed",
	"claude-3-opus":            "closed",
	"deepseek-chat":            "open",
	"deepseek-coder":           "open",
	"deepseek-v4-pro":          "open",
	"deepseek-v4-flash":        "open",
	"glm-5":                    "open",
	"glm-5.1":                  "open",
	"kimi-2.5":                 "open",
	"kimi-2.6":                 "open",
	"minimax-m2.7":             "open",
	"gemini-1.5-pro":           "closed",
	"gemini-1.5-flash":         "closed",
	"gemini-2.0-flash":         "closed",
	"gemini-2.5-pro":           "closed",
	"gemini-2.5-flash":         "closed",
	"qwen-max":                 "open",
	"qwen-plus":                "open",
	"mistral-large":            "open",
	"llama-3.1-70b":            "open",
	"llama-3.1-405b":           "open",
	"gpt-3.5-turbo":            "closed",
	"o3":                       "closed",
	"o3-mini":                  "closed",
	"o1":                       "closed",
	"o1-mini":                  "closed",
	"claude-3-5-haiku":         "closed",
	"claude-sonnet-4-20250514": "closed",
	"claude-opus-4-20250514":   "closed",
}

func initModelPricing() map[string]ModelPriceInfo {
	return map[string]ModelPriceInfo{
		"gpt-4o":            {InputPricePer1M: 2.50, OutputPricePer1M: 10.00, Quality: 10},
		"gpt-4o-mini":       {InputPricePer1M: 0.15, OutputPricePer1M: 0.60, Quality: 7},
		"gpt-4-turbo":       {InputPricePer1M: 10.00, OutputPricePer1M: 30.00, Quality: 9},
		"gpt-5":             {InputPricePer1M: 5.00, OutputPricePer1M: 20.00, Quality: 11},
		"gpt-5-mini":        {InputPricePer1M: 0.30, OutputPricePer1M: 1.20, Quality: 8},
		"gpt-5-nano":        {InputPricePer1M: 0.10, OutputPricePer1M: 0.40, Quality: 6},
		"claude-4-sonnet":   {InputPricePer1M: 4.00, OutputPricePer1M: 18.00, Quality: 11},
		"claude-4-opus":     {InputPricePer1M: 18.00, OutputPricePer1M: 80.00, Quality: 12},
		"claude-3-5-sonnet": {InputPricePer1M: 3.00, OutputPricePer1M: 15.00, Quality: 10},
		"claude-3-haiku":    {InputPricePer1M: 0.25, OutputPricePer1M: 1.25, Quality: 6},
		"claude-3-opus":     {InputPricePer1M: 15.00, OutputPricePer1M: 75.00, Quality: 10},
		"deepseek-chat":     {InputPricePer1M: 0.27, OutputPricePer1M: 1.10, Quality: 7},
		"deepseek-coder":    {InputPricePer1M: 0.14, OutputPricePer1M: 0.28, Quality: 6},
		"deepseek-v4-pro":   {InputPricePer1M: 0.50, OutputPricePer1M: 2.00, Quality: 9},
		"deepseek-v4-flash": {InputPricePer1M: 0.10, OutputPricePer1M: 0.40, Quality: 7},
		"glm-5":             {InputPricePer1M: 0.50, OutputPricePer1M: 2.00, Quality: 8},
		"glm-5.1":           {InputPricePer1M: 0.80, OutputPricePer1M: 3.00, Quality: 9},
		"kimi-2.5":          {InputPricePer1M: 0.60, OutputPricePer1M: 2.50, Quality: 8},
		"kimi-2.6":          {InputPricePer1M: 1.00, OutputPricePer1M: 4.00, Quality: 9},
		"minimax-m2.7":      {InputPricePer1M: 0.40, OutputPricePer1M: 1.50, Quality: 7},
		"gemini-1.5-pro":    {InputPricePer1M: 1.25, OutputPricePer1M: 5.00, Quality: 9},
		"gemini-1.5-flash":  {InputPricePer1M: 0.075, OutputPricePer1M: 0.30, Quality: 6},
		"gemini-2.0-flash":  {InputPricePer1M: 0.10, OutputPricePer1M: 0.40, Quality: 7},
		"gemini-2.5-pro":    {InputPricePer1M: 1.25, OutputPricePer1M: 10.00, Quality: 9},
		"gemini-2.5-flash":  {InputPricePer1M: 0.15, OutputPricePer1M: 0.60, Quality: 7},
		"qwen-max":          {InputPricePer1M: 2.00, OutputPricePer1M: 6.00, Quality: 8},
		"qwen-plus":         {InputPricePer1M: 0.80, OutputPricePer1M: 2.00, Quality: 7},
		"mistral-large":     {InputPricePer1M: 2.00, OutputPricePer1M: 6.00, Quality: 8},
		"llama-3.1-70b":     {InputPricePer1M: 0.59, OutputPricePer1M: 0.79, Quality: 7},
		"llama-3.1-405b":    {InputPricePer1M: 2.00, OutputPricePer1M: 6.00, Quality: 8},
	}
}

func (s *Service) GetOptimizationPlan(mode string, taskType string) *OptimizationPlan {
	if mode == "" {
		mode = ModeValueFirst
	}
	taskType = normalizeTaskType(taskType)
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels, _ := s.configSvc.ListChannels()
	usageRecords := s.usageSvc.GetAllRecords()
	modelStats := s.aggregateModelUsage(usageRecords)

	recommendations := make([]Recommendation, 0)
	recommendations = append(recommendations, s.findRuntimeParameterOptimizations(taskType)...)

	for modelName, stats := range modelStats {
		currentPrice, hasCurrent := s.pricing[modelName]
		if !hasCurrent {
			continue
		}
		recommendation := s.findBestAlternative(modelName, currentPrice, stats, channels, mode, taskType)
		if recommendation != nil {
			recommendation.Reason = decorateTaskTypeReason(recommendation.Reason, taskType)
			recommendations = append(recommendations, *recommendation)
		}
	}

	// Generate fallback chain enhancement recommendations
	fallbackRecs := s.findFallbackChainEnhancements(channels, taskType)
	recommendations = append(recommendations, fallbackRecs...)

	switch mode {
	case ModeQualityFirst:
		sort.Slice(recommendations, func(i, j int) bool {
			qi := s.pricing[recommendations[i].ToModel].Quality
			qj := s.pricing[recommendations[j].ToModel].Quality
			if qi != qj {
				return qi > qj
			}
			return recommendations[i].SavingsUSD > recommendations[j].SavingsUSD
		})
	case ModeAutoStrategy:
		sort.Slice(recommendations, func(i, j int) bool {
			scoreI := s.scoreValue(recommendations[i])
			scoreJ := s.scoreValue(recommendations[j])
			return scoreI > scoreJ
		})
	default:
		sort.Slice(recommendations, func(i, j int) bool {
			return recommendations[i].SavingsUSD > recommendations[j].SavingsUSD
		})
	}
	sortRecommendationsByTaskType(recommendations, taskType)

	totalSavings := 0.0
	for _, r := range recommendations {
		totalSavings += r.SavingsUSD
	}

	strategy := mode
	if mode == ModeAutoStrategy {
		if len(recommendations) > 0 && recommendations[0].SavingsUSD > 10 {
			strategy = "cost_optimized"
		} else {
			strategy = "balanced"
		}
	}

	return &OptimizationPlan{
		Recommendations: recommendations,
		MonthlySavings:  totalSavings,
		Strategy:        strategy,
		Mode:            mode,
		TaskType:        taskType,
	}
}

func (s *Service) scoreValue(r Recommendation) float64 {
	src, hasSrc := s.pricing[r.FromModel]
	dst, hasDst := s.pricing[r.ToModel]
	if !hasSrc || !hasDst {
		return r.SavingsUSD
	}
	qualityGap := dst.Quality - src.Quality
	costRatio := src.InputPricePer1M / math.Max(dst.InputPricePer1M, 0.001)
	latencyDelta := 0.0
	errorDelta := 0.0
	if src.AvgLatencyMs > 0 && dst.AvgLatencyMs > 0 {
		latencyDelta = (src.AvgLatencyMs - dst.AvgLatencyMs) / 100
	}
	if src.ErrorRate > 0 && dst.ErrorRate > 0 {
		errorDelta = (src.ErrorRate - dst.ErrorRate) * 50
	}
	return r.SavingsUSD*0.35 + float64(qualityGap)*2.0 + costRatio*3.0 + latencyDelta*1.0 + errorDelta*2.0
}

type modelAggregate struct {
	ModelName     string
	TotalRequests int
	TotalInput    int64
	TotalOutput   int64
	TotalCost     float64
	ChannelIDs    []string
}

func (s *Service) aggregateModelUsage(records []usage.UsageRecord) map[string]*modelAggregate {
	stats := make(map[string]*modelAggregate)
	for _, r := range records {
		key := r.Model
		if _, ok := stats[key]; !ok {
			stats[key] = &modelAggregate{ModelName: r.Model}
		}
		s := stats[key]
		s.TotalRequests += r.RequestCount
		s.TotalInput += r.InputTokens
		s.TotalOutput += r.OutputTokens
		s.TotalCost += r.CostUSD
		s.ChannelIDs = append(s.ChannelIDs, r.ChannelID)
	}
	return stats
}

func (s *Service) findBestAlternative(modelName string, currentPrice ModelPriceInfo, stats *modelAggregate, channels []config.Channel, mode string, taskType string) *Recommendation {
	var candidates []candidate
	for name, price := range s.pricing {
		if name == modelName {
			continue
		}
		candidates = append(candidates, candidate{name: name, price: price})
	}

	if len(candidates) == 0 {
		return nil
	}
	sortCandidatesByTaskType(candidates, taskType)

	switch mode {
	case ModeQualityFirst:
		return s.findQualityAlternative(modelName, currentPrice, stats, channels, candidates)
	case ModeValueFirst:
		return s.findValueAlternative(modelName, currentPrice, stats, channels, candidates)
	default:
		return s.findValueAlternative(modelName, currentPrice, stats, channels, candidates)
	}
}

func normalizeTaskType(taskType string) string {
	switch taskType {
	case TaskToolCalling, "tools":
		return TaskToolCalling
	case TaskStructured, "json":
		return TaskStructured
	case TaskLongContext:
		return TaskLongContext
	case TaskVision:
		return TaskVision
	default:
		return TaskGeneralChat
	}
}

func decorateTaskTypeReason(reason string, taskType string) string {
	switch taskType {
	case TaskToolCalling:
		return "工具调用场景优先考虑函数调用稳定性。" + reason
	case TaskStructured:
		return "结构化输出场景优先考虑 JSON 与格式稳定性。" + reason
	case TaskLongContext:
		return "长上下文场景优先考虑上下文容量与连续性。" + reason
	case TaskVision:
		return "视觉理解场景优先考虑多模态兼容性。" + reason
	default:
		return reason
	}
}

func sortRecommendationsByTaskType(recommendations []Recommendation, taskType string) {
	if len(recommendations) <= 1 || taskType == TaskGeneralChat {
		return
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		scoreI := taskTypeModelScore(recommendations[i].ToModel, taskType)
		scoreJ := taskTypeModelScore(recommendations[j].ToModel, taskType)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return recommendations[i].SavingsUSD > recommendations[j].SavingsUSD
	})
}

func sortCandidatesByTaskType(candidates []candidate, taskType string) {
	if len(candidates) <= 1 || taskType == TaskGeneralChat {
		return
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		scoreI := taskTypeModelScore(candidates[i].name, taskType)
		scoreJ := taskTypeModelScore(candidates[j].name, taskType)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		if candidates[i].price.Quality != candidates[j].price.Quality {
			return candidates[i].price.Quality > candidates[j].price.Quality
		}
		return candidates[i].price.InputPricePer1M < candidates[j].price.InputPricePer1M
	})
}

func taskTypeModelScore(modelName string, taskType string) int {
	name := strings.ToLower(modelName)
	switch taskType {
	case TaskToolCalling:
		switch {
		case strings.HasPrefix(name, "gpt-4o"), strings.HasPrefix(name, "gpt-5"), strings.HasPrefix(name, "claude-4"), strings.HasPrefix(name, "claude-3-5-sonnet"), strings.HasPrefix(name, "gemini-2.5"), strings.HasPrefix(name, "gemini-2.0"):
			return 5
		case strings.HasPrefix(name, "deepseek"), strings.HasPrefix(name, "glm-5"), strings.HasPrefix(name, "qwen-max"):
			return 3
		default:
			return 1
		}
	case TaskStructured:
		switch {
		case strings.HasPrefix(name, "gpt-4o"), strings.HasPrefix(name, "gpt-4.1"), strings.HasPrefix(name, "gpt-5"), strings.HasPrefix(name, "claude-4"), strings.HasPrefix(name, "claude-3-5"), strings.HasPrefix(name, "gemini-2.5"):
			return 5
		case strings.HasPrefix(name, "gemini-2.0"), strings.HasPrefix(name, "deepseek"), strings.HasPrefix(name, "qwen"), strings.HasPrefix(name, "glm-5"):
			return 3
		default:
			return 1
		}
	case TaskLongContext:
		switch {
		case strings.HasPrefix(name, "gemini-2.5"), strings.HasPrefix(name, "gemini-1.5"), strings.HasPrefix(name, "claude-4"), strings.HasPrefix(name, "claude-3-5"), strings.HasPrefix(name, "kimi"), strings.HasPrefix(name, "gpt-4.1"):
			return 5
		case strings.HasPrefix(name, "gpt-4o"), strings.HasPrefix(name, "glm-5"), strings.HasPrefix(name, "qwen"):
			return 3
		default:
			return 1
		}
	case TaskVision:
		switch {
		case strings.HasPrefix(name, "gpt-4o"), strings.HasPrefix(name, "gemini-2.5"), strings.HasPrefix(name, "gemini-2.0"), strings.HasPrefix(name, "claude-4"), strings.HasPrefix(name, "claude-3-5-sonnet"):
			return 5
		case strings.HasPrefix(name, "gpt-4.1"), strings.HasPrefix(name, "qwen"):
			return 2
		default:
			return 0
		}
	default:
		return 0
	}
}

func (s *Service) findQualityAlternative(modelName string, currentPrice ModelPriceInfo, stats *modelAggregate, channels []config.Channel, candidates []candidate) *Recommendation {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].price.Quality != candidates[j].price.Quality {
			return candidates[i].price.Quality > candidates[j].price.Quality
		}
		return candidates[i].price.InputPricePer1M < candidates[j].price.InputPricePer1M
	})
	best := candidates[0]
	if best.price.Quality <= currentPrice.Quality {
		return nil
	}
	monthlyInput := float64(stats.TotalInput) / 1000000
	monthlyOutput := float64(stats.TotalOutput) / 1000000
	extraCost := (monthlyInput*best.price.InputPricePer1M + monthlyOutput*best.price.OutputPricePer1M) -
		(monthlyInput*currentPrice.InputPricePer1M + monthlyOutput*currentPrice.OutputPricePer1M)
	channelID, channelName := s.resolveChannel(stats, channels)
	reason := fmt.Sprintf("升级到质量更高的 %s（Quality %d），每月仅增加约 $%.2f", best.name, best.price.Quality, math.Max(extraCost, 0))

	targetStats, hasTS := s.modelStats[best.name]
	srcStats, hasSS := s.modelStats[modelName]
	if hasTS {
		reason += fmt.Sprintf("，延迟 %s", formatLatency(targetStats.AvgLatencyMs))
		if hasSS && srcStats.AvgLatencyMs > 0 {
			reason += fmt.Sprintf("（当前 %s）", formatLatency(srcStats.AvgLatencyMs))
		}
		if targetStats.ErrorRate > 0 {
			reason += fmt.Sprintf("，错误率 %s", formatErrorRate(targetStats.ErrorRate))
		}
	}
	if hasSS && srcStats.ErrorRate > 0.05 {
		reason += fmt.Sprintf("，当前模型错误率 %s", formatErrorRate(srcStats.ErrorRate))
	}

	rec := &Recommendation{
		Type:        "model_replacement",
		FromModel:   modelName,
		ToModel:     best.name,
		ModelTag:    modelTag(best.name),
		ChannelID:   channelID,
		ChannelName: channelName,
		SavingsUSD:  -extraCost,
		QualityDiff: "better",
		Reason:      reason,
	}
	if hasTS {
		rec.AvgLatencyMs = targetStats.AvgLatencyMs
		rec.ErrorRate = targetStats.ErrorRate
	}
	return rec
}

func (s *Service) findValueAlternative(modelName string, currentPrice ModelPriceInfo, stats *modelAggregate, channels []config.Channel, candidates []candidate) *Recommendation {
	var cheaperBest *candidate
	var upgradeBest *candidate
	var openBest *candidate

	for _, c := range candidates {
		if c.price.Quality >= currentPrice.Quality && c.price.InputPricePer1M < currentPrice.InputPricePer1M {
			if cheaperBest == nil || c.price.InputPricePer1M < cheaperBest.price.InputPricePer1M {
				cheaperBest = &c
			}
		}
		if c.price.Quality > currentPrice.Quality && c.price.InputPricePer1M <= currentPrice.InputPricePer1M*1.5 {
			if upgradeBest == nil || c.price.Quality > upgradeBest.price.Quality {
				upgradeBest = &c
			}
		}
		tag := modelTag(c.name)
		if tag == "open" && c.price.Quality >= currentPrice.Quality && c.price.InputPricePer1M < currentPrice.InputPricePer1M {
			if openBest == nil || c.price.InputPricePer1M < openBest.price.InputPricePer1M {
				openBest = &c
			}
		}
	}

	monthlyInput := float64(stats.TotalInput) / 1000000
	monthlyOutput := float64(stats.TotalOutput) / 1000000
	channelID, channelName := s.resolveChannel(stats, channels)

	buildReason := func(targetName string, base string) string {
		targetStats, hasTS := s.modelStats[targetName]
		srcStats, hasSS := s.modelStats[modelName]
		if hasTS {
			base += fmt.Sprintf("，延迟 %s", formatLatency(targetStats.AvgLatencyMs))
			if hasSS && srcStats.AvgLatencyMs > 0 {
				base += fmt.Sprintf("（当前 %s）", formatLatency(srcStats.AvgLatencyMs))
			}
			if targetStats.ErrorRate > 0 {
				base += fmt.Sprintf("，错误率 %s", formatErrorRate(targetStats.ErrorRate))
			}
		}
		if hasSS && srcStats.ErrorRate > 0.05 {
			base += fmt.Sprintf("，当前模型错误率 %s", formatErrorRate(srcStats.ErrorRate))
		}
		return base
	}

	fillStats := func(rec *Recommendation, targetName string) {
		if ts, ok := s.modelStats[targetName]; ok {
			rec.AvgLatencyMs = ts.AvgLatencyMs
			rec.ErrorRate = ts.ErrorRate
		}
	}

	if openBest != nil {
		savings := (monthlyInput*currentPrice.InputPricePer1M + monthlyOutput*currentPrice.OutputPricePer1M) -
			(monthlyInput*openBest.price.InputPricePer1M + monthlyOutput*openBest.price.OutputPricePer1M)
		rec := &Recommendation{
			Type:        "model_replacement",
			FromModel:   modelName,
			ToModel:     openBest.name,
			ModelTag:    "open",
			ChannelID:   channelID,
			ChannelName: channelName,
			SavingsUSD:  savings,
			QualityDiff: "equivalent_or_better",
			Reason:      buildReason(openBest.name, fmt.Sprintf("切换到开放模型 %s 可节省约 $%.2f/月，质量不低于当前模型", openBest.name, savings)),
		}
		fillStats(rec, openBest.name)
		return rec
	}

	if cheaperBest != nil {
		savings := (monthlyInput*currentPrice.InputPricePer1M + monthlyOutput*currentPrice.OutputPricePer1M) -
			(monthlyInput*cheaperBest.price.InputPricePer1M + monthlyOutput*cheaperBest.price.OutputPricePer1M)
		rec := &Recommendation{
			Type:        "model_replacement",
			FromModel:   modelName,
			ToModel:     cheaperBest.name,
			ModelTag:    modelTag(cheaperBest.name),
			ChannelID:   channelID,
			ChannelName: channelName,
			SavingsUSD:  savings,
			QualityDiff: "equivalent_or_better",
			Reason:      buildReason(cheaperBest.name, fmt.Sprintf("切换到 %s 可节省约 $%.2f/月，质量不低于当前模型", cheaperBest.name, savings)),
		}
		fillStats(rec, cheaperBest.name)
		return rec
	}

	if upgradeBest != nil {
		extraCost := (monthlyInput*upgradeBest.price.InputPricePer1M + monthlyOutput*upgradeBest.price.OutputPricePer1M) -
			(monthlyInput*currentPrice.InputPricePer1M + monthlyOutput*currentPrice.OutputPricePer1M)
		rec := &Recommendation{
			Type:        "model_replacement",
			FromModel:   modelName,
			ToModel:     upgradeBest.name,
			ModelTag:    modelTag(upgradeBest.name),
			ChannelID:   channelID,
			ChannelName: channelName,
			SavingsUSD:  -extraCost,
			QualityDiff: "better",
			Reason:      buildReason(upgradeBest.name, fmt.Sprintf("升级到 %s 仅增加约 $%.2f/月，但获得更高质量", upgradeBest.name, extraCost)),
		}
		fillStats(rec, upgradeBest.name)
		return rec
	}

	return nil
}

func (s *Service) resolveChannel(stats *modelAggregate, channels []config.Channel) (string, string) {
	if len(stats.ChannelIDs) == 0 {
		return "", ""
	}
	for _, ch := range channels {
		if ch.ChannelID == stats.ChannelIDs[0] {
			return ch.ChannelID, ch.DisplayName
		}
	}
	return "", ""
}

func modelTag(name string) string {
	if tag, ok := modelTagMap[name]; ok {
		return tag
	}
	for k, v := range modelTagMap {
		if strings.HasPrefix(name, k) {
			return v
		}
	}
	return "closed"
}

func (s *Service) SetCloudModelStats(stats []ModelStatsInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelStats = make(map[string]ModelStatsInfo)
	for _, st := range stats {
		s.modelStats[st.Model] = st
	}
}

func (s *Service) GetCloudModelStats() []ModelStatsInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ModelStatsInfo, 0, len(s.modelStats))
	for _, v := range s.modelStats {
		result = append(result, v)
	}
	return result
}

func hasLatencyIssue(info ModelStatsInfo) bool {
	return info.AvgLatencyMs > 3000 || info.ErrorRate > 0.1
}

func formatLatency(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0fms", ms)
	}
	return fmt.Sprintf("%.1fs", ms/1000)
}

func formatErrorRate(rate float64) string {
	return fmt.Sprintf("%.1f%%", rate*100)
}

// findRuntimeParameterOptimizations generates lightweight runtime-parameter recommendations.
// It does not mutate runtime defaults; it only surfaces guidance in the optimization plan.
func (s *Service) findRuntimeParameterOptimizations(taskType string) []Recommendation {
	recs := make([]Recommendation, 0, 2)
	switch taskType {
	case TaskStructured:
		recs = append(recs, Recommendation{
			Type:        "runtime_params",
			FromModel:   "runtime",
			ToModel:     "structured_output",
			SavingsUSD:  0,
			QualityDiff: "equivalent_or_better",
			Reason:      "结构化输出场景建议降低 temperature 并显式设置 response_format/json schema，以提升稳定性。",
		})
	case TaskLongContext:
		recs = append(recs, Recommendation{
			Type:        "runtime_params",
			FromModel:   "runtime",
			ToModel:     "long_context",
			SavingsUSD:  0,
			QualityDiff: "equivalent_or_better",
			Reason:      "长上下文场景建议控制 max_tokens 并适度降低 temperature，减少截断和长响应成本。",
		})
	case TaskBatchLowCost:
		recs = append(recs, Recommendation{
			Type:        "runtime_params",
			FromModel:   "runtime",
			ToModel:     "batch_low_cost",
			SavingsUSD:  0,
			QualityDiff: "equivalent_or_better",
			Reason:      "批量低成本场景建议降低 max_tokens 上限，并优先使用更保守的采样参数以减少成本波动。",
		})
	case TaskToolCalling:
		recs = append(recs, Recommendation{
			Type:        "runtime_params",
			FromModel:   "runtime",
			ToModel:     "tool_calling",
			SavingsUSD:  0,
			QualityDiff: "equivalent_or_better",
			Reason:      "工具调用场景建议降低 temperature，并限制无关输出，提升 function calling 命中率。",
		})
	}
	return recs
}

// findFallbackChainEnhancements generates recommendations for improving existing combo fallback chains.
// For combos with only 1 step, suggests adding a backup step.
// For combos with 2 steps, suggests adding a last_resort step.
// Returns recommendations with type "fallback_chain".
func (s *Service) findFallbackChainEnhancements(channels []config.Channel, taskType string) []Recommendation {
	combos := s.configSvc.ListModelCombos()
	recommendations := make([]Recommendation, 0, len(combos))

	for _, combo := range combos {
		steps := combo.Steps
		if len(steps) == 0 {
			continue
		}

		// Suggest filling missing roles in the fallback chain
		hasPrimary := false
		hasBackup := false
		hasLastResort := false
		for _, step := range steps {
			switch step.StepRole {
			case "primary":
				hasPrimary = true
			case "backup":
				hasBackup = true
			case "last_resort":
				hasLastResort = true
			}
		}

		// If the combo has only 1 step and no backup, suggest adding a backup
		if len(steps) == 1 && !hasBackup && !hasLastResort {
			primaryModel := steps[0].Model
			// Find a cheaper/simpler alternative model as backup
			backupModel := findBackupModel(primaryModel, s.pricing)
			if backupModel != "" {
				recommendations = append(recommendations, Recommendation{
					Type:        "fallback_chain",
					FromModel:   combo.Name,
					ToModel:     backupModel,
					SavingsUSD:  0,
					QualityDiff: "equivalent_or_better",
					Reason:      fmt.Sprintf("Combo「%s」仅有一个步骤，建议添加回退模型 %s 作为 backup 步骤，提高可用性", combo.Name, backupModel),
				})
			}
		}

		// If the combo has 2 steps but no last_resort, suggest adding one
		if len(steps) == 2 && !hasLastResort {
			lastModel := steps[len(steps)-1].Model
			backupModel := findBackupModel(lastModel, s.pricing)
			if backupModel != "" {
				recommendations = append(recommendations, Recommendation{
					Type:        "fallback_chain",
					FromModel:   combo.Name,
					ToModel:     backupModel,
					SavingsUSD:  0,
					QualityDiff: "equivalent_or_better",
					Reason:      fmt.Sprintf("Combo「%s」仅有 2 个步骤，建议添加 %s 作为 last_resort 保底步骤，防止全部回退失败", combo.Name, backupModel),
				})
			}
		}

		// If the combo has no primary step role assigned, suggest a role assignment
		if !hasPrimary && len(steps) > 0 {
			recommendations = append(recommendations, Recommendation{
				Type:        "fallback_chain",
				FromModel:   combo.Name,
				ToModel:     steps[0].Model,
				SavingsUSD:  0,
				QualityDiff: "equivalent_or_better",
				Reason:      fmt.Sprintf("Combo「%s」的步骤缺少角色语义(primary/backup/last_resort)，建议分配角色以优化回退顺序", combo.Name),
			})
		}
	}

	return recommendations
}

// findBackupModel finds a suitable backup model for a given primary model.
// Returns a cheaper or simpler model that can serve as fallback.
func findBackupModel(primaryModel string, pricing map[string]ModelPriceInfo) string {
	primaryPrice, hasPrimary := pricing[primaryModel]
	if !hasPrimary {
		return ""
	}

	// Look for a cheaper model with similar or acceptable quality
	var bestMatch string
	var bestScore float64

	for model, price := range pricing {
		if model == primaryModel {
			continue
		}
		// Prefer models that are cheaper and have reasonable quality
		if price.InputPricePer1M < primaryPrice.InputPricePer1M && price.Quality >= primaryPrice.Quality-1 {
			score := (primaryPrice.InputPricePer1M - price.InputPricePer1M) * float64(price.Quality+1)
			if score > bestScore {
				bestScore = score
				bestMatch = model
			}
		}
	}

	if bestMatch == "" {
		// Fallback: any cheaper model
		for model, price := range pricing {
			if model == primaryModel {
				continue
			}
			if price.InputPricePer1M < primaryPrice.InputPricePer1M {
				if bestMatch == "" || price.InputPricePer1M < pricing[bestMatch].InputPricePer1M {
					bestMatch = model
				}
			}
		}
	}

	return bestMatch
}
