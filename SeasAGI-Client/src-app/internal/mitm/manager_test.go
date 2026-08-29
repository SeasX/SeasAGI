package mitm

import (
	"context"
	"sync"
	"testing"
	"time"
)

// mockTrustInstaller 是 TrustInstaller 的测试 mock。
type mockTrustInstaller struct {
	installed   bool
	installErr  error
	uninstallMu sync.Mutex
	uninstalled bool
}

func (m *mockTrustInstaller) Install(caPEM []byte) error {
	if m.installErr != nil {
		return m.installErr
	}
	m.installed = true
	return nil
}

func (m *mockTrustInstaller) Uninstall() error {
	m.uninstallMu.Lock()
	defer m.uninstallMu.Unlock()
	m.installed = false
	m.uninstalled = true
	return nil
}

func (m *mockTrustInstaller) IsInstalled() (bool, error) {
	return m.installed, nil
}

// mockSystemProxySetter 是 SystemProxySetter 的测试 mock。
type mockSystemProxySetter struct {
	mu         sync.Mutex
	active     bool
	setErr     error
	clearErr   error
	setCount   int
	clearCount int
}

func (m *mockSystemProxySetter) Set(addr string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.mu.Lock()
	m.active = true
	m.setCount++
	m.mu.Unlock()
	return nil
}

func (m *mockSystemProxySetter) Clear() error {
	if m.clearErr != nil {
		return m.clearErr
	}
	m.mu.Lock()
	m.active = false
	m.clearCount++
	m.mu.Unlock()
	return nil
}

func (m *mockSystemProxySetter) IsActive() (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active, nil
}

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999") // 不存在的 gateway，测试不实际转发
	return mgr
}

func TestStartStopLifecycle(t *testing.T) {
	mgr := setupTestManager(t)

	status := mgr.GetStatus()
	if status.State != StateStopped {
		t.Fatalf("initial state: got %s, want stopped", status.State)
	}

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	status = mgr.GetStatus()
	if status.State != StateRunning {
		t.Fatalf("after Start: got %s, want running", status.State)
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	status = mgr.GetStatus()
	if status.State != StateStopped {
		t.Fatalf("after Stop: got %s, want stopped", status.State)
	}
}

func TestStartIdempotent(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// 再次 Start 不应报错
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("second Start should be idempotent: %v", err)
	}

	_ = mgr.Stop()
}

func TestStopIdempotent(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	_ = mgr.Stop()
	// 再次 Stop 不应 panic
	if err := mgr.Stop(); err != nil {
		t.Fatalf("second Stop should be idempotent: %v", err)
	}
}

func TestGetStatus(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	status := mgr.GetStatus()
	if status.State != StateRunning {
		t.Errorf("state: got %s, want running", status.State)
	}
	if status.ProxyPort != 8080 {
		t.Errorf("proxy_port: got %d, want 8080", status.ProxyPort)
	}
	if status.RulesCount != 6 {
		t.Errorf("rules_count: got %d, want 6", status.RulesCount)
	}

	_ = mgr.Stop()
}

func TestManagerRulesDynamicUpdate(t *testing.T) {
	mgr := setupTestManager(t)

	initial := mgr.GetStatus().RulesCount
	if err := mgr.AddRule("api.test.com"); err != nil {
		t.Fatalf("AddRule: %v", err)
	}
	if mgr.GetStatus().RulesCount != initial+1 {
		t.Errorf("count after Add: got %d, want %d", mgr.GetStatus().RulesCount, initial+1)
	}

	if err := mgr.RemoveRule("api.test.com"); err != nil {
		t.Fatalf("RemoveRule: %v", err)
	}
	if mgr.GetStatus().RulesCount != initial {
		t.Errorf("count after Remove: got %d, want %d", mgr.GetStatus().RulesCount, initial)
	}
}

