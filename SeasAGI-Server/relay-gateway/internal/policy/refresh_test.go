package policy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 策略同步必须走内部端点并携带服务间令牌；
// 此前匿名 GET /api/v1/channels 恒 401，控制面与数据面实际断开（P1-3）。
func TestRefreshChannelsUsesInternalEndpointWithToken(t *testing.T) {
	t.Setenv("INTERNAL_API_TOKEN", "internal-secret")

	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"channel_id":"c1","provider_type":"openai","base_url":"https://example.invalid","enabled":true,"models":["gpt-4o"],"sort_order":1}]}`))
	}))
	defer srv.Close()

	c := &Cache{platformAPIURL: srv.URL, refreshInterval: time.Minute}
	c.refreshChannels()

	if gotPath != "/api/v1/internal/channels/snapshot" {
		t.Fatalf("request path = %q, want internal snapshot endpoint", gotPath)
	}
	if gotAuth != "Bearer internal-secret" {
		t.Fatalf("Authorization = %q, want Bearer internal-secret", gotAuth)
	}
	if len(c.channels) != 1 || c.channels[0].ChannelID != "c1" || len(c.channels[0].Models) != 1 {
		t.Fatalf("snapshot not loaded: %+v", c.channels)
	}
	if c.lastChannelFetch.IsZero() {
		t.Fatal("lastChannelFetch was not updated")
	}
}

// 两个令牌都未配置时不发送匿名请求（fail-closed）。
func TestRefreshChannelsFailsClosedWithoutToken(t *testing.T) {
	t.Setenv("INTERNAL_API_TOKEN", "")
	t.Setenv("ADMIN_SECRET", "")

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Cache{platformAPIURL: srv.URL, refreshInterval: time.Minute}
	c.refreshChannels()

	if calls != 0 {
		t.Fatalf("sent %d anonymous requests, want 0", calls)
	}
}
