package usage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// maxRecords caps how many in-memory records are retained; the oldest are
// dropped once the cap is exceeded (and the file is compacted).
const maxRecords = 10000

// Fallback pricing (USD per 1M tokens) used when a model is not present in the
// pricing table. The record is flagged via UsageRecord.CostEstimated.
const (
	fallbackInputPricePer1M  = 1.0
	fallbackOutputPricePer1M = 1.0
)

// Cache token price ratios applied to the model's input price. Providers bill
// prompt-cache reads at a steep discount and cache writes at a premium.
const (
	cacheReadPriceRatio  = 0.1
	cacheWritePriceRatio = 1.25
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
	// CostEstimated is true when the model was not found in the pricing table and
	// a fallback price was applied.
	CostEstimated bool `json:"cost_estimated,omitempty"`

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

// persistLine is one line of the append-only usage log. Exactly one of the two
// fields is set per line.
type persistLine struct {
	Record  *UsageRecord   `json:"record,omitempty"`
	Pricing []ModelPricing `json:"pricing,omitempty"`
}

// load reads the usage log. New files are JSONL (one persistLine per line); a
// legacy single-JSON snapshot ({records:[...],pricing:[...]}) is still accepted
// for backward compatibility.
func (s *Service) load() {
	data, err := os.ReadFile(s.path)
	if err != nil || len(data) == 0 {
		return
	}
	if records, pricing, ok := parseJSONL(data); ok {
		s.records = records
		if len(pricing) > 0 {
			s.pricing = pricing
		}
		return
	}
	// Legacy snapshot fallback.
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

// parseJSONL decodes the whole file as JSONL. It returns ok=false (without side
// effects) as soon as any line is not a standalone JSON object, which signals a
// legacy pretty-printed snapshot.
func parseJSONL(data []byte) ([]UsageRecord, []ModelPricing, bool) {
	records := make([]UsageRecord, 0)
	var pricing []ModelPricing
	sawLine := false
	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		var pl persistLine
		if err := json.Unmarshal(line, &pl); err != nil {
			return nil, nil, false
		}
		sawLine = true
		if pl.Record != nil {
			records = append(records, *pl.Record)
		}
		if len(pl.Pricing) > 0 {
			pricing = pl.Pricing
		}
	}
	return records, pricing, sawLine
}

// appendRecordLocked incrementally appends a single record to the log. The
// caller must hold s.mu.
func (s *Service) appendRecordLocked(record UsageRecord) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return
	}
	line, err := json.Marshal(persistLine{Record: &record})
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// persist rewrites the whole log atomically (tmp file + rename). Used on
// initialisation, pricing changes, purge and compaction.
func (s *Service) persist() {
	var buf bytes.Buffer
	writeLine := func(pl persistLine) {
		line, err := json.Marshal(pl)
		if err != nil {
			return
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	writeLine(persistLine{Pricing: s.pricing})
	for i := range s.records {
		writeLine(persistLine{Record: &s.records[i]})
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return
	}
	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		return
	}
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

// defaultPricing is the single source of truth for model prices (USD per 1M
// tokens, split by input/output). config.ModelCost derives its routing scalar
// from this table so the two price tables can no longer diverge.
var defaultPricing = []ModelPricing{
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

// lookupIn finds the pricing entry for model using exact match first, then the
// longest prefix match (so "gpt-4o-mini-2024" resolves to gpt-4o-mini, not gpt-4o).
func lookupIn(pricing []ModelPricing, model string) (ModelPricing, bool) {
	for _, p := range pricing {
		if p.Model == model {
			return p, true
		}
	}
	var best ModelPricing
	bestLen := -1
	for _, p := range pricing {
		if strings.HasPrefix(model, p.Model) && len(p.Model) > bestLen {
			best = p
			bestLen = len(p.Model)
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return ModelPricing{}, false
}

// LookupPricing returns the canonical pricing entry for a model. It is used by
// other packages (e.g. config routing) so that all cost estimates share one table.
func LookupPricing(model string) (ModelPricing, bool) {
	return lookupIn(defaultPricing, model)
}

func (s *Service) initDefaultPricing() {
	if len(s.pricing) > 0 {
		return
	}
	s.pricing = append([]ModelPricing(nil), defaultPricing...)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	pricing, found := lookupIn(s.pricing, detail.Model)
	if !found {
		// Unknown model: apply a fallback price and flag the record so the
		// estimated cost is visible downstream instead of silently reading 0.
		pricing = ModelPricing{
			Model:            detail.Model,
			InputPricePer1M:  fallbackInputPricePer1M,
			OutputPricePer1M: fallbackOutputPricePer1M,
		}
	}

	costUSD := estimateCost(detail, pricing)

	// Determine token quality (P0-10)
	quality := classifyTokenQuality(detail)

	inputTokens := detail.InputTokens
	outputTokens := detail.OutputTokens
	if inputTotal := detail.UncachedInputTokens + detail.CacheReadTokens + detail.CacheWriteTokens; inputTotal > 0 {
		inputTokens = inputTotal
	}
	if outputTotal := detail.NonReasoningOutput + detail.ReasoningTokens; outputTotal > 0 {
		outputTokens = outputTotal
	}

	record := UsageRecord{
		Timestamp:           time.Now().Format(time.RFC3339),
		ChannelID:           detail.ChannelID,
		ChannelName:         detail.ChannelName,
		Model:               detail.Model,
		RequestCount:        1,
		InputTokens:         inputTokens,
		OutputTokens:        outputTokens,
		CostUSD:             costUSD,
		CostEstimated:       !found,
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

	s.records = append(s.records, record)
	if len(s.records) > maxRecords {
		// Compaction: drop the oldest and rewrite the log atomically.
		s.records = s.records[len(s.records)-maxRecords:]
		s.persist()
		return
	}
	// Incremental append: only the new record is written, avoiding an O(n)
	// full rewrite on every single request.
	s.appendRecordLocked(record)
}

// estimateCost computes the USD cost of a request. When the granular v2 token
// breakdown is available it is priced component by component (with prompt-cache
// discounts); otherwise the coarse input/output counts are used at full price.
func estimateCost(detail UsageDetail, p ModelPricing) float64 {
	const perMillion = 1_000_000.0
	inRate := p.InputPricePer1M / perMillion
	outRate := p.OutputPricePer1M / perMillion

	var cost float64
	inputTotal := detail.UncachedInputTokens + detail.CacheReadTokens + detail.CacheWriteTokens
	if inputTotal > 0 {
		cost += float64(detail.UncachedInputTokens) * inRate
		cost += float64(detail.CacheReadTokens) * inRate * cacheReadPriceRatio
		cost += float64(detail.CacheWriteTokens) * inRate * cacheWritePriceRatio
	} else {
		cost += float64(detail.InputTokens) * inRate
	}

	outputTotal := detail.NonReasoningOutput + detail.ReasoningTokens
	if outputTotal > 0 {
		// Reasoning tokens are billed at the output rate.
		cost += float64(detail.NonReasoningOutput) * outRate
		cost += float64(detail.ReasoningTokens) * outRate
	} else {
		cost += float64(detail.OutputTokens) * outRate
	}

	// Unclassified tokens were previously not priced at all; charge them at the
	// (cheaper) input rate so they are not silently free.
	cost += float64(detail.UnclassifiedTokens) * inRate

	return cost
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
