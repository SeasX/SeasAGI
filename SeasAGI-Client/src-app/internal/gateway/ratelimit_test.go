package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

func TestRateLimiterRPM(t *testing.T) {
	lim := newRateLimiter()
	cfg := config.RateLimitConfig{DefaultRPM: 2}

	for i := 0; i < 2; i++ {
		if ok, reason := lim.admit(rateLimitGlobalKey, cfg); !ok {
			t.Fatalf("request %d should be admitted, got %s", i+1, reason)
		}
	}
	if ok, reason := lim.admit(rateLimitGlobalKey, cfg); ok || reason != "rpm" {
		t.Fatalf("third request should be rejected with rpm, got ok=%v reason=%s", ok, reason)
	}
}

func TestRateLimiterMinInterval(t *testing.T) {
	lim := newRateLimiter()
	cfg := config.RateLimitConfig{MinIntervalMs: 60_000}

	if ok, _ := lim.admit(rateLimitGlobalKey, cfg); !ok {
		t.Fatal("first request should be admitted")
	}
	if ok, reason := lim.admit(rateLimitGlobalKey, cfg); ok || reason != "min_interval" {
		t.Fatalf("immediate second request should hit min_interval, got ok=%v reason=%s", ok, reason)
	}
}

func TestRateLimiterMaxWaitAllowsWaiting(t *testing.T) {
	lim := newRateLimiter()
	cfg := config.RateLimitConfig{MinIntervalMs: 30, MaxWaitMs: 500}

	if ok, _ := lim.admit(rateLimitGlobalKey, cfg); !ok {
		t.Fatal("first request should be admitted")
	}
	if ok, reason := lim.admit(rateLimitGlobalKey, cfg); !ok {
		t.Fatalf("second request should be admitted after waiting, got %s", reason)
	}
}

func TestRateLimiterTPMUsesRecordedTokens(t *testing.T) {
	lim := newRateLimiter()
	cfg := config.RateLimitConfig{DefaultTPM: 100}

	if ok, _ := lim.admit(rateLimitGlobalKey, cfg); !ok {
		t.Fatal("first request should be admitted before any tokens are recorded")
	}
	lim.addTokens(rateLimitGlobalKey, 100)
	if ok, reason := lim.admit(rateLimitGlobalKey, cfg); ok || reason != "tpm" {
		t.Fatalf("request should be rejected with tpm, got ok=%v reason=%s", ok, reason)
	}
}

func TestRateLimiterConcurrency(t *testing.T) {
	lim := newRateLimiter()

	if !lim.acquireConcurrent(1) {
		t.Fatal("first slot should be acquired")
	}
	if lim.acquireConcurrent(1) {
		t.Fatal("second slot should be rejected when max=1")
	}
	lim.releaseConcurrent()
	if !lim.acquireConcurrent(1) {
		t.Fatal("slot should be reusable after release")
	}
	if !lim.acquireConcurrent(0) {
		t.Fatal("max<=0 means unlimited")
	}
}

func TestRateLimiterKeysAreIsolated(t *testing.T) {
	lim := newRateLimiter()
	cfg := config.RateLimitConfig{DefaultRPM: 1}

	if ok, _ := lim.admit(rateLimitGlobalKey, cfg); !ok {
		t.Fatal("global key should be admitted")
	}
	if ok, reason := lim.admit(channelRateKey("ch1"), cfg); !ok {
		t.Fatalf("channel key has its own window, got %s", reason)
	}
}

func TestEffectiveRateLimitOverrides(t *testing.T) {
	base := config.RateLimitConfig{DefaultRPM: 60, DefaultTPM: 1000, MinIntervalMs: 100, MaxConcurrent: 4}

	got := effectiveRateLimit(base, nil)
	if got.DefaultRPM != base.DefaultRPM || got.DefaultTPM != base.DefaultTPM ||
		got.MinIntervalMs != base.MinIntervalMs || got.MaxConcurrent != base.MaxConcurrent {
		t.Fatalf("nil override should keep base, got %+v", got)
	}

	got = effectiveRateLimit(base, &config.ChannelRateLimit{RPM: 10})
	if got.DefaultRPM != 10 || got.DefaultTPM != 1000 || got.MinIntervalMs != 100 || got.MaxConcurrent != 4 {
		t.Fatalf("zero override fields should fall back to base, got %+v", got)
	}
}

