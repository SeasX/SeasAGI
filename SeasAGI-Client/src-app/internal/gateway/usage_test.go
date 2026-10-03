package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

// newUsageTestService 在隔离的 HOME 下创建用量服务，避免污染真实用户数据。
func newUsageTestService(t *testing.T) *usage.Service {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return usage.NewService()
}

func TestUsageCaptureNonStreamRecordsUsage(t *testing.T) {
	svc := newUsageTestService(t)
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, false)
	c.WriteHeader(http.StatusOK)
	body := `{"id":"x","usage":{"prompt_tokens":100,"completion_tokens":20,` +
		`"prompt_tokens_details":{"cached_tokens":40},"completion_tokens_details":{"reasoning_tokens":5}}}`
	if _, err := c.Write([]byte(body)); err != nil {
		t.Fatalf("write: %v", err)
	}
	c.recordUsage(svc, usageMeta{
		ChannelID:   "ch1",
		ChannelName: "Channel 1",
		Model:       "gpt-4o",
		Headers:     http.Header{"X-RateLimit-Remaining": []string{"42"}},
	})

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.ChannelID != "ch1" || r.ChannelName != "Channel 1" || r.Model != "gpt-4o" {
		t.Errorf("unexpected meta: %+v", r)
	}
	if r.InputTokens != 100 {
		t.Errorf("expected input 100, got %d", r.InputTokens)
	}
	if r.OutputTokens != 20 {
		t.Errorf("expected output 20, got %d", r.OutputTokens)
	}
	if r.CacheReadTokens != 40 {
		t.Errorf("expected cache read 40, got %d", r.CacheReadTokens)
	}
	if r.UncachedInputTokens != 60 {
		t.Errorf("expected uncached input 60, got %d", r.UncachedInputTokens)
	}
	if r.ReasoningTokens != 5 || r.NonReasoningOutput != 15 {
		t.Errorf("expected reasoning 5 / non-reasoning 15, got %d / %d", r.ReasoningTokens, r.NonReasoningOutput)
	}
	if r.TokenQuality != "complete" {
		t.Errorf("expected token quality complete, got %s", r.TokenQuality)
	}
	if r.RateLimitRemaining != 42 {
		t.Errorf("expected rate limit remaining 42, got %d", r.RateLimitRemaining)
	}
	if r.TTFTMs < 0 || r.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got ttft=%d latency=%d", r.TTFTMs, r.LatencyMs)
	}
}

func TestUsageCaptureStreamParsesOpenAIUsage(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, true)
	c.WriteHeader(http.StatusOK)
	c.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	// 分片写入，验证跨 write 的行缓冲
	c.Write([]byte("data: {\"usage\":{\"prompt_tokens\":7,"))
	c.Write([]byte("\"completion_tokens\":3}}\n\n"))
	c.Write([]byte("data: [DONE]\n\n"))

	status, ttftMs, usageObj, body := c.usageSnapshot()
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if ttftMs < 0 {
		t.Fatalf("expected ttft recorded, got %d", ttftMs)
	}
	if body != nil {
		t.Fatalf("stream mode should not capture body, got %d bytes", len(body))
	}
	if usageObj == nil || tokenValue(usageObj, "prompt_tokens") != 7 || tokenValue(usageObj, "completion_tokens") != 3 {
		t.Fatalf("unexpected usage object: %+v", usageObj)
	}
}

func TestUsageCaptureStreamParsesAnthropicUsage(t *testing.T) {
	svc := newUsageTestService(t)
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, true)
	c.WriteHeader(http.StatusOK)
	c.Write([]byte("event: message_start\n"))
	c.Write([]byte(`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":3,"output_tokens":2}}}` + "\n\n"))

	c.recordUsage(svc, usageMeta{Model: "claude-sonnet-4-20250514"})

	records := svc.GetAllRecords()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	// Anthropic 语义：总输入 = input_tokens + cache_read
	if r.InputTokens != 13 || r.UncachedInputTokens != 10 || r.CacheReadTokens != 3 {
		t.Errorf("unexpected anthropic tokens: %+v", r)
	}
	if r.TokenQuality != "complete" {
		t.Errorf("expected token quality complete, got %s", r.TokenQuality)
	}
}

func TestUsageCaptureSkipsErrorStatus(t *testing.T) {
	svc := newUsageTestService(t)
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, false)
	c.WriteHeader(http.StatusInternalServerError)
	c.Write([]byte(`{"usage":{"prompt_tokens":5}}`))
	c.recordUsage(svc, usageMeta{Always: true, Model: "gpt-4o"})

	if got := len(svc.GetAllRecords()); got != 0 {
		t.Fatalf("expected no records for error response, got %d", got)
	}
}

func TestUsageCaptureAlwaysRecordsRequestCount(t *testing.T) {
	svc := newUsageTestService(t)

	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, false)
	c.WriteHeader(http.StatusOK)
	c.Write([]byte(`{"id":"x"}`))
	c.recordUsage(svc, usageMeta{Model: "gpt-4o"})
	if got := len(svc.GetAllRecords()); got != 0 {
		t.Fatalf("expected no record without usage field, got %d", got)
	}

	rec2 := httptest.NewRecorder()
	c2 := newUsageCapture(rec2, false)
	c2.WriteHeader(http.StatusOK)
	c2.Write([]byte(`{"id":"x"}`))
	c2.recordUsage(svc, usageMeta{Always: true, Model: "gpt-4o"})
	if got := len(svc.GetAllRecords()); got != 1 {
		t.Fatalf("expected request count record, got %d", got)
	}
}

func TestUsageCaptureNilServiceIsNoop(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, false)
	c.WriteHeader(http.StatusOK)
	c.Write([]byte(`{"usage":{"prompt_tokens":1}}`))
	c.recordUsage(nil, usageMeta{Always: true})
}

func TestUsageCaptureFlushDelegates(t *testing.T) {
	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, true)
	c.Flush()
	if !rec.Flushed {
		t.Fatal("expected Flush to delegate to underlying http.Flusher")
	}
}
