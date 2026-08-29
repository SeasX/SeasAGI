package tunnel

import "testing"

func TestNgrokTunnelStart(t *testing.T) {
	mgr := NewNgrokTunnelManager()

	// 无 authtoken 应报错
	err := mgr.Start("8080", "")
	if err == nil {
		t.Fatal("expected error for missing authtoken")
	}

	status := mgr.GetStatus()
	if status.Phase != PhaseNeedsAuth {
		t.Fatalf("expected needs_auth, got %s", status.Phase)
	}

	// 有 authtoken 应成功
	err = mgr.Start("8080", "test-token")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	status = mgr.GetStatus()
	if status.Phase != PhaseRunning {
		t.Fatalf("expected running, got %s", status.Phase)
	}
	if status.PublicURL == "" {
		t.Fatal("expected non-empty public URL")
	}
}

func TestNgrokTunnelStatus(t *testing.T) {
	mgr := NewNgrokTunnelManager()
	mgr.Start("8080", "token")

	status := mgr.GetStatus()
	if !mgr.IsRunning() {
		t.Fatal("should be running")
	}
	if status.Phase != PhaseRunning {
		t.Fatalf("expected running, got %s", status.Phase)
	}
}

func TestNgrokTunnelStop(t *testing.T) {
	mgr := NewNgrokTunnelManager()
	mgr.Start("8080", "token")

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status := mgr.GetStatus()
	if status.Phase != PhaseStopped {
		t.Fatalf("expected stopped, got %s", status.Phase)
	}
	if mgr.IsRunning() {
		t.Fatal("should not be running after stop")
	}
}

func TestNgrokTunnelDoubleStart(t *testing.T) {
	mgr := NewNgrokTunnelManager()
	mgr.Start("8080", "token")

	err := mgr.Start("8080", "token")
	if err == nil {
		t.Fatal("expected error for double start")
	}
}

func TestNgrokTunnelParseURL(t *testing.T) {
	status := NgrokTunnelStatus{
		Phase:    PhaseRunning,
		PublicURL: "https://my-tunnel.ngrok.io",
	}
	url := ParseTunnelURL(status)
	if url != "my-tunnel.ngrok.io" {
		t.Fatalf("expected my-tunnel.ngrok.io, got %s", url)
	}

	// 非运行状态应返回空
	status.Phase = PhaseStopped
	url = ParseTunnelURL(status)
	if url != "" {
		t.Fatal("expected empty URL for stopped tunnel")
	}
}
