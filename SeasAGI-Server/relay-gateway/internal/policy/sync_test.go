package policy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/channel"
)

func testStore(t *testing.T) *channel.Store {
	t.Helper()
	t.Setenv("RELAY_DATA_DIR", t.TempDir())
	t.Setenv("RELAY_DATA_KEY", "relay-sync-test-key-0123456789abcdef-xyz")
	s, err := channel.NewStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

type fakePlatform struct {
	mu       sync.Mutex
	channels []map[string]any
	version  string
	status   int
	delay    time.Duration
	auths    []string
	inm      []string
	calls    atomic.Int32
}

func (f *fakePlatform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != relayConfigPath {
		http.NotFound(w, r)
		return
	}
	f.auths = append(f.auths, r.Header.Get("Authorization"))
	f.inm = append(f.inm, r.Header.Get("If-None-Match"))
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.status != 0 && f.status != http.StatusOK {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
		return
	}
	if r.Header.Get("If-None-Match") == `"`+f.version+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "version": f.version, "channels": f.channels})
}

func (f *fakePlatform) set(version string, channels ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.version = version
	f.channels = channels
}

func ch(id, key, model string) map[string]any {
	return map[string]any{"channel_id": id, "provider_type": "openai", "display_name": id, "base_url": "https://" + id + ".example", "api_key": key, "enabled": true, "models": []string{model}, "weight": 100}
}

func newTestSyncer(url string, store Applier) *Syncer {
	return NewSyncer(Config{PlatformURL: url, Token: "internal-secret", Interval: time.Minute, Timeout: 2 * time.Second}, store)
}

// 同步带服务间令牌，应用后立即影响路由；未变化时用 If-None-Match 走 304。
func TestSyncAppliesSnapshotAndAffectsRouting(t *testing.T) {
	store := testStore(t)
	fp := &fakePlatform{}
	fp.set("v1", ch("a", "sk-a", "gpt-4o"))
	srv := httptest.NewServer(fp)
	defer srv.Close()
	s := newTestSyncer(srv.URL, store)

	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
	got, upstream := store.ResolveChannel("gpt-4o")
	if got == nil || got.ChannelID != "a" || got.APIKey != "sk-a" || upstream != "gpt-4o" {
		t.Fatalf("synced channel not routed: %+v", got)
	}
	if fp.auths[0] != "Bearer internal-secret" {
		t.Fatalf("Authorization = %q", fp.auths[0])
	}

	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatalf("second SyncOnce: %v", err)
	}
	if fp.inm[1] != `"v1"` {
		t.Fatalf("If-None-Match = %q, want applied version", fp.inm[1])
	}

	// 控制面改动：换渠道与模型 → relay 路由随之变化。
	fp.set("v2", ch("b", "sk-b", "claude-sonnet"))
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.ResolveChannel("gpt-4o"); got != nil {
		t.Fatal("channel removed in control plane still routable")
	}
	if got, _ := store.ResolveChannel("claude-sonnet"); got == nil || got.APIKey != "sk-b" {
		t.Fatalf("new control-plane channel not routable: %+v", got)
	}
	st := s.Status()
	if st.AppliedVersion != "v2" || st.ConsecutiveFailures != 0 || st.Channels != 1 {
		t.Fatalf("status = %+v", st)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "sk-") {
		t.Fatalf("status leaks credentials: %s", raw)
	}
}

// 失败（HTTP 错误、超时、非法快照）都不得改动已应用的配置。
func TestSyncFailuresKeepLastKnownGood(t *testing.T) {
	store := testStore(t)
	fp := &fakePlatform{}
	fp.set("v1", ch("a", "sk-a", "gpt-4o"))
	srv := httptest.NewServer(fp)
	defer srv.Close()
	s := newTestSyncer(srv.URL, store)
	if err := s.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}

	check := func(label string) {
		t.Helper()
		if got, _ := store.ResolveChannel("gpt-4o"); got == nil || got.APIKey != "sk-a" {
			t.Fatalf("%s: last known good config lost", label)
		}
	}

	fp.mu.Lock()
	fp.status = http.StatusInternalServerError
	fp.mu.Unlock()
	if err := s.SyncOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("500 not surfaced: %v", err)
	}
	check("http 500")

	fp.mu.Lock()
	fp.status = 0
	fp.delay = 300 * time.Millisecond
	fp.mu.Unlock()
	s.client.Timeout = 50 * time.Millisecond
	if err := s.SyncOnce(context.Background()); err == nil {
		t.Fatal("timeout not surfaced")
	}
	check("timeout")
	s.client.Timeout = 2 * time.Second

	fp.mu.Lock()
	fp.delay = 0
	fp.mu.Unlock()
	bad := ch("", "sk-x", "m") // 缺 channel_id → 整份快照拒绝
	fp.set("v-bad", ch("c", "sk-c", "other"), bad)
	if err := s.SyncOnce(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid relay config") {
		t.Fatalf("invalid snapshot not rejected: %v", err)
	}
	check("invalid snapshot")
	if got, _ := store.GetChannel("c"); got != nil {
		t.Fatal("partially applied invalid snapshot")
	}
	if st := s.Status(); st.ConsecutiveFailures != 3 || st.AppliedVersion != "v1" || st.LastError == "" {
		t.Fatalf("status after failures = %+v", st)
	}
}

