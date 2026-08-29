package usage

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc := &Service{
		records: make([]UsageRecord, 0),
		pricing: make([]ModelPricing, 0),
		path:    filepath.Join(t.TempDir(), "usage_records.json"),
	}
	svc.initDefaultPricing()
	return svc
}

func TestNewService(t *testing.T) {
	svc := newTestService(t)
	pricing := svc.ListPricing()
	if len(pricing) == 0 {
		t.Fatal("expected default pricing to be initialized")
	}
	// Verify a few known models
	known := map[string]bool{
		"gpt-4o": false, "gpt-4o-mini": false, "claude-sonnet-4-20250514": false,
	}
	for _, p := range pricing {
		if _, ok := known[p.Model]; ok {
			known[p.Model] = true
		}
	}
	for model, found := range known {
		if !found {
			t.Errorf("expected model %q in default pricing", model)
		}
	}
}

func TestRecordUsage_AddsRecordAndCalculatesCost(t *testing.T) {
	svc := newTestService(t)
	svc.RecordUsage("ch1", "Channel 1", "gpt-4o", 1_000_000, 500_000)

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.ChannelID != "ch1" {
		t.Errorf("expected channel_id ch1, got %s", r.ChannelID)
	}
	if r.ChannelName != "Channel 1" {
		t.Errorf("expected channel_name Channel 1, got %s", r.ChannelName)
	}
	if r.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", r.Model)
	}
	if r.RequestCount != 1 {
		t.Errorf("expected request_count 1, got %d", r.RequestCount)
	}
	if r.InputTokens != 1_000_000 {
		t.Errorf("expected input_tokens 1000000, got %d", r.InputTokens)
	}
	if r.OutputTokens != 500_000 {
		t.Errorf("expected output_tokens 500000, got %d", r.OutputTokens)
	}
	// gpt-4o: input=2.5/1M, output=10/1M => (1_000_000/1_000_000)*2.5 + (500_000/1_000_000)*10 = 2.5 + 5.0 = 7.5
	expectedCost := 7.5
	if r.CostUSD != expectedCost {
		t.Errorf("expected cost %.4f, got %.4f", expectedCost, r.CostUSD)
	}
}

func TestRecordUsage_UnknownModelCostZero(t *testing.T) {
	svc := newTestService(t)
	svc.RecordUsage("ch1", "Channel 1", "unknown-model", 1_000_000, 500_000)

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].CostUSD != 0 {
		t.Errorf("expected cost 0 for unknown model, got %.4f", records[0].CostUSD)
	}
}

func TestRecordUsage_TruncatesAtMaxRecords(t *testing.T) {
	svc := newTestService(t)
	// Pre-populate 10000 records directly to avoid O(n^2) persist calls
	svc.mu.Lock()
	for i := 0; i < 10000; i++ {
		svc.records = append(svc.records, UsageRecord{
			Timestamp:    time.Now().Format(time.RFC3339),
			ChannelID:    "ch1",
			ChannelName:  "Channel 1",
			Model:        "gpt-4o-mini",
			RequestCount: 1,
			InputTokens:  100,
			OutputTokens: 50,
		})
	}
	svc.mu.Unlock()

	// Adding one more via RecordUsage triggers truncation to 10000
	svc.RecordUsage("ch1", "Channel 1", "gpt-4o-mini", 100, 50)

	records := svc.GetAllRecords()
	if len(records) != 10000 {
		t.Fatalf("expected 10000 records after truncation, got %d", len(records))
	}
}

func TestGetUsageSummary_NoRecords(t *testing.T) {
	svc := newTestService(t)
	summary := svc.GetUsageSummary()
	if summary.MonthRequests != 0 {
		t.Errorf("expected MonthRequests 0, got %d", summary.MonthRequests)
	}
	if summary.MonthInputTokens != 0 {
		t.Errorf("expected MonthInputTokens 0, got %d", summary.MonthInputTokens)
	}
	if summary.MonthOutputTokens != 0 {
		t.Errorf("expected MonthOutputTokens 0, got %d", summary.MonthOutputTokens)
	}
	if summary.MonthCostUSD != 0 {
		t.Errorf("expected MonthCostUSD 0, got %.4f", summary.MonthCostUSD)
	}
}

