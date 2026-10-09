package providers

import (
	"testing"
	"time"
)

func TestCircuitBreakerOpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	if !cb.Allow() {
		t.Fatal("breaker should allow before any failures")
	}
	if cb.State() != "closed" {
		t.Fatalf("state = %s, want closed", cb.State())
	}

	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Fatalf("state after 1 failure = %s, want closed", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("breaker should still allow below threshold")
	}

	cb.RecordFailure()
	if cb.State() != "open" {
		t.Fatalf("state after reaching threshold = %s, want open", cb.State())
	}
	if cb.Allow() {
		t.Fatal("open breaker must reject requests during reset window")
	}
}

func TestCircuitBreakerRecoversAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(1, 40*time.Millisecond)
	cb.RecordFailure()
	if cb.Allow() {
		t.Fatal("should be open right after failure")
	}

	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("should allow after reset timeout")
	}
	// Allow() 在超时后重置计数
	if cb.State() != "closed" {
		t.Fatalf("state after reset = %s, want closed", cb.State())
	}
}

func TestCircuitBreakerHalfOpenState(t *testing.T) {
	cb := NewCircuitBreaker(1, 40*time.Millisecond)
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	// 未调用 Allow（未触发重置）时，计数仍超阈值但窗口已过 → half-open
	if cb.State() != "half-open" {
		t.Fatalf("state = %s, want half-open", cb.State())
	}
}

func TestCircuitBreakerRecordSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	if cb.State() != "closed" {
		t.Fatalf("state after success = %s, want closed", cb.State())
	}
	// 计数已重置，再两次失败仍未达阈值
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != "closed" {
		t.Fatalf("state = %s, want closed after reset + 2 failures", cb.State())
	}
}
