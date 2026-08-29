package tls

import (
	"testing"
	"time"
)

func TestFingerprintChromeProfile(t *testing.T) {
	mgr := NewFingerprintManager(Chrome124)
	p := mgr.GetProfile()
	if p.Name != "chrome_124" {
		t.Fatalf("expected chrome_124, got %s", p.Name)
	}
	if p.ClientSpec.Browser != "chrome" {
		t.Fatalf("expected browser chrome, got %s", p.ClientSpec.Browser)
	}
	if len(p.ClientSpec.CipherSuites) == 0 {
		t.Fatal("chrome profile should have cipher suites")
	}
}

func TestFingerprintFirefoxProfile(t *testing.T) {
	mgr := NewFingerprintManager(Firefox120)
	p := mgr.GetProfile()
	if p.Name != "firefox_120" {
		t.Fatalf("expected firefox_120, got %s", p.Name)
	}
	if p.ClientSpec.Browser != "firefox" {
		t.Fatalf("expected browser firefox, got %s", p.ClientSpec.Browser)
	}
}

func TestFingerprintSetProfile(t *testing.T) {
	mgr := NewFingerprintManager(Chrome124)
	mgr.SetProfile(Firefox120)
	if mgr.GetProfile().Name != "firefox_120" {
		t.Fatal("SetProfile did not switch to firefox")
	}
}

func TestFingerprintNewHTTPClient(t *testing.T) {
	mgr := NewFingerprintManager(Chrome124)
	client, err := mgr.NewHTTPClient(30 * time.Second)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", client.Timeout)
	}
}

func TestFingerprintHTTPClientBreakerOpen(t *testing.T) {
	mgr := NewFingerprintManager(Chrome124)
	// 触发熔断
	for i := 0; i < 3; i++ {
		mgr.RecordFailure()
	}
	_, err := mgr.NewHTTPClient(10 * time.Second)
	if err == nil {
		t.Fatal("expected circuit breaker open error")
	}
}

func TestCircuitBreakerTrips(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second, 600*time.Second)
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.Allow() {
		t.Fatal("breaker should be open after 3 failures")
	}
}

func TestCircuitBreakerRecovery(t *testing.T) {
	cb := NewCircuitBreaker(3, 50*time.Millisecond, 200*time.Millisecond)
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.Allow() {
		t.Fatal("breaker should be open")
	}
	// 等待冷却结束
	time.Sleep(60 * time.Millisecond)
	if !cb.Allow() {
		t.Fatal("breaker should allow after cooldown (half-open)")
	}
	// 成功后恢复
	cb.RecordSuccess()
	if !cb.Allow() {
		t.Fatal("breaker should be closed after success")
	}
}

func TestCircuitBreakerExponentialBackoff(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond, 80*time.Millisecond)

	// 第一次开路 → 用 10ms 开路，翻倍后 currentCooldown=20ms
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
	}
	first := cb.CurrentCooldown()
	if first != 20*time.Millisecond {
		t.Fatalf("expected 20ms after first trip, got %v", first)
	}

	// 冷却后半开 → 再次失败 → 用 20ms 开路，翻倍后 currentCooldown=40ms
	time.Sleep(15 * time.Millisecond)
	cb.Allow() // 进入 half-open
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
	}
	second := cb.CurrentCooldown()
	if second != 40*time.Millisecond {
		t.Fatalf("expected 40ms after second trip, got %v", second)
	}

	// 冷却后半开 → 再次失败 → 用 40ms 开路，翻倍后 currentCooldown=80ms
	time.Sleep(45 * time.Millisecond)
	cb.Allow()
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
	}
	third := cb.CurrentCooldown()
	if third != 80*time.Millisecond {
		t.Fatalf("expected 80ms after third trip, got %v", third)
	}

	// 第四次不应超过 maxCooldown
	time.Sleep(85 * time.Millisecond)
	cb.Allow()
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
	}
	fourth := cb.CurrentCooldown()
	if fourth != 80*time.Millisecond {
		t.Fatalf("expected 80ms (capped), got %v", fourth)
	}
}

func TestCircuitBreakerSuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(3, 30*time.Second, 600*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	if cb.FailureCount() != 0 {
		t.Fatal("failure count should reset after success")
	}
}
