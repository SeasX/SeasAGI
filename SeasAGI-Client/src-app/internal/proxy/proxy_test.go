package proxy

import (
	"testing"
	"time"
)

// --- Egress IP 测试 ---

func TestEgressIPCache(t *testing.T) {
	mgr := NewEgressManager("", 5*time.Minute)
	// 手动写入缓存
	mgr.mu.Lock()
	mgr.cache["proxy1:8080"] = &egressEntry{IP: "1.2.3.4", ProxyAddr: "proxy1:8080", CheckedAt: time.Now()}
	mgr.cache[""] = &egressEntry{IP: "5.6.7.8", ProxyAddr: "", CheckedAt: time.Now()}
	mgr.mu.Unlock()

	// 命中缓存
	ip, err := mgr.DetectEgressIP("proxy1:8080")
	if err != nil {
		t.Fatalf("expected cached IP, got error: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", ip)
	}

	// 直连缓存
	ip, err = mgr.DetectEgressIP("")
	if err != nil {
		t.Fatalf("expected cached direct IP, got error: %v", err)
	}
	if ip != "5.6.7.8" {
		t.Fatalf("expected 5.6.7.8, got %s", ip)
	}
}

func TestEgressIPCacheExpiry(t *testing.T) {
	mgr := NewEgressManager("", 50*time.Millisecond)
	mgr.mu.Lock()
	mgr.cache["proxy1:8080"] = &egressEntry{IP: "1.2.3.4", ProxyAddr: "proxy1:8080", CheckedAt: time.Now()}
	mgr.mu.Unlock()

	// 立即命中缓存
	ip, err := mgr.DetectEgressIP("proxy1:8080")
	if err != nil || ip != "1.2.3.4" {
		t.Fatalf("expected cached 1.2.3.4, got %s err=%v", ip, err)
	}

	// 等待缓存过期
	time.Sleep(60 * time.Millisecond)
	// 过期后会尝试真实探测，但 echo 服务不可达，所以应返回错误
	_, err = mgr.DetectEgressIP("proxy1:8080")
	if err == nil {
		t.Fatal("expected error after cache expiry (no echo service)")
	}
}

func TestEgressIPSharingDetection(t *testing.T) {
	mgr := NewEgressManager("", 5*time.Minute)
	mgr.SetRotationGroup("openai", "auth0-group")
	mgr.SetRotationGroup("codex", "auth0-group")
	mgr.SetRotationGroup("anthropic", "anthropic-group")

	// 手动写入缓存 — 两个 provider 共享同一 IP
	mgr.mu.Lock()
	mgr.cache["proxy-a:8080"] = &egressEntry{IP: "10.0.0.1", ProxyAddr: "proxy-a:8080", CheckedAt: time.Now()}
	mgr.cache["proxy-b:8080"] = &egressEntry{IP: "10.0.0.1", ProxyAddr: "proxy-b:8080", CheckedAt: time.Now()}
	mgr.mu.Unlock()

	proxyMap := map[string]string{
		"openai":    "proxy-a:8080",
		"codex":     "proxy-b:8080",
		"anthropic": "proxy-c:8080",
	}

	results := mgr.AnalyzeEgressSharing(proxyMap)
	if len(results) == 0 {
		t.Fatal("expected sharing detection for auth0-group")
	}
	found := false
	for _, r := range results {
		if r.Group == "auth0-group" && r.Shared {
			found = true
			if len(r.SharedIPs) == 0 {
				t.Fatal("shared IPs should not be empty")
			}
		}
	}
	if !found {
		t.Fatal("auth0-group sharing not detected")
	}
}

func TestEgressIPNoSharing(t *testing.T) {
	mgr := NewEgressManager("", 5*time.Minute)
	mgr.SetRotationGroup("openai", "auth0-group")
	mgr.SetRotationGroup("codex", "auth0-group")

	// 不同 IP
	mgr.mu.Lock()
	mgr.cache["proxy-a:8080"] = &egressEntry{IP: "10.0.0.1", ProxyAddr: "proxy-a:8080", CheckedAt: time.Now()}
	mgr.cache["proxy-b:8080"] = &egressEntry{IP: "10.0.0.2", ProxyAddr: "proxy-b:8080", CheckedAt: time.Now()}
	mgr.mu.Unlock()

	proxyMap := map[string]string{
		"openai": "proxy-a:8080",
		"codex":  "proxy-b:8080",
	}

	results := mgr.AnalyzeEgressSharing(proxyMap)
	if len(results) != 0 {
		t.Fatal("should not detect sharing when IPs differ")
	}
}

// --- Health Check 测试 ---

func TestProxyHealthTCPProbe(t *testing.T) {
	// 使用一个不会监听的端口
	hc := NewHealthChecker(100*time.Millisecond, 30*time.Second, 2*time.Second)
	reachable := hc.IsReachable("127.0.0.1:1") // 端口 1 通常不可达
	if reachable {
		t.Fatal("port 1 should not be reachable")
	}
}

func TestProxyHealthCache(t *testing.T) {
	hc := NewHealthChecker(100*time.Millisecond, 30*time.Second, 2*time.Second)

	// 探测不可达地址
	hc.IsReachable("127.0.0.1:1")

	// 再次查询应命中缓存（不健康缓存 2s）
	statuses := hc.GetAllStatuses()
	if len(statuses) == 0 {
		t.Fatal("expected cached health status")
	}
	if statuses[0].Reachable {
		t.Fatal("expected unreachable status in cache")
	}
}

// --- Distribution 测试 ---

func TestProxyDistribution1to1(t *testing.T) {
	pool := []string{"proxy-a:8080", "proxy-b:8080", "proxy-c:8080"}
	providers := []string{"openai", "codex", "anthropic"}

	plan := PlanDistribution(pool, providers)
	if len(plan.Conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", plan.Conflicts)
	}
	if len(plan.Assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(plan.Assignments))
	}
	if !ValidateDistribution(plan.Assignments) {
		t.Fatal("distribution should be valid 1:1")
	}
}

func TestProxyDistributionNoConflict(t *testing.T) {
	pool := []string{"proxy-a:8080", "proxy-b:8080"}
	providers := []string{"openai", "codex"}

	plan := PlanDistribution(pool, providers)
	if !ValidateDistribution(plan.Assignments) {
		t.Fatal("distribution should be valid")
	}
	if plan.Assignments["openai"] == plan.Assignments["codex"] {
		t.Fatal("openai and codex should not share same proxy")
	}
}

func TestProxyDistributionConflict(t *testing.T) {
	pool := []string{"proxy-a:8080"}
	providers := []string{"openai", "codex", "anthropic"}

	plan := PlanDistribution(pool, providers)
	if len(plan.Conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(plan.Conflicts))
	}
	if len(plan.Assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(plan.Assignments))
	}
}

func TestProxyDistributionValidate(t *testing.T) {
	// 两个 provider 共享同一代理 → 无效
	assignments := map[string]string{
		"openai": "proxy-a:8080",
		"codex":  "proxy-a:8080",
	}
	if ValidateDistribution(assignments) {
		t.Fatal("should detect shared proxy as invalid")
	}

	// 1:1 → 有效
	assignments = map[string]string{
		"openai": "proxy-a:8080",
		"codex":  "proxy-b:8080",
	}
	if !ValidateDistribution(assignments) {
		t.Fatal("should be valid 1:1")
	}
}
