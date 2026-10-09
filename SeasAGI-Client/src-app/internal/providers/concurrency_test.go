package providers

import (
	"sync"
	"testing"
)

func TestConcurrencyLimiterUnlimitedByDefault(t *testing.T) {
	cl := NewConcurrencyLimiter()
	// 未配置 limit（0）→ 无限，总是可获取
	for i := 0; i < 100; i++ {
		if !cl.Acquire("ch") {
			t.Fatal("unlimited channel should always acquire")
		}
	}
	// 未配置时为 0，且不应因 Acquire 而累加计数
	if got := cl.Inflight("ch"); got != 0 {
		t.Fatalf("inflight = %d, want 0 for unlimited channel", got)
	}
}

func TestConcurrencyLimiterCapacity(t *testing.T) {
	cl := NewConcurrencyLimiter()
	cl.SetLimit("ch", 2)
	if cl.GetLimit("ch") != 2 {
		t.Fatalf("GetLimit = %d, want 2", cl.GetLimit("ch"))
	}

	if !cl.Acquire("ch") || !cl.Acquire("ch") {
		t.Fatal("first two acquires should succeed")
	}
	if cl.Acquire("ch") {
		t.Fatal("third acquire should fail at capacity")
	}
	if got := cl.Inflight("ch"); got != 2 {
		t.Fatalf("inflight = %d, want 2", got)
	}

	cl.Release("ch")
	if !cl.Acquire("ch") {
		t.Fatal("acquire should succeed after release")
	}
	if got := cl.Inflight("ch"); got != 2 {
		t.Fatalf("inflight = %d, want 2", got)
	}
}

func TestConcurrencyLimiterReleaseNoop(t *testing.T) {
	cl := NewConcurrencyLimiter()
	cl.SetLimit("ch", 1)
	// 未 Acquire 直接 Release 不应变成负数
	cl.Release("ch")
	cl.Release("unknown")
	if got := cl.Inflight("ch"); got != 0 {
		t.Fatalf("inflight = %d, want 0", got)
	}
}

func TestConcurrencyLimiterRemoveChannel(t *testing.T) {
	cl := NewConcurrencyLimiter()
	cl.SetLimit("ch", 1)
	_ = cl.Acquire("ch")
	cl.RemoveChannel("ch")

	if got := cl.Inflight("ch"); got != 0 {
		t.Fatalf("inflight after remove = %d, want 0", got)
	}
	// 移除后无限制 → 总能获取
	if !cl.Acquire("ch") {
		t.Fatal("channel should be unlimited after removal")
	}
}

func TestConcurrencyLimiterConcurrent(t *testing.T) {
	cl := NewConcurrencyLimiter()
	cl.SetLimit("ch", 4)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cl.Acquire("ch") {
				defer cl.Release("ch")
			}
		}()
	}
	wg.Wait()

	if got := cl.Inflight("ch"); got != 0 {
		t.Fatalf("inflight after concurrent run = %d, want 0", got)
	}
}