func TestGetUsageSummary_WithRecordsInCurrentMonth(t *testing.T) {
	svc := newTestService(t)
	// Manually add a record with a current-month timestamp
	now := time.Now()
	svc.mu.Lock()
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.Format(time.RFC3339),
		ChannelID:    "ch1",
		ChannelName:  "Channel 1",
		Model:        "gpt-4o",
		RequestCount: 5,
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      7.5,
	})
	svc.mu.Unlock()

	summary := svc.GetUsageSummary()
	if summary.MonthRequests != 5 {
		t.Errorf("expected MonthRequests 5, got %d", summary.MonthRequests)
	}
	if summary.MonthInputTokens != 1000 {
		t.Errorf("expected MonthInputTokens 1000, got %d", summary.MonthInputTokens)
	}
	if summary.MonthOutputTokens != 500 {
		t.Errorf("expected MonthOutputTokens 500, got %d", summary.MonthOutputTokens)
	}
	if summary.MonthCostUSD != 7.5 {
		t.Errorf("expected MonthCostUSD 7.5, got %.4f", summary.MonthCostUSD)
	}
}

func TestGetDailyUsage_WithRecords(t *testing.T) {
	svc := newTestService(t)
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Add a record for today
	svc.mu.Lock()
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.Format(time.RFC3339),
		ChannelID:    "ch1",
		ChannelName:  "Ch1",
		Model:        "gpt-4o",
		RequestCount: 3,
		InputTokens:  3000,
		OutputTokens: 1500,
		CostUSD:      10.0,
	})
	// Add a record for yesterday
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.AddDate(0, 0, -1).Format(time.RFC3339),
		ChannelID:    "ch2",
		ChannelName:  "Ch2",
		Model:        "gpt-4o-mini",
		RequestCount: 2,
		InputTokens:  2000,
		OutputTokens: 1000,
		CostUSD:      5.0,
	})
	svc.mu.Unlock()

	daily := svc.GetDailyUsage(3)
	if len(daily) != 3 {
		t.Fatalf("expected 3 daily entries, got %d", len(daily))
	}

	// Find today and yesterday entries
	var todayEntry, yesterdayEntry *DailyUsage
	for i := range daily {
		if daily[i].Date == today {
			todayEntry = &daily[i]
		}
		if daily[i].Date == yesterday {
			yesterdayEntry = &daily[i]
		}
	}

	if todayEntry == nil {
		t.Fatal("expected today entry in daily usage")
	}
	if todayEntry.TotalRequests != 3 {
		t.Errorf("expected today total_requests 3, got %d", todayEntry.TotalRequests)
	}
	if todayEntry.TotalCostUSD != 10.0 {
		t.Errorf("expected today total_cost_usd 10.0, got %.4f", todayEntry.TotalCostUSD)
	}
	if ch, ok := todayEntry.ByChannel["ch1"]; !ok {
		t.Error("expected ch1 in today by_channel")
	} else if ch.Requests != 3 {
		t.Errorf("expected ch1 requests 3, got %d", ch.Requests)
	}

	if yesterdayEntry == nil {
		t.Fatal("expected yesterday entry in daily usage")
	}
	if yesterdayEntry.TotalRequests != 2 {
		t.Errorf("expected yesterday total_requests 2, got %d", yesterdayEntry.TotalRequests)
	}
	if ch, ok := yesterdayEntry.ByChannel["ch2"]; !ok {
		t.Error("expected ch2 in yesterday by_channel")
	} else if ch.Requests != 2 {
		t.Errorf("expected ch2 requests 2, got %d", ch.Requests)
	}
}

