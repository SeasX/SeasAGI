package mitm

import (
	"context"
	"fmt"
	"testing"
)

// 注意：TrustInstaller 和 SystemProxySetter 的真实集成测试需要系统权限，
// 此处使用 mock 接口验证幂等性和回滚逻辑。

func TestTrustInstallUninstallMock(t *testing.T) {
	ti := &mockTrustInstaller{}

	installed, _ := ti.IsInstalled()
	if installed {
		t.Error("should not be installed initially")
	}

	if err := ti.Install([]byte("fake-ca-pem")); err != nil {
		t.Fatalf("Install: %v", err)
	}
	installed, _ = ti.IsInstalled()
	if !installed {
		t.Error("should be installed after Install")
	}

	if err := ti.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	installed, _ = ti.IsInstalled()
	if installed {
		t.Error("should not be installed after Uninstall")
	}
}

func TestTrustInstallIdempotentMock(t *testing.T) {
	ti := &mockTrustInstaller{}
	_ = ti.Install([]byte("fake-ca-pem"))
	_ = ti.Install([]byte("fake-ca-pem"))
	installed, _ := ti.IsInstalled()
	if !installed {
		t.Error("should still be installed")
	}
}

func TestTrustUninstallNotInstalledMock(t *testing.T) {
	ti := &mockTrustInstaller{}
	if err := ti.Uninstall(); err != nil {
		t.Errorf("Uninstall when not installed should not error: %v", err)
	}
}

func TestProxySetClearMock(t *testing.T) {
	sps := &mockSystemProxySetter{}

	active, _ := sps.IsActive()
	if active {
		t.Error("should not be active initially")
	}

	if err := sps.Set("127.0.0.1:8080"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	active, _ = sps.IsActive()
	if !active {
		t.Error("should be active after Set")
	}

	if err := sps.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	active, _ = sps.IsActive()
	if active {
		t.Error("should not be active after Clear")
	}
}

func TestProxyClearIdempotentMock(t *testing.T) {
	sps := &mockSystemProxySetter{}
	if err := sps.Clear(); err != nil {
		t.Errorf("Clear when not set should not error: %v", err)
	}
	if err := sps.Clear(); err != nil {
		t.Errorf("second Clear should not error: %v", err)
	}
}

func TestProxyClearNotSetMock(t *testing.T) {
	sps := &mockSystemProxySetter{}
	if err := sps.Clear(); err != nil {
		t.Errorf("Clear when not set should not error: %v", err)
	}
}

func TestFullStartStopWithSystemProxyMock(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	trust := &mockTrustInstaller{}
	sps := &mockSystemProxySetter{}
	mgr.SetTrustInstaller(trust)
	mgr.SetSystemProxySetter(sps)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	status := mgr.GetStatus()
	if status.State != StateRunning {
		t.Fatalf("state: got %s, want running", status.State)
	}
	if !status.SystemProxy {
		t.Error("SystemProxy should be true")
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	active, _ := sps.IsActive()
	if active {
		t.Error("system proxy should be cleared after Stop")
	}
}

func TestStartSystemProxyFailureRollbackMock(t *testing.T) {
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	rules := NewDefaultRules()
	mgr := NewManager(ca, rules, "http://127.0.0.1:9999")

	trust := &mockTrustInstaller{}
	sps := &mockSystemProxySetter{setErr: fmt.Errorf("permission denied")}
	mgr.SetTrustInstaller(trust)
	mgr.SetSystemProxySetter(sps)

	_ = mgr.Start(context.Background())

	status := mgr.GetStatus()
	if status.State != StateError {
		t.Errorf("state: got %s, want error", status.State)
	}
}
