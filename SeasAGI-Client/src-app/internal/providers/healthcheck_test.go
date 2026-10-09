package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// healthTestUpstream 返回一个 /v1/models 上游；healthy 控制返回 200 还是 500。
func healthTestUpstream(t *testing.T, healthy bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if !healthy {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "test-model", "object": "model"}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestHealthCheckerHealthyChannel(t *testing.T) {
	srv := healthTestUpstream(t, true)

	var mu sync.Mutex
	updates := map[string]string{}
	hc := NewHealthChecker(
		func() []HealthChannelInfo {
			return []HealthChannelInfo{{
				ChannelID: "ch1", ChannelType: "byok", ProviderType: "openai",
				BaseURL: srv.URL, Enabled: true, HealthStatus: "unknown",
			}}
		},
		func(id, health string) error {
			mu.Lock()
			updates[id] = health
			mu.Unlock()
			return nil
		},
	)
	hc.SetCheckInterval(time.Hour)

	hc.runOnce()

	mu.Lock()
	defer mu.Unlock()
	if updates["ch1"] != "healthy" {
		t.Fatalf("updateFn got %q, want healthy", updates["ch1"])
	}
	failures, result, _ := hc.GetChannelState("ch1")
	if failures != 0 || result != "healthy" {
		t.Fatalf("state = (%d, %q), want (0, healthy)", failures, result)
	}
}

func TestHealthCheckerUnhealthyAfterMaxFailures(t *testing.T) {
	srv := healthTestUpstream(t, false)

	var mu sync.Mutex
	updates := map[string]string{}
	hc := NewHealthChecker(
		func() []HealthChannelInfo {
			return []HealthChannelInfo{{
				ChannelID: "ch1", ChannelType: "byok", ProviderType: "openai",
				BaseURL: srv.URL, Enabled: true, HealthStatus: "healthy",
			}}
		},
		func(id, health string) error {
			mu.Lock()
			updates[id] = health
			mu.Unlock()
			return nil
		},
	)
	hc.SetMaxFailures(2)

	hc.runOnce() // failures=1，未达阈值
	mu.Lock()
	if _, ok := updates["ch1"]; ok {
		mu.Unlock()
		t.Fatal("should not update health before reaching max failures")
	}
	mu.Unlock()

	hc.runOnce() // failures=2 → 标记 unhealthy
	mu.Lock()
	defer mu.Unlock()
	if updates["ch1"] != "unhealthy" {
		t.Fatalf("updateFn got %q, want unhealthy", updates["ch1"])
	}
}

func TestHealthCheckerSkipsDisabledAndEmpty(t *testing.T) {
	var calls int
	hc := NewHealthChecker(
		func() []HealthChannelInfo {
			return []HealthChannelInfo{{
				ChannelID: "ch1", Enabled: false,
			}}
		},
		func(id, health string) error {
			calls++
			return nil
		},
	)

	hc.runOnce()
	if calls != 0 {
		t.Fatal("disabled channel should not trigger health updates")
	}

	empty := NewHealthChecker(
		func() []HealthChannelInfo { return nil },
		func(id, health string) error { calls++; return nil },
	)
	empty.runOnce()
	if calls != 0 {
		t.Fatal("empty channel list should be a no-op")
	}
}

func TestHealthCheckerStates(t *testing.T) {
	hc := NewHealthChecker(func() []HealthChannelInfo { return nil }, func(string, string) error { return nil })

	if failures, result, check := hc.GetChannelState("nope"); failures != 0 || result != "" || !check.IsZero() {
		t.Fatalf("unknown channel state = (%d, %q, %v), want zero", failures, result, check)
	}
	if len(hc.GetAllStates()) != 0 {
		t.Fatal("GetAllStates should be empty initially")
	}

	hc.SetCheckInterval(0) // 非法值应被忽略
	hc.SetMaxFailures(-1)  // 非法值应被忽略
	if hc.checkInterval != 5*time.Minute {
		t.Fatalf("checkInterval = %v, want default 5m", hc.checkInterval)
	}
	if hc.maxFailures != 3 {
		t.Fatalf("maxFailures = %d, want default 3", hc.maxFailures)
	}
}
