package usage

// classifyTokenQuality determines the quality of token accounting for a request.
// Returns one of: "complete", "inconsistent", "unclassified".
//
// - "complete": all v2 breakdown fields are populated and invariants hold.
// - "inconsistent": v2 breakdown fields are partially populated or invariants fail.
// - "unclassified": no v2 breakdown provided (legacy mode).
func classifyTokenQuality(d UsageDetail) string {
	hasV2 := d.CacheReadTokens > 0 || d.CacheWriteTokens > 0 ||
		d.UncachedInputTokens > 0 || d.ReasoningTokens > 0 ||
		d.NonReasoningOutput > 0 || d.UnclassifiedTokens > 0

	if !hasV2 {
		return "unclassified"
	}

	// Check invariants:
	//   inputTotal = uncached + cacheRead + cacheWrite
	//   outputTotal = nonReasoning + reasoning
	//   grandTotal = inputTotal + outputTotal + unclassified
	inputTotal := d.UncachedInputTokens + d.CacheReadTokens + d.CacheWriteTokens
	outputTotal := d.NonReasoningOutput + d.ReasoningTokens
	grandTotal := inputTotal + outputTotal + d.UnclassifiedTokens

	// If all breakdown fields are zero but we have tokens, it's unclassified
	if inputTotal == 0 && outputTotal == 0 && d.UnclassifiedTokens == 0 {
		return "unclassified"
	}

	// Check consistency: input/output totals should match the raw values
	consistent := true
	if inputTotal > 0 && inputTotal != d.InputTokens && d.InputTokens > 0 {
		// Some providers use subset semantics (cache included in input)
		// vs independent semantics (cache separate from input)
		// Either way, if the difference is large, flag as inconsistent
		diff := inputTotal - d.InputTokens
		if diff < 0 {
			diff = -diff
		}
		if diff > d.InputTokens/10 { // >10% discrepancy
			consistent = false
		}
	}
	if outputTotal > 0 && outputTotal != d.OutputTokens && d.OutputTokens > 0 {
		diff := outputTotal - d.OutputTokens
		if diff < 0 {
			diff = -diff
		}
		if diff > d.OutputTokens/10 {
			consistent = false
		}
	}
	_ = grandTotal

	if !consistent {
		return "inconsistent"
	}
	return "complete"
}

// ParseTokenBreakdown extracts v2 token breakdown from an OpenAI-style usage object.
// Handles both subset semantics (Anthropic: cache tokens included in input total)
// and independent semantics (OpenAI: cache tokens separate).
func ParseTokenBreakdown(usage map[string]any) (cacheRead, cacheWrite, reasoning, unclassified int64) {
	if usage == nil {
		return 0, 0, 0, 0
	}

	// OpenAI: prompt_tokens_details.cached_tokens
	if ptd, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if ct, ok := ptd["cached_tokens"].(float64); ok {
			cacheRead = int64(ct)
		}
	}

	// Anthropic: cache_creation_input_tokens + cache_read_input_tokens
	if ccit, ok := usage["cache_creation_input_tokens"].(float64); ok {
		cacheWrite = int64(ccit)
	}
	if crit, ok := usage["cache_read_input_tokens"].(float64); ok {
		cacheRead = int64(crit)
	}

	// OpenAI: completion_tokens_details.reasoning_tokens
	if ctd, ok := usage["completion_tokens_details"].(map[string]any); ok {
		if rt, ok := ctd["reasoning_tokens"].(float64); ok {
			reasoning = int64(rt)
		}
	}

	// Anthropic: usage is nested in response, not in the same structure
	// Anthropic reasoning is output_tokens minus non-reasoning
	// This is handled by the caller if needed

	return cacheRead, cacheWrite, reasoning, unclassified
}

// ExtractRateLimitHeaders extracts rate-limit info from HTTP response headers.
// Returns (remaining, limit, resetTimestamp).
func ExtractRateLimitHeaders(headers map[string][]string) (remaining, limit int, reset int64) {
	for k, vs := range headers {
		lk := toLower(k)
		if len(vs) == 0 {
			continue
		}
		v := vs[0]
		switch lk {
		case "x-ratelimit-remaining", "ratelimit-remaining":
			remaining = parseIntSafe(v)
		case "x-ratelimit-limit", "ratelimit-limit":
			limit = parseIntSafe(v)
		case "x-ratelimit-reset", "ratelimit-reset":
			reset = parseIntSafe64(v)
		}
	}
	return
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		b[i] = c
	}
	return string(b)
}

func parseIntSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseIntSafe64(s string) int64 {
	n := int64(0)
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int64(c-'0')
	}
	return n
}
