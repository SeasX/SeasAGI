package providers

import (
	"testing"
	"time"
)

func TestPenaltyManagerAccumulateAndCap(t *testing.T) {
	pm := NewPenaltyManager()

	for i := 0; i < 4; i++ {
		pm.RecordRateLimit("k")
	}
	// 首次 3，之后每命中 +3，累计封顶 10
	if got := pm.GetPenalty("k"); got != pm.maxPenalty {
		t.Fatalf("penalty = %d, want capped at %d", got, pm.maxPenalty)
	}
}

func TestPenaltyManagerSuccessDecays(t *testing.T) {
	pm := NewPenaltyManager()
	pm.RecordRateLimit("k") // 3
	pm.RecordSuccess("k")   // 2
	if got := pm.GetPenalty("k"); got != 2 {
		t.Fatalf("penalty after success = %d, want 2", got)
	}
	pm.RecordSuccess("k") // 1
	pm.RecordSuccess("k") // 0 → 删除
	if got := pm.GetPenalty("k"); got != 0 {
		t.Fatalf("penalty after full decay = %d, want 0", got)
	}
}

func TestPenaltyManagerResetAndGetAll(t *testing.T) {
	pm := NewPenaltyManager()
	pm.RecordRateLimit("a")
	pm.RecordRateLimit("b")

	all := pm.GetAllPenalties()
	if len(all) != 2 || all["a"] != 3 || all["b"] != 3 {
		t.Fatalf("GetAllPenalties() = %v, want a=3 b=3", all)
	}

	pm.Reset("a")
	if _, ok := pm.GetAllPenalties()["a"]; ok {
		t.Error("Reset should remove entry")
	}
	if pm.GetPenalty("a") != 0 {
		t.Error("GetPenalty after Reset should be 0")
	}
}

func TestPenaltyManagerTimeDecay(t *testing.T) {
	pm := NewPenaltyManager()
	pm.SetDecayInterval(20 * time.Millisecond)
	pm.RecordRateLimit("k") // 3
	pm.RecordRateLimit("k") // 6

	time.Sleep(45 * time.Millisecond) // 至少经历一步衰减
	got := pm.GetPenalty("k")
	if got >= 6 {
		t.Fatalf("penalty should decay over time, still %d", got)
	}
	if got < 0 {
		t.Fatalf("penalty should not be negative, got %d", got)
	}
}

func TestPenaltyManagerSetPenaltyPer429(t *testing.T) {
	pm := NewPenaltyManager()
	pm.SetPenaltyPer429(5)
	pm.RecordRateLimit("k")
	if got := pm.GetPenalty("k"); got != 5 {
		t.Fatalf("penalty = %d, want 5", got)
	}
	// 非法值应被忽略
	pm.SetPenaltyPer429(0)
	pm.Reset("k")
	pm.RecordRateLimit("k")
	if got := pm.GetPenalty("k"); got != 5 {
		t.Fatalf("penalty after invalid set = %d, want 5", got)
	}
}

func TestCooldownManagerLifecycle(t *testing.T) {
	cm := NewCooldownManager(80 * time.Millisecond)
	defer cm.Stop()

	// 原始 key 明文不应作为状态键落库
	cm.SetCooldown("sk-secret-plaintext")
	if !cm.IsOnCooldown("sk-secret-plaintext") {
		t.Fatal("key should be on cooldown")
	}
	if remaining := cm.GetRemaining("sk-secret-plaintext"); remaining <= 0 {
		t.Fatalf("remaining should be positive, got %v", remaining)
	}

	all := cm.GetAll()
	if len(all) != 1 {
		t.Fatalf("GetAll() len = %d, want 1", len(all))
	}
	for k := range all {
		if k == "sk-secret-plaintext" {
			t.Fatal("state key must be hashed, not the raw API key")
		}
	}

	cm.Clear("sk-secret-plaintext")
	if cm.IsOnCooldown("sk-secret-plaintext") {
		t.Fatal("key should not be on cooldown after Clear")
	}

	// 过期后自动失效
	cm.SetCooldownWithReason("k2", "key_failed")
	time.Sleep(120 * time.Millisecond)
	if cm.IsOnCooldown("k2") {
		t.Fatal("key should expire after duration")
	}
}

func TestCooldownManagerEmptyKeyIgnored(t *testing.T) {
	cm := NewCooldownManager(time.Minute)
	defer cm.Stop()

	cm.SetCooldown("")
	if cm.IsOnCooldown("") {
		t.Fatal("empty key must never be on cooldown")
	}
	if cm.GetRemaining("") != 0 {
		t.Fatal("empty key remaining should be 0")
	}
	if len(cm.GetAll()) != 0 {
		t.Fatal("empty key should not create an entry")
	}
}

func TestCooldownKeyStableAndHashed(t *testing.T) {
	k1 := cooldownKey("same-key")
	k2 := cooldownKey("same-key")
	if k1 != k2 {
		t.Fatal("cooldownKey must be stable for the same input")
	}
	if k1 == "same-key" {
		t.Fatal("cooldownKey must hash the input")
	}
	if cooldownKey("") != "" {
		t.Fatal("cooldownKey of empty string must be empty")
	}
}
