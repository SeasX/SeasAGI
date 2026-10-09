package usage

import (
	"testing"
)

func TestClassifyTokenQuality(t *testing.T) {
	cases := []struct {
		name string
		d    UsageDetail
		want string
	}{
		{
			name: "no v2 breakdown is unclassified",
			d:    UsageDetail{InputTokens: 50, OutputTokens: 10},
			want: "unclassified",
		},
		{
			name: "all zero is unclassified",
			d:    UsageDetail{},
			want: "unclassified",
		},
		{
			name: "consistent breakdown is complete",
			d: UsageDetail{
				InputTokens: 100, OutputTokens: 20,
				UncachedInputTokens: 60, CacheReadTokens: 40,
				ReasoningTokens: 5, NonReasoningOutput: 15,
			},
			want: "complete",
		},
		{
			name: "input discrepancy over 10 percent is inconsistent",
			d: UsageDetail{
				InputTokens: 100, UncachedInputTokens: 10,
			},
			want: "inconsistent",
		},
		{
			name: "output discrepancy over 10 percent is inconsistent",
			d: UsageDetail{
				OutputTokens: 100, NonReasoningOutput: 10,
			},
			want: "inconsistent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyTokenQuality(tc.d); got != tc.want {
				t.Fatalf("classifyTokenQuality = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseTokenBreakdownOpenAI(t *testing.T) {
	usage := map[string]any{
		"prompt_tokens":     float64(100),
		"completion_tokens": float64(20),
		"prompt_tokens_details": map[string]any{
			"cached_tokens": float64(40),
		},
		"completion_tokens_details": map[string]any{
			"reasoning_tokens": float64(5),
		},
	}
	cacheRead, cacheWrite, reasoning, unclassified := ParseTokenBreakdown(usage)
	if cacheRead != 40 || cacheWrite != 0 || reasoning != 5 || unclassified != 0 {
		t.Fatalf("got (read=%d write=%d reasoning=%d unclassified=%d), want (40,0,5,0)",
			cacheRead, cacheWrite, reasoning, unclassified)
	}
}

func TestParseTokenBreakdownAnthropic(t *testing.T) {
	usage := map[string]any{
		"cache_creation_input_tokens": float64(12),
		"cache_read_input_tokens":     float64(30),
	}
	cacheRead, cacheWrite, reasoning, unclassified := ParseTokenBreakdown(usage)
	if cacheRead != 30 || cacheWrite != 12 || reasoning != 0 || unclassified != 0 {
		t.Fatalf("got (read=%d write=%d reasoning=%d unclassified=%d), want (30,12,0,0)",
			cacheRead, cacheWrite, reasoning, unclassified)
	}
}

func TestParseTokenBreakdownNil(t *testing.T) {
	r, w, reason, u := ParseTokenBreakdown(nil)
	if r != 0 || w != 0 || reason != 0 || u != 0 {
		t.Fatalf("nil usage should yield all zeros, got (%d,%d,%d,%d)", r, w, reason, u)
	}
}

func TestExtractRateLimitHeaders(t *testing.T) {
	headers := map[string][]string{
		"X-RateLimit-Remaining": {"42"},
		"x-ratelimit-limit":     {"100"},
		"X-RateLimit-Reset":     {"1700000000"},
	}
	remaining, limit, reset := ExtractRateLimitHeaders(headers)
	if remaining != 42 || limit != 100 || reset != 1700000000 {
		t.Fatalf("got (remaining=%d limit=%d reset=%d), want (42,100,1700000000)", remaining, limit, reset)
	}
}

func TestExtractRateLimitHeadersVariants(t *testing.T) {
	headers := map[string][]string{
		"ratelimit-remaining": {"7"},
		"ratelimit-limit":     {"9"},
		"ratelimit-reset":     {"123"},
	}
	remaining, limit, reset := ExtractRateLimitHeaders(headers)
	if remaining != 7 || limit != 9 || reset != 123 {
		t.Fatalf("got (remaining=%d limit=%d reset=%d), want (7,9,123)", remaining, limit, reset)
	}
}

func TestExtractRateLimitHeadersEdgeCases(t *testing.T) {
	// 空值切片应跳过
	if r, l, reset := ExtractRateLimitHeaders(map[string][]string{"x-ratelimit-remaining": {}}); r != 0 || l != 0 || reset != 0 {
		t.Fatalf("empty value should be skipped, got (%d,%d,%d)", r, l, reset)
	}
	// 非数字串按遇到的首个非数字位截断 → 0
	if r, _, _ := ExtractRateLimitHeaders(map[string][]string{"x-ratelimit-remaining": {"abc"}}); r != 0 {
		t.Fatalf("non-numeric should parse to 0, got %d", r)
	}
	// 前缀数字解析
	if r, _, _ := ExtractRateLimitHeaders(map[string][]string{"x-ratelimit-remaining": {"12abc"}}); r != 12 {
		t.Fatalf("prefix digits should parse, got %d", r)
	}
}
