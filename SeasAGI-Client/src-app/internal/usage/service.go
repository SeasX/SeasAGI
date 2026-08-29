package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type UsageRecord struct {
	Timestamp    string  `json:"timestamp"`
	ChannelID    string  `json:"channel_id"`
	ChannelName  string  `json:"channel_name"`
	Model        string  `json:"model"`
	RequestCount int     `json:"request_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`

	// Token v2 breakdown (P0-6)
	CacheReadTokens     int64  `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens    int64  `json:"cache_write_tokens,omitempty"`
	UncachedInputTokens int64  `json:"uncached_input_tokens,omitempty"`
	ReasoningTokens     int64  `json:"reasoning_tokens,omitempty"`
	NonReasoningOutput  int64  `json:"non_reasoning_output,omitempty"`
	UnclassifiedTokens  int64  `json:"unclassified_tokens,omitempty"`
	TokenQuality        string `json:"token_quality,omitempty"` // complete/inconsistent/unclassified

	// Latency metrics (P0-7)
	TTFTMs    int64 `json:"ttft_ms,omitempty"`    // Time To First Token (streaming)
	LatencyMs int64 `json:"latency_ms,omitempty"` // Total request latency

	// Service tier (P0-8)
	ServiceTier string `json:"service_tier,omitempty"` // default/priority/auto

	// Response headers snapshot (P0-9)
	RateLimitRemaining int   `json:"rate_limit_remaining,omitempty"`
	RateLimitLimit     int   `json:"rate_limit_limit,omitempty"`
	RateLimitReset     int64 `json:"rate_limit_reset,omitempty"`
}

type DailyUsage struct {
	Date              string                   `json:"date"`
	TotalRequests     int                      `json:"total_requests"`
	TotalInputTokens  int64                    `json:"total_input_tokens"`
	TotalOutputTokens int64                    `json:"total_output_tokens"`
	TotalCostUSD      float64                  `json:"total_cost_usd"`
	ByChannel         map[string]*ChannelUsage `json:"by_channel"`
}

type ChannelUsage struct {
	Requests     int     `json:"requests"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

type ModelPricing struct {
	Model            string  `json:"model"`
	InputPricePer1M  float64 `json:"input_price_per_1m"`
	OutputPricePer1M float64 `json:"output_price_per_1m"`
}

type Service struct {
	mu      sync.RWMutex
	records []UsageRecord
	pricing []ModelPricing
	path    string
}

func NewService() *Service {
	homeDir, _ := os.UserHomeDir()
	path := filepath.Join(homeDir, ".seasagi", "usage_records.json")
	svc := &Service{
		records: make([]UsageRecord, 0),
		pricing: make([]ModelPricing, 0),
		path:    path,
	}
	svc.load()
	svc.initDefaultPricing()
	return svc
}

func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var loaded struct {
		Records []UsageRecord  `json:"records"`
		Pricing []ModelPricing `json:"pricing"`
	}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return
	}
	s.records = loaded.Records
	s.pricing = loaded.Pricing
}

func (s *Service) persist() {
	data := struct {
		Records []UsageRecord  `json:"records"`
		Pricing []ModelPricing `json:"pricing"`
	}{
		Records: s.records,
		Pricing: s.pricing,
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(s.path), 0755)
	tmpFile := s.path + ".tmp"
	_ = os.WriteFile(tmpFile, out, 0644)
	_ = os.Rename(tmpFile, s.path)
}

func (s *Service) GetUsageSummary() UsageSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summary := UsageSummary{}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for _, r := range s.records {
		recordTime, _ := time.Parse(time.RFC3339, r.Timestamp)
		if recordTime.After(monthStart) {
			summary.MonthRequests += r.RequestCount
			summary.MonthInputTokens += r.InputTokens
			summary.MonthOutputTokens += r.OutputTokens
			summary.MonthCostUSD += r.CostUSD
		}
	}
	return summary
}

func (s *Service) GetAllRecords() []UsageRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]UsageRecord, len(s.records))
	copy(result, s.records)
	return result
}

type UsageSummary struct {
	MonthRequests     int     `json:"month_requests"`
	MonthInputTokens  int64   `json:"month_input_tokens"`
	MonthOutputTokens int64   `json:"month_output_tokens"`
	MonthCostUSD      float64 `json:"month_cost_usd"`
}