func TestWriteRateLimitError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimitError(rec, "rpm")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func newRateLimitTestService(t *testing.T) (*Service, *config.Service, *usage.Service) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfgSvc := config.NewService(config.AppConfig{})
	usageSvc := usage.NewService()
	return &Service{configSvc: cfgSvc, usageSvc: usageSvc, rateLimiter: newRateLimiter()}, cfgSvc, usageSvc
}

func TestCheckRateLimitsDisabledIsPassThrough(t *testing.T) {
	s, _, _ := newRateLimitTestService(t)

	release, ok := s.checkRateLimits(httptest.NewRecorder())
	if !ok {
		t.Fatal("disabled rate limiting should not block")
	}
	release()
}

func TestCheckRateLimitsEnforcesRPM(t *testing.T) {
	s, cfgSvc, _ := newRateLimitTestService(t)
	if err := cfgSvc.SetRateLimitConfig(config.RateLimitConfig{Enabled: true, DefaultRPM: 1}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	release, ok := s.checkRateLimits(httptest.NewRecorder())
	if !ok {
		t.Fatal("first request should pass")
	}
	release()

	rec := httptest.NewRecorder()
	if _, ok := s.checkRateLimits(rec); ok {
		t.Fatal("second request should be rate limited")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestCheckRateLimitsMonthlyCostLimit(t *testing.T) {
	s, cfgSvc, usageSvc := newRateLimitTestService(t)
	if err := cfgSvc.SetRateLimitConfig(config.RateLimitConfig{MonthlyCostLimitUSD: 1}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	usageSvc.RecordUsageV2(usage.UsageDetail{Model: "gpt-4o", InputTokens: 1_000_000})

	rec := httptest.NewRecorder()
	if _, ok := s.checkRateLimits(rec); ok {
		t.Fatal("cost limit should block requests")
	}
	if rec.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", rec.Code)
	}
}

func TestCheckChannelRateLimitWithoutOverride(t *testing.T) {
	s, cfgSvc, _ := newRateLimitTestService(t)
	if err := cfgSvc.SetRateLimitConfig(config.RateLimitConfig{Enabled: true, DefaultRPM: 60}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if !s.checkChannelRateLimit(httptest.NewRecorder(), "ch-without-override") {
		t.Fatal("channel without override should pass")
	}
}

func TestCheckChannelRateLimitEnforcesOverride(t *testing.T) {
	s, cfgSvc, _ := newRateLimitTestService(t)
	if err := cfgSvc.SetRateLimitConfig(config.RateLimitConfig{
		Enabled:          true,
		DefaultRPM:       60,
		ChannelOverrides: map[string]*config.ChannelRateLimit{"ch1": {RPM: 1}},
	}); err != nil {
		t.Fatalf("set config: %v", err)
	}

	if !s.checkChannelRateLimit(httptest.NewRecorder(), "ch1") {
		t.Fatal("first request on ch1 should pass")
	}
	rec := httptest.NewRecorder()
	if s.checkChannelRateLimit(rec, "ch1") {
		t.Fatal("second request on ch1 should be limited by override")
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

func TestRecordUsageAndTokensFeedsLimiter(t *testing.T) {
	s, _, _ := newRateLimitTestService(t)

	rec := httptest.NewRecorder()
	c := newUsageCapture(rec, false)
	c.WriteHeader(http.StatusOK)
	c.Write([]byte(`{"usage":{"prompt_tokens":30,"completion_tokens":10}}`))
	s.recordUsageAndTokens(c, usageMeta{ChannelID: "ch1", Model: "gpt-4o", Always: true})

	s.rateLimiter.mu.Lock()
	global := len(s.rateLimiter.windows[rateLimitGlobalKey].tokens)
	channel := len(s.rateLimiter.windows[channelRateKey("ch1")].tokens)
	s.rateLimiter.mu.Unlock()

	if global != 1 || channel != 1 {
		t.Fatalf("expected token samples recorded for global and channel, got global=%d channel=%d", global, channel)
	}
}