func TestSyncFailsClosedWithoutToken(t *testing.T) {
	fp := &fakePlatform{}
	srv := httptest.NewServer(fp)
	defer srv.Close()
	s := NewSyncer(Config{PlatformURL: srv.URL}, testStore(t))
	if err := s.SyncOnce(context.Background()); err == nil {
		t.Fatal("sync without token must fail")
	}
	if fp.calls.Load() != 0 {
		t.Fatal("anonymous request sent")
	}
}

func TestCheckTransport(t *testing.T) {
	cases := []struct {
		url      string
		insecure bool
		ok       bool
	}{
		{"https://platform.internal:9318/x", false, true},
		{"http://127.0.0.1:9318/x", false, true},
		{"http://localhost:9318/x", false, true},
		{"http://[::1]:9318/x", false, true},
		{"http://10.0.0.5:9318/x", false, false},
		{"http://10.0.0.5:9318/x", true, true},
		{"ftp://10.0.0.5/x", true, false},
	}
	for _, tc := range cases {
		if err := checkTransport(tc.url, tc.insecure); (err == nil) != tc.ok {
			t.Errorf("checkTransport(%q, %v) = %v, want ok=%v", tc.url, tc.insecure, err, tc.ok)
		}
	}
}

func TestValidateRejectsBadSnapshots(t *testing.T) {
	good := relayChannel{ChannelID: "a", BaseURL: "https://a"}
	for name, cfg := range map[string]relayConfig{
		"schema":    {SchemaVersion: 2, Version: "v", Channels: []relayChannel{good}},
		"version":   {SchemaVersion: 1, Channels: []relayChannel{good}},
		"nil list":  {SchemaVersion: 1, Version: "v"},
		"duplicate": {SchemaVersion: 1, Version: "v", Channels: []relayChannel{good, good}},
		"base_url":  {SchemaVersion: 1, Version: "v", Channels: []relayChannel{{ChannelID: "a", BaseURL: "javascript:alert(1)"}}},
		"gray":      {SchemaVersion: 1, Version: "v", Channels: []relayChannel{{ChannelID: "a", BaseURL: "https://a", GrayPercent: 101}}},
	} {
		if _, err := cfg.validate(); err == nil {
			t.Errorf("%s: invalid snapshot accepted", name)
		}
	}
	// 空列表是合法快照（控制面删除了全部渠道）。
	if _, err := (&relayConfig{SchemaVersion: 1, Version: "v", Channels: []relayChannel{}}).validate(); err != nil {
		t.Fatalf("empty snapshot rejected: %v", err)
	}
}

// Run 在失败后退避重试，控制面恢复后完成同步；ctx 取消后退出。
func TestRunRetriesUntilControlPlaneRecovers(t *testing.T) {
	store := testStore(t)
	fp := &fakePlatform{status: http.StatusServiceUnavailable}
	fp.set("v1", ch("a", "sk-a", "gpt-4o"))
	srv := httptest.NewServer(fp)
	defer srv.Close()
	s := newTestSyncer(srv.URL, store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	time.Sleep(200 * time.Millisecond)
	fp.mu.Lock()
	fp.status = 0
	fp.mu.Unlock()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := store.ResolveChannel("gpt-4o"); got != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got, _ := store.ResolveChannel("gpt-4o"); got == nil {
		t.Fatal("syncer did not recover after control plane came back")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after cancel")
	}
	if fp.calls.Load() < 2 {
		t.Fatalf("expected a retry, got %d calls", fp.calls.Load())
	}
}
