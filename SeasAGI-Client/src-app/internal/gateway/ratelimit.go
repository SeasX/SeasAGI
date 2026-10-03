package gateway

import (
	"net/http"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
)

// rateLimitGlobalKey 全局限流计数键；per-channel 计数键加 channelRateKey 前缀。
const rateLimitGlobalKey = "global"

func channelRateKey(channelID string) string { return "channel:" + channelID }

type tokenSample struct {
	at     time.Time
	tokens int64
}

// rateLimitWindow 单个 key 的滑动窗口状态（近 1 分钟）。
type rateLimitWindow struct {
	reqs   []time.Time
	tokens []tokenSample
	last   time.Time
}

// rateLimiter 请求级限流执行器：RPM / TPM / 最小间隔 / 全局并发。
// 月度成本硬上限依赖 usage.Service 汇总，不在此结构内。
type rateLimiter struct {
	mu       sync.Mutex
	windows  map[string]*rateLimitWindow
	inflight int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]*rateLimitWindow)}
}

// acquireConcurrent 占用一个全局并发槽位；max<=0 表示不限。超出上限返回 false。
func (l *rateLimiter) acquireConcurrent(max int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if max > 0 && l.inflight >= max {
		return false
	}
	l.inflight++
	return true
}

func (l *rateLimiter) releaseConcurrent() {
	l.mu.Lock()
	if l.inflight > 0 {
		l.inflight--
	}
	l.mu.Unlock()
}

// admit 判定 key 是否放行；放行时记录本次请求（供 RPM 与最小间隔计数）。
// MaxWaitMs > 0 时超限后按 50ms 步进重试，直到放行或超时。
func (l *rateLimiter) admit(key string, lim config.RateLimitConfig) (bool, string) {
	deadline := time.Now().Add(time.Duration(lim.MaxWaitMs) * time.Millisecond)
	for {
		ok, reason := l.tryAdmit(key, lim)
		if ok {
			return true, ""
		}
		if lim.MaxWaitMs <= 0 || !time.Now().Before(deadline) {
			return false, reason
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (l *rateLimiter) tryAdmit(key string, lim config.RateLimitConfig) (bool, string) {
	now := time.Now()
	cutoff := now.Add(-time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()

	w := l.windows[key]
	if w == nil {
		w = &rateLimitWindow{}
		l.windows[key] = w
	}
	w.reqs = pruneTimes(w.reqs, cutoff)
	w.tokens = pruneSamples(w.tokens, cutoff)

	if lim.MinIntervalMs > 0 && !w.last.IsZero() &&
		now.Sub(w.last) < time.Duration(lim.MinIntervalMs)*time.Millisecond {
		return false, "min_interval"
	}
	if lim.DefaultRPM > 0 && len(w.reqs) >= lim.DefaultRPM {
		return false, "rpm"
	}
	if lim.DefaultTPM > 0 {
		var used int64
		for _, s := range w.tokens {
			used += s.tokens
		}
		if used >= int64(lim.DefaultTPM) {
			return false, "tpm"
		}
	}

	w.reqs = append(w.reqs, now)
	w.last = now
	return true, ""
}

// addTokens 回填 key 已消耗的 token 数（响应采集后调用，供下一轮 TPM 判定）。
func (l *rateLimiter) addTokens(key string, tokens int64) {
	if tokens <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.windows[key]
	if w == nil {
		w = &rateLimitWindow{}
		l.windows[key] = w
	}
	w.tokens = append(w.tokens, tokenSample{at: time.Now(), tokens: tokens})
}

func pruneTimes(ts []time.Time, cutoff time.Time) []time.Time {
	idx := 0
	for idx < len(ts) && ts[idx].Before(cutoff) {
		idx++
	}
	return ts[idx:]
}

func pruneSamples(ss []tokenSample, cutoff time.Time) []tokenSample {
	idx := 0
	for idx < len(ss) && ss[idx].at.Before(cutoff) {
		idx++
	}
	return ss[idx:]
}

// effectiveRateLimit 将 per-channel 覆盖叠加到全局配置（覆盖值为 0 时沿用全局）。
func effectiveRateLimit(base config.RateLimitConfig, ov *config.ChannelRateLimit) config.RateLimitConfig {
	if ov == nil {
		return base
	}
	if ov.RPM > 0 {
		base.DefaultRPM = ov.RPM
	}
	if ov.TPM > 0 {
		base.DefaultTPM = ov.TPM
	}
	if ov.MinIntervalMs > 0 {
		base.MinIntervalMs = ov.MinIntervalMs
	}
	if ov.MaxConcurrent > 0 {
		base.MaxConcurrent = ov.MaxConcurrent
	}
	return base
}

func writeRateLimitError(w http.ResponseWriter, reason string) {
	w.Header().Set("Retry-After", "1")
	writeJSONError(w, http.StatusTooManyRequests, "Rate limit exceeded: "+reason)
}

// checkRateLimits 全局预检：月度成本硬上限、RPM / TPM / 最小间隔、全局并发。
// 返回的 release 必须在请求结束时调用；ok=false 时已写入错误响应。
func (s *Service) checkRateLimits(w http.ResponseWriter) (func(), bool) {
	cfg := s.configSvc.GetRateLimitConfig()

	if cfg.MonthlyCostLimitUSD > 0 {
		if svc := s.currentUsageService(); svc != nil {
			if svc.GetUsageSummary().MonthCostUSD >= cfg.MonthlyCostLimitUSD {
				writeJSONError(w, http.StatusPaymentRequired,
					"Monthly cost limit exceeded; requests are blocked until the limit is raised or the month resets")
				return nil, false
			}
		}
	}

	if !cfg.Enabled {
		return func() {}, true
	}

	if ok, reason := s.rateLimiter.admit(rateLimitGlobalKey, cfg); !ok {
		writeRateLimitError(w, reason)
		return nil, false
	}
	if !s.rateLimiter.acquireConcurrent(cfg.MaxConcurrent) {
		writeRateLimitError(w, "max_concurrent")
		return nil, false
	}
	return s.rateLimiter.releaseConcurrent, true
}

// checkChannelRateLimit 在已知目标 channel 时叠加 per-channel 覆盖（无覆盖则跳过）。
func (s *Service) checkChannelRateLimit(w http.ResponseWriter, channelID string) bool {
	cfg := s.configSvc.GetRateLimitConfig()
	if !cfg.Enabled || channelID == "" {
		return true
	}
	ov := cfg.ChannelOverrides[channelID]
	if ov == nil {
		return true
	}
	if ok, reason := s.rateLimiter.admit(channelRateKey(channelID), effectiveRateLimit(cfg, ov)); !ok {
		writeRateLimitError(w, reason)
		return false
	}
	return true
}

// recordUsageAndTokens 落账用量，并把 token 消耗回填到限流器的全局与 channel 窗口。
func (s *Service) recordUsageAndTokens(c *usageCapture, meta usageMeta) {
	tokens := c.recordUsage(s.currentUsageService(), meta)
	if tokens <= 0 {
		return
	}
	s.rateLimiter.addTokens(rateLimitGlobalKey, tokens)
	if meta.ChannelID != "" {
		s.rateLimiter.addTokens(channelRateKey(meta.ChannelID), tokens)
	}
}