func TestGetTotalUsage_ReturnsCorrectTotals(t *testing.T) {
	svc := newTestService(t)
	svc.RecordUsage("ch1", "Ch1", "gpt-4o", 1000, 500)
	svc.RecordUsage("ch2", "Ch2", "gpt-4o-mini", 2000, 1000)

	total := svc.GetTotalUsage()
	if total["total_requests"].(int) != 2 {
		t.Errorf("expected total_requests 2, got %d", total["total_requests"].(int))
	}
	if total["total_input_tokens"].(int64) != 3000 {
		t.Errorf("expected total_input_tokens 3000, got %d", total["total_input_tokens"].(int64))
	}
	if total["total_output_tokens"].(int64) != 1500 {
		t.Errorf("expected total_output_tokens 1500, got %d", total["total_output_tokens"].(int64))
	}
	if total["record_count"].(int) != 2 {
		t.Errorf("expected record_count 2, got %d", total["record_count"].(int))
	}
}

func TestPurgeBefore_RemovesOldRecords(t *testing.T) {
	svc := newTestService(t)
	now := time.Now()

	// Add a record from 10 days ago
	svc.mu.Lock()
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.AddDate(0, 0, -10).Format(time.RFC3339),
		ChannelID:    "old",
		ChannelName:  "Old",
		Model:        "gpt-4o",
		RequestCount: 1,
	})
	// Add a recent record
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.Format(time.RFC3339),
		ChannelID:    "new",
		ChannelName:  "New",
		Model:        "gpt-4o",
		RequestCount: 1,
	})
	svc.mu.Unlock()

	removed := svc.PurgeBefore(7) // purge records older than 7 days
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record after purge, got %d", len(records))
	}
	if records[0].ChannelID != "new" {
		t.Errorf("expected remaining record channel_id 'new', got %s", records[0].ChannelID)
	}
}

func TestPurgeBefore_NoOldRecords(t *testing.T) {
	svc := newTestService(t)
	now := time.Now()

	svc.mu.Lock()
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    now.Format(time.RFC3339),
		ChannelID:    "ch1",
		ChannelName:  "Ch1",
		Model:        "gpt-4o",
		RequestCount: 1,
	})
	svc.mu.Unlock()

	removed := svc.PurgeBefore(30) // purge records older than 30 days
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Errorf("expected 1 record still present, got %d", len(records))
	}
}

func TestGetModelStats_ReturnsCorrectStats(t *testing.T) {
	svc := newTestService(t)
	svc.RecordUsage("ch1", "Ch1", "gpt-4o", 100, 50)
	svc.RecordUsage("ch2", "Ch2", "gpt-4o-mini", 200, 100)
	svc.RecordUsage("ch3", "Ch3", "gpt-4o", 300, 150)

	stats := svc.GetModelStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 model stats entries, got %d", len(stats))
	}

	// Should be sorted by total_requests descending: gpt-4o (2) > gpt-4o-mini (1)
	if stats[0].Model != "gpt-4o" {
		t.Errorf("expected first entry gpt-4o, got %s", stats[0].Model)
	}
	if stats[0].TotalRequests != 2 {
		t.Errorf("expected gpt-4o total_requests 2, got %d", stats[0].TotalRequests)
	}
	if stats[1].Model != "gpt-4o-mini" {
		t.Errorf("expected second entry gpt-4o-mini, got %s", stats[1].Model)
	}
	if stats[1].TotalRequests != 1 {
		t.Errorf("expected gpt-4o-mini total_requests 1, got %d", stats[1].TotalRequests)
	}
}

func TestGetModelStats_Empty(t *testing.T) {
	svc := newTestService(t)
	stats := svc.GetModelStats()
	if stats != nil {
		t.Errorf("expected nil stats for empty records, got %v", stats)
	}
}

func TestListPricing_ReturnsPricing(t *testing.T) {
	svc := newTestService(t)
	pricing := svc.ListPricing()
	if len(pricing) == 0 {
		t.Fatal("expected non-empty pricing list")
	}
	// Verify the returned slice is a copy (modifying it shouldn't affect the original)
	origLen := len(pricing)
	pricing = append(pricing, ModelPricing{Model: "fake"})
	after := svc.ListPricing()
	if len(after) != origLen {
		t.Errorf("expected original pricing length %d, got %d", origLen, len(after))
	}
}

