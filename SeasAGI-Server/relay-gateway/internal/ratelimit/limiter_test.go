package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	l := NewLimiter(60)
	if l == nil {
		t.Fatal("NewLimiter() returned nil")
	}
	if l.maxRate != 60 {
		t.Errorf("maxRate = %f, want 60", l.maxRate)
	}
	if l.buckets == nil {
		t.Error("buckets map is nil")
	}
	if l.policies == nil {
		t.Error("policies map is nil")
	}
}

func TestNewLimiter_DefaultRate(t *testing.T) {
	l := NewLimiter(0)
	if l.maxRate != 60 {
		t.Errorf("maxRate = %f, want 60 (default)", l.maxRate)
	}

	l2 := NewLimiter(-10)
	if l2.maxRate != 60 {
		t.Errorf("maxRate = %f, want 60 (default)", l2.maxRate)
	}
}

func TestAllow_FirstRequestReturnsTrue(t *testing.T) {
	l := NewLimiter(60)
	if !l.allow("test-key") {
		t.Error("first request should be allowed")
	}
}

func TestAllow_ReturnsFalseWhenRateExceeded(t *testing.T) {
	l := NewLimiter(10) // 10 RPM

	// Consume all 10 tokens
	for i := 0; i < 10; i++ {
		if !l.allow("test") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// Next request should be denied
	if l.allow("test") {
		t.Error("request after rate exceeded should be denied")
	}
}

func TestAllow_RefillsTokensOverTime(t *testing.T) {
	l := NewLimiter(60) // 60 RPM = 1 token per second

	// Consume all tokens
	for i := 0; i < 60; i++ {
		l.allow("test")
	}

	// Verify rate limited
	if l.allow("test") {
		t.Fatal("should be rate limited after consuming all tokens")
	}

	// Wait for 1 token refill (1 second + buffer)
	time.Sleep(1100 * time.Millisecond)

	if !l.allow("test") {
		t.Error("should be allowed after refill")
	}
}

func TestSetPolicy_UpdatesRateForKey(t *testing.T) {
	l := NewLimiter(60)
	l.SetPolicy("test", 5) // 5 RPM

	// Should only allow 5 requests
	for i := 0; i < 5; i++ {
		if !l.allow("test") {
			t.Fatalf("request %d should be allowed with policy", i+1)
		}
	}

	// 6th request should be denied
	if l.allow("test") {
		t.Error("request should be denied after policy limit exceeded")
	}
}

func TestSetPolicy_ZeroRPM(t *testing.T) {
	l := NewLimiter(60)
	l.SetPolicy("test", 5) // set custom policy
	l.SetPolicy("test", 0) // delete policy, should fall back to maxRate

	// Should allow up to 60 requests now (default maxRate)
	for i := 0; i < 60; i++ {
		if !l.allow("test") {
			t.Fatalf("request %d should be allowed after policy removal", i+1)
		}
	}

	// 61st should be denied
	if l.allow("test") {
		t.Error("request should be denied after default rate limit exceeded")
	}
}

func TestCleanupLoop_RemovesStaleBuckets(t *testing.T) {
	l := NewLimiter(60)
	l.allow("test") // create a bucket

	// Verify bucket exists
	b, ok := l.buckets["test"]
	if !ok {
		t.Fatal("bucket should exist after allow()")
	}

	// Manually set lastTime to be > 5 minutes ago
	b.lastTime = time.Now().Add(-6 * time.Minute)

	// Manually run cleanup logic (same as CleanupLoop)
	l.mu.Lock()
	now := time.Now()
	for k, b := range l.buckets {
		if now.Sub(b.lastTime) > 5*time.Minute {
			delete(l.buckets, k)
		}
	}
	l.mu.Unlock()

	// Verify bucket was removed
	if _, ok := l.buckets["test"]; ok {
		t.Error("stale bucket should have been removed by cleanup")
	}
}

func TestCleanupLoop_KeepsRecentBuckets(t *testing.T) {
	l := NewLimiter(60)
	l.allow("test") // create a bucket

	// Run cleanup logic (bucket is recent, should not be removed)
	l.mu.Lock()
	now := time.Now()
	for k, b := range l.buckets {
		if now.Sub(b.lastTime) > 5*time.Minute {
			delete(l.buckets, k)
		}
	}
	l.mu.Unlock()

	if _, ok := l.buckets["test"]; !ok {
		t.Error("recent bucket should not be removed by cleanup")
	}
}

func TestDifferentKeysIndependent(t *testing.T) {
	l := NewLimiter(5) // 5 RPM

	// Consume all tokens for key1
	for i := 0; i < 5; i++ {
		if !l.allow("key1") {
			t.Fatalf("key1 request %d should be allowed", i+1)
		}
	}

	// key1 should be limited now
	if l.allow("key1") {
		t.Error("key1 should be rate limited")
	}

	// key2 should still have full quota (independent keys)
	for i := 0; i < 5; i++ {
		if !l.allow("key2") {
			t.Fatalf("key2 request %d should be allowed (independent keys)", i+1)
		}
	}

	// key2 should now also be limited
	if l.allow("key2") {
		t.Error("key2 should be rate limited after its own quota exhausted")
	}
}

func TestAllow_BurstAtStart(t *testing.T) {
	l := NewLimiter(100) // 100 RPM

	// First request should be allowed
	if !l.allow("test") {
		t.Fatal("first request should be allowed")
	}

	// Check that bucket has maxToken - 1 tokens remaining
	b, ok := l.buckets["test"]
	if !ok {
		t.Fatal("bucket should exist after first request")
	}
	if b.tokens != 99 {
		t.Errorf("tokens after first request = %f, want 99", b.tokens)
	}
	if b.maxToken != 100 {
		t.Errorf("maxToken = %f, want 100", b.maxToken)
	}

	// Consume remaining 99 tokens
	for i := 0; i < 99; i++ {
		if !l.allow("test") {
			t.Fatalf("burst request %d should be allowed", i+2)
		}
	}

	// 101st request should be denied
	if l.allow("test") {
		t.Error("should be rate limited after burst")
	}
}

func TestConcurrentAccess(t *testing.T) {
	l := NewLimiter(1000)
	var wg sync.WaitGroup

	// Concurrently access the limiter from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "concurrent-key"
			l.allow(key)
		}(i)
	}

	wg.Wait()
	// No race condition should occur; test with `go test -race`
}

func TestSetPolicyChangesBucketRate(t *testing.T) {
	l := NewLimiter(60)

	// First create a bucket with default rate (60 RPM)
	l.allow("test")

	// Now set a lower policy
	l.SetPolicy("test", 10)

	// The bucket should be reconfigured on next allow
	// With 10 RPM, we can make 10 requests
	for i := 0; i < 10; i++ {
		if !l.allow("test") {
			t.Fatalf("request %d should be allowed with new policy", i+1)
		}
	}

	// 11th should be denied (10 RPM limit)
	if l.allow("test") {
		t.Error("should be rate limited after policy change")
	}
}