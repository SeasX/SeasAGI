package network

import "testing"

func TestNoopProxySetter(t *testing.T) {
	n := &noopProxySetter{}
	if err := n.Set("127.0.0.1:8080"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := n.Clear(); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	active, err := n.IsActive()
	if err != nil || active {
		t.Fatalf("IsActive = (%v, %v), want (false, nil)", active, err)
	}
	addr, err := n.CurrentAddr()
	if err != nil || addr != "" {
		t.Fatalf("CurrentAddr = (%q, %v), want (\"\", nil)", addr, err)
	}
}

func TestCallbackRegistration(t *testing.T) {
	woke := false
	changed := false
	OnWake(func() { woke = true })
	OnNetworkChange(func() { changed = true })

	networkStateMu.RLock()
	wakeCB := onWakeCB
	netCB := onNetworkCB
	networkStateMu.RUnlock()

	if wakeCB == nil || netCB == nil {
		t.Fatal("expected both callbacks to be registered")
	}
	wakeCB()
	netCB()
	if !woke || !changed {
		t.Fatalf("callbacks not invoked: woke=%v changed=%v", woke, changed)
	}
}
