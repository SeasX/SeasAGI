package routing

import (
	"sync"
	"time"
)

// latencyTracker tracks per-channel rolling latency samples and computes P95.
type latencyTracker struct {
	mu       sync.RWMutex
	samples  map[string][]latencySample // channel_id → samples
	maxAge   time.Duration
	maxItems int
}

type latencySample struct {
	ts        time.Time
	latencyMs float64
}

var defaultLatencyTracker = &latencyTracker{
	samples:  make(map[string][]latencySample),
	maxAge:   5 * time.Minute,
	maxItems: 50,
}

// RecordLatency records a latency sample for a channel.
func RecordLatency(channelID string, latencyMs float64) {
	defaultLatencyTracker.record(channelID, latencyMs)
}

func (lt *latencyTracker) record(channelID string, latencyMs float64) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	s := latencySample{ts: time.Now(), latencyMs: latencyMs}
	samples := lt.samples[channelID]
	samples = append(samples, s)

	// Trim old samples
	cutoff := time.Now().Add(-lt.maxAge)
	start := 0
	for start < len(samples) && samples[start].ts.Before(cutoff) {
		start++
	}
	if start > 0 {
		samples = samples[start:]
	}

	// Cap items
	if len(samples) > lt.maxItems {
		samples = samples[len(samples)-lt.maxItems:]
	}

	lt.samples[channelID] = samples
}

// GetP95Latency returns the P95 latency in ms for a channel, or 0 if no samples.
func GetP95Latency(channelID string) float64 {
	return defaultLatencyTracker.p95(channelID)
}

func (lt *latencyTracker) p95(channelID string) float64 {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	samples := lt.samples[channelID]
	if len(samples) == 0 {
		return 0
	}

	// Copy and sort
	vals := make([]float64, len(samples))
	for i, s := range samples {
		vals[i] = s.latencyMs
	}
	sortFloat64s(vals)

	idx := int(float64(len(vals)) * 0.95)
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx]
}

// GetAvgLatency returns the average latency in ms for a channel, or 0 if no samples.
func GetAvgLatency(channelID string) float64 {
	return defaultLatencyTracker.avg(channelID)
}

func (lt *latencyTracker) avg(channelID string) float64 {
	lt.mu.RLock()
	defer lt.mu.RUnlock()

	samples := lt.samples[channelID]
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range samples {
		sum += s.latencyMs
	}
	return sum / float64(len(samples))
}

func sortFloat64s(a []float64) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