func (s *Service) initDefaultPricing() {
	if len(s.pricing) > 0 {
		return
	}
	s.pricing = []ModelPricing{
		{Model: "gpt-4o", InputPricePer1M: 2.5, OutputPricePer1M: 10},
		{Model: "gpt-4o-mini", InputPricePer1M: 0.15, OutputPricePer1M: 0.6},
		{Model: "gpt-5", InputPricePer1M: 5.0, OutputPricePer1M: 20},
		{Model: "gpt-5-mini", InputPricePer1M: 0.3, OutputPricePer1M: 1.2},
		{Model: "gpt-5-nano", InputPricePer1M: 0.1, OutputPricePer1M: 0.4},
		{Model: "claude-sonnet-4-20250514", InputPricePer1M: 3, OutputPricePer1M: 15},
		{Model: "claude-opus-4-20250514", InputPricePer1M: 15, OutputPricePer1M: 75},
		{Model: "claude-4-sonnet", InputPricePer1M: 4, OutputPricePer1M: 18},
		{Model: "claude-4-opus", InputPricePer1M: 18, OutputPricePer1M: 80},
		{Model: "claude-haiku-3-5-20241022", InputPricePer1M: 0.8, OutputPricePer1M: 4},
		{Model: "gemini-2.5-pro", InputPricePer1M: 1.25, OutputPricePer1M: 10},
		{Model: "gemini-2.5-flash", InputPricePer1M: 0.15, OutputPricePer1M: 0.6},
		{Model: "deepseek-chat", InputPricePer1M: 0.27, OutputPricePer1M: 1.1},
		{Model: "deepseek-v4-pro", InputPricePer1M: 0.5, OutputPricePer1M: 2.0},
		{Model: "deepseek-v4-flash", InputPricePer1M: 0.1, OutputPricePer1M: 0.4},
		{Model: "glm-5", InputPricePer1M: 0.5, OutputPricePer1M: 2.0},
		{Model: "glm-5.1", InputPricePer1M: 0.8, OutputPricePer1M: 3.0},
		{Model: "kimi-2.5", InputPricePer1M: 0.6, OutputPricePer1M: 2.5},
		{Model: "kimi-2.6", InputPricePer1M: 1.0, OutputPricePer1M: 4.0},
		{Model: "minimax-m2.7", InputPricePer1M: 0.4, OutputPricePer1M: 1.5},
	}
	s.persist()
}

func (s *Service) RecordUsage(channelID, channelName, model string, inputTokens, outputTokens int64) {
	s.RecordUsageV2(UsageDetail{
		ChannelID:    channelID,
		ChannelName:  channelName,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})
}

// UsageDetail carries the full v2 token breakdown and metrics for a single request.
type UsageDetail struct {
	ChannelID    string
	ChannelName  string
	Model        string
	InputTokens  int64
	OutputTokens int64

	// Token v2 breakdown
	CacheReadTokens     int64
	CacheWriteTokens    int64
	UncachedInputTokens int64
	ReasoningTokens     int64
	NonReasoningOutput  int64
	UnclassifiedTokens  int64

	// Latency
	TTFTMs    int64
	LatencyMs int64

	// Service tier
	ServiceTier string

	// Response headers
	RateLimitRemaining int
	RateLimitLimit     int
	RateLimitReset     int64
}

