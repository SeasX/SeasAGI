package providers

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// CL-11：同一渠道并发刷新必须合并为一次请求，且轮换后的 refresh token 回写。
func TestOAuthRefresherSingleflightAndPersist(t *testing.T) {
	var calls int32
	var persisted string
	var persistMu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // 放大并发窗口
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-new","refresh_token":"rt-new","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer srv.Close()

	refresher := NewOAuthTokenRefresher(60 * time.Second)
	refresher.SetRefreshTokenPersist(func(channelID, refreshToken string) error {
		persistMu.Lock()
		persisted = channelID + ":" + refreshToken
		persistMu.Unlock()
		return nil
	})

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	results := make([]string, workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = refresher.EnsureValidToken("chan-1", "rt-old", "cid", "secret", srv.URL)
		}(i)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("refresh endpoint called %d times, want 1", got)
	}
	for i := 0; i < workers; i++ {
		if errs[i] != nil {
			t.Fatalf("worker %d error: %v", i, errs[i])
		}
		if results[i] != "at-new" {
			t.Fatalf("worker %d token = %q, want at-new", i, results[i])
		}
	}

	persistMu.Lock()
	got := persisted
	persistMu.Unlock()
	if got != "chan-1:rt-new" {
		t.Fatalf("rotated refresh token not persisted, got %q", got)
	}

	if cached := refresher.GetToken("chan-1"); cached == nil || cached.RefreshToken != "rt-new" {
		t.Fatalf("cached token should carry rotated refresh token, got %+v", cached)
	}
}