func TestSavePricing_UpdatesPricing(t *testing.T) {
	svc := newTestService(t)
	newPricing := []ModelPricing{
		{Model: "custom-model", InputPricePer1M: 1.0, OutputPricePer1M: 2.0},
	}
	err := svc.SavePricing(newPricing)
	if err != nil {
		t.Fatalf("SavePricing returned error: %v", err)
	}
	pricing := svc.ListPricing()
	if len(pricing) != 1 {
		t.Fatalf("expected 1 pricing entry, got %d", len(pricing))
	}
	if pricing[0].Model != "custom-model" {
		t.Errorf("expected model custom-model, got %s", pricing[0].Model)
	}
	if pricing[0].InputPricePer1M != 1.0 {
		t.Errorf("expected input price 1.0, got %.4f", pricing[0].InputPricePer1M)
	}
	if pricing[0].OutputPricePer1M != 2.0 {
		t.Errorf("expected output price 2.0, got %.4f", pricing[0].OutputPricePer1M)
	}
}

func TestConcurrentAccessSafety(t *testing.T) {
	svc := newTestService(t)
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			chID := "ch"
			if i%2 == 0 {
				chID = "ch-even"
			}
			svc.RecordUsage(chID, "Channel", "gpt-4o", 100, 50)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.GetUsageSummary()
			_ = svc.GetTotalUsage()
			_ = svc.GetDailyUsage(7)
			_ = svc.ListPricing()
		}()
	}

	wg.Wait()

	records := svc.GetAllRecords()
	if len(records) != 50 {
		t.Errorf("expected 50 records after concurrent writes, got %d", len(records))
	}
}

func TestRecordUsage_PersistsToFile(t *testing.T) {
	svc := newTestService(t)
	svc.RecordUsage("ch1", "Channel 1", "gpt-4o", 1000, 500)

	// Create a new service with the same path to verify persistence
	svc2 := &Service{
		records: make([]UsageRecord, 0),
		pricing: make([]ModelPricing, 0),
		path:    svc.path,
	}
	svc2.load()
	svc2.initDefaultPricing()

	records := svc2.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record loaded from file, got %d", len(records))
	}
	if records[0].ChannelID != "ch1" {
		t.Errorf("expected channel_id ch1, got %s", records[0].ChannelID)
	}
}

func TestGetDailyUsage_Empty(t *testing.T) {
	svc := newTestService(t)
	daily := svc.GetDailyUsage(5)
	if len(daily) != 5 {
		t.Fatalf("expected 5 daily entries, got %d", len(daily))
	}
	for _, d := range daily {
		if d.TotalRequests != 0 {
			t.Errorf("expected 0 requests for date %s, got %d", d.Date, d.TotalRequests)
		}
		if d.TotalCostUSD != 0 {
			t.Errorf("expected 0 cost for date %s, got %.4f", d.Date, d.TotalCostUSD)
		}
	}
}

func TestGetUsageSummary_OnlyFutureRecord(t *testing.T) {
	svc := newTestService(t)
	// Add a record from last month — should not be included in current month summary
	lastMonth := time.Now().AddDate(0, -1, 0)
	svc.mu.Lock()
	svc.records = append(svc.records, UsageRecord{
		Timestamp:    lastMonth.Format(time.RFC3339),
		ChannelID:    "ch1",
		ChannelName:  "Ch1",
		Model:        "gpt-4o",
		RequestCount: 10,
		InputTokens:  10000,
		OutputTokens: 5000,
		CostUSD:      25.0,
	})
	svc.mu.Unlock()

	summary := svc.GetUsageSummary()
	if summary.MonthRequests != 0 {
		t.Errorf("expected 0 for last month record, got %d", summary.MonthRequests)
	}
}