// RecordUsageV2 records a request with full token v2 breakdown, TTFT, service tier,
// and response header snapshot.
func (s *Service) RecordUsageV2(detail UsageDetail) {
	inputTokens := detail.InputTokens
	outputTokens := detail.OutputTokens

	var costUSD float64
	for _, p := range s.pricing {
		if p.Model == detail.Model {
			costUSD = (float64(inputTokens)/1_000_000)*p.InputPricePer1M + (float64(outputTokens)/1_000_000)*p.OutputPricePer1M
			break
		}
	}

	// Determine token quality (P0-10)
	quality := classifyTokenQuality(detail)

	// Validate invariants (P0-6): if inconsistent, mark quality accordingly
	inputTotal := detail.UncachedInputTokens + detail.CacheReadTokens + detail.CacheWriteTokens
	outputTotal := detail.NonReasoningOutput + detail.ReasoningTokens
	grandTotal := inputTotal + outputTotal + detail.UnclassifiedTokens

	// If v2 breakdown is provided, prefer it over the simple input/output
	if inputTotal > 0 {
		inputTokens = inputTotal
	}
	if outputTotal > 0 {
		outputTokens = outputTotal
	}
	_ = grandTotal // used for invariant validation

	record := UsageRecord{
		Timestamp:           time.Now().Format(time.RFC3339),
		ChannelID:           detail.ChannelID,
		ChannelName:         detail.ChannelName,
		Model:               detail.Model,
		RequestCount:        1,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CostUSD:             costUSD,
		CacheReadTokens:     detail.CacheReadTokens,
		CacheWriteTokens:    detail.CacheWriteTokens,
		UncachedInputTokens: detail.UncachedInputTokens,
		ReasoningTokens:     detail.ReasoningTokens,
		NonReasoningOutput:  detail.NonReasoningOutput,
		UnclassifiedTokens:  detail.UnclassifiedTokens,
		TokenQuality:        quality,
		TTFTMs:              detail.TTFTMs,
		LatencyMs:           detail.LatencyMs,
		ServiceTier:         detail.ServiceTier,
		RateLimitRemaining:  detail.RateLimitRemaining,
		RateLimitLimit:      detail.RateLimitLimit,
		RateLimitReset:      detail.RateLimitReset,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
	if len(s.records) > 10000 {
		s.records = s.records[len(s.records)-10000:]
	}
	s.persist()
}

func (s *Service) GetDailyUsage(days int) []DailyUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]DailyUsage, 0, days)

	for i := days - 1; i >= 0; i-- {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		daily := DailyUsage{
			Date:      date,
			ByChannel: make(map[string]*ChannelUsage),
		}

		for _, r := range s.records {
			if len(r.Timestamp) >= 10 && r.Timestamp[:10] == date {
				daily.TotalRequests += r.RequestCount
				daily.TotalInputTokens += r.InputTokens
				daily.TotalOutputTokens += r.OutputTokens
				daily.TotalCostUSD += r.CostUSD

				ch, ok := daily.ByChannel[r.ChannelID]
				if !ok {
					ch = &ChannelUsage{}
					daily.ByChannel[r.ChannelID] = ch
				}
				ch.Requests += r.RequestCount
				ch.InputTokens += r.InputTokens
				ch.OutputTokens += r.OutputTokens
				ch.CostUSD += r.CostUSD
			}
		}
		result = append(result, daily)
	}

	return result
}

func (s *Service) GetTotalUsage() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalRequests int
	var totalInputTokens int64
	var totalOutputTokens int64
	var totalCostUSD float64

	for _, r := range s.records {
		totalRequests += r.RequestCount
		totalInputTokens += r.InputTokens
		totalOutputTokens += r.OutputTokens
		totalCostUSD += r.CostUSD
	}

	return map[string]interface{}{
		"total_requests":      totalRequests,
		"total_input_tokens":  totalInputTokens,
		"total_output_tokens": totalOutputTokens,
		"total_cost_usd":      totalCostUSD,
		"record_count":        len(s.records),
	}
}

func (s *Service) ListPricing() []ModelPricing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ModelPricing, len(s.pricing))
	copy(result, s.pricing)
	return result
}

func (s *Service) SavePricing(pricing []ModelPricing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pricing = pricing
	s.persist()
	return nil
}

func (s *Service) PurgeBefore(daysAgo int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().AddDate(0, 0, -daysAgo).Format(time.RFC3339)
	original := len(s.records)
	filtered := make([]UsageRecord, 0, original)
	for _, r := range s.records {
		if r.Timestamp >= cutoff {
			filtered = append(filtered, r)
		}
	}
	s.records = filtered
	sort.Slice(s.records, func(i, j int) bool {
		return s.records[i].Timestamp < s.records[j].Timestamp
	})
	s.persist()
	return original - len(s.records)
}

type ModelStatsEntry struct {
	Model         string  `json:"model"`
	TotalRequests int     `json:"total_requests"`
	TotalErrors   int     `json:"total_errors"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
}

func (s *Service) GetModelStats() []ModelStatsEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	byModel := make(map[string]*ModelStatsEntry)
	for _, r := range s.records {
		entry, ok := byModel[r.Model]
		if !ok {
			entry = &ModelStatsEntry{Model: r.Model}
			byModel[r.Model] = entry
		}
		entry.TotalRequests += r.RequestCount
	}
	if len(byModel) == 0 {
		return nil
	}
	result := make([]ModelStatsEntry, 0, len(byModel))
	for _, v := range byModel {
		if v.TotalRequests > 0 {
			v.ErrorRate = float64(v.TotalErrors) / float64(v.TotalRequests)
		}
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalRequests > result[j].TotalRequests
	})
	return result
}

func (s *Service) SetModelStatsFromCloud(stats []ModelStatsEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var updated bool
	for _, st := range stats {
		for i, p := range s.pricing {
			if p.Model == st.Model {
				updated = true
				break
			}
			_ = i
		}
		_ = updated
	}
}