func TestStartProxyFailureRollback(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}

	// 先占用 8080 端口使 Proxy 启动失败
	// 使用一个已占用的端口——创建第一个 Manager 占用端口
	mgr1 := setupTestManager(t)
	ctx := context.Background()
	_ = mgr1.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// 第二个 Manager 尝试用同一端口
	rules := NewDefaultRules()
	mgr2 := NewManager(ca, rules, "http://127.0.0.1:9999")

	if err := mgr2.Start(ctx); err != nil {
		t.Fatalf("Start should not return error (records in status): %v", err)
	}

	status := mgr2.GetStatus()
	if status.State != StateError {
		t.Errorf("state: got %s, want error", status.State)
	}
	if status.LastError == "" {
		t.Error("last_error should not be empty on failure")
	}

	_ = mgr1.Stop()
	_ = mgr2.Stop()
}

func TestManagerStartWithTrustAndProxy(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	trust := &mockTrustInstaller{}
	proxy := &mockSystemProxySetter{}
	mgr.SetTrustInstaller(trust)
	mgr.SetSystemProxySetter(proxy)

	ctx := context.Background()
	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	status := mgr.GetStatus()
	if status.State != StateRunning {
		t.Errorf("state: got %s, want running", status.State)
	}
	if !status.CAInstalled {
		t.Error("CAInstalled should be true")
	}
	if !status.SystemProxy {
		t.Error("SystemProxy should be true")
	}
	if !trust.installed {
		t.Error("trust should be installed")
	}

	_ = mgr.Stop()

	// Stop 后系统代理应清除
	proxy.mu.Lock()
	clearCount := proxy.clearCount
	proxy.mu.Unlock()
	if clearCount == 0 {
		t.Error("system proxy should have been cleared on Stop")
	}
}

func TestManagerStopCleansAll(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	proxy := &mockSystemProxySetter{}
	mgr.SetSystemProxySetter(proxy)

	ctx := context.Background()
	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	_ = mgr.Stop()

	active, _ := proxy.IsActive()
	if active {
		t.Error("system proxy should be inactive after Stop")
	}

	status := mgr.GetStatus()
	if status.State != StateStopped {
		t.Errorf("state: got %s, want stopped", status.State)
	}
}

func TestStartCleansResidualSystemProxy(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	proxy := &mockSystemProxySetter{}
	// 模拟残留：proxy 已 active
	proxy.active = true
	mgr.SetSystemProxySetter(proxy)

	ctx := context.Background()
	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// 残留代理应被清除（clearCount >= 1 来自清理，然后 Set 再 active）
	proxy.mu.Lock()
	setCount := proxy.setCount
	proxy.mu.Unlock()
	if setCount < 1 {
		t.Error("system proxy should have been set after residual cleanup")
	}

	_ = mgr.Stop()
}

func TestStartIdempotentAlreadyRunning(t *testing.T) {
	mgr := setupTestManager(t)
	ctx := context.Background()

	_ = mgr.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// 再次 Start 不应报错且不应改变状态
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("second Start should be idempotent: %v", err)
	}

	status := mgr.GetStatus()
	if status.State != StateRunning {
		t.Errorf("state should remain running, got %s", status.State)
	}

	_ = mgr.Stop()
}

func TestStartProxyNotHealthyGate(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	proxy := &mockSystemProxySetter{}
	mgr.SetSystemProxySetter(proxy)

	// 先占用 8080 端口使 Proxy 启动失败
	mgr1 := setupTestManager(t)
	ctx := context.Background()
	_ = mgr1.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// mgr2 启动会因端口占用失败，proxy 不健康，不应设置系统代理
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start should not return error: %v", err)
	}

	status := mgr.GetStatus()
	if status.State != StateError {
		t.Errorf("state: got %s, want error", status.State)
	}
	if status.SystemProxy {
		t.Error("system proxy should not be set when proxy is unhealthy")
	}

	_ = mgr1.Stop()
	_ = mgr.Stop()
}
