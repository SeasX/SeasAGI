package routing

import (
	"testing"
	"time"
)

func newTestLatencyTracker() *latencyTracker {
	return &latencyTracker{
		samples:  make(map[string][]latencySample),
		maxAge:   5 * time.Minute,
		maxItems: 50,
	}
}

func TestLatencyTrackerP95(t *testing.T) {
	lt := newTestLatencyTracker()
	for i := 1; i <= 20; i++ {
		lt.record("ch", float64(i))
	}
	// 20 个样本，P95 → idx=int(20*0.95)=19 → 最大值 20
	if got := lt.p95("ch"); got != 20 {
		t.Fatalf("p95 = %v, want 20", got)
	}
}

func TestLatencyTrackerAvg(t *testing.T) {
	lt := newTestLatencyTracker()
	lt.record("ch", 10)
	lt.record("ch", 20)
	lt.record("ch", 30)
	if got := lt.avg("ch"); got != 20 {
		t.Fatalf("avg = %v, want 20", got)
	}
}

func TestLatencyTrackerEmpty(t *testing.T) {
	lt := newTestLatencyTracker()
	if got := lt.p95("missing"); got != 0 {
		t.Fatalf("p95 of unknown channel = %v, want 0", got)
	}
	if got := lt.avg("missing"); got != 0 {
		t.Fatalf("avg of unknown channel = %v, want 0", got)
	}
}

func TestLatencyTrackerCapsItems(t *testing.T) {
	lt := newTestLatencyTracker()
	for i := 0; i < 120; i++ {
		lt.record("ch", float64(i))
	}
	lt.mu.RLock()
	n := len(lt.samples["ch"])
	lt.mu.RUnlock()
	if n != lt.maxItems {
		t.Fatalf("sample count = %d, want capped at %d", n, lt.maxItems)
	}
}

func TestLatencyTrackerDropsStaleSamples(t *testing.T) {
	lt := &latencyTracker{
		samples:  make(map[string][]latencySample),
		maxAge:   10 * time.Millisecond,
		maxItems: 50,
	}
	lt.record("ch", 100)
	time.Sleep(30 * time.Millisecond)
	lt.record("ch", 200) // 触发裁剪，旧样本被丢弃

	lt.mu.RLock()
	n := len(lt.samples["ch"])
	lt.mu.RUnlock()
	if n != 1 {
		t.Fatalf("stale samples not trimmed, got %d", n)
	}
}
