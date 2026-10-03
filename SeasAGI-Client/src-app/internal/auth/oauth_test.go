package auth

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordedBody 记录客户端发给服务端的 JSON 请求体。
type recordedBody struct {
	mu      sync.Mutex
	payload map[string]any
}

func (r *recordedBody) record(t *testing.T, req *http.Request) {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.payload = map[string]any{}
	if err := json.Unmarshal(body, &r.payload); err != nil {
		t.Errorf("decode request body: %v", err)
	}
}

func (r *recordedBody) get() map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.payload
}

// startOAuthEnterpriseStub 启动一个模拟企业服务端的 httptest server，
// 覆盖 /auth/oauth/providers、/auth/oauth/authorize 与 /auth/oauth/poll 三个端点，
// 并返回 authorize / poll 请求体记录器。authorize_url 默认指向本服务的授权跳转页。
func startOAuthEnterpriseStub(t *testing.T, authorizeHandler, pollHandler http.HandlerFunc) (*httptest.Server, *recordedBody, *recordedBody) {
	t.Helper()
	authorizeBody, pollBody := &recordedBody{}, &recordedBody{}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/oauth/providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"providers":[{"name":"google","client_id":"g-cid"},{"name":"github","client_id":"h-cid"}]}`))
	})

	var ts *httptest.Server
	mux.HandleFunc("/auth/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		authorizeBody.record(t, r)
		if authorizeHandler != nil {
			authorizeHandler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"session_id":"sess-1","authorize_url":"%s/auth/oauth/start?session_id=sess-1"}`, ts.URL)
	})
	mux.HandleFunc("/auth/oauth/poll", func(w http.ResponseWriter, r *http.Request) {
		pollBody.record(t, r)
		if pollHandler != nil {
			pollHandler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"completed","access_token":"stub-jwt-token","refresh_token":"r-1","expires_in":86400}`))
	})

	ts = httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, authorizeBody, pollBody
}

// oauthBrowserSpy 记录 openBrowser 与 keychain 持久化的行为。
type oauthBrowserSpy struct {
	Opened bool
	URL    string
	Saved  string
}

// stubOAuthBrowser 拦截 openBrowser 与 savePlatformToken，返回行为探针。
// check 在浏览器"打开"时校验授权跳转地址（应为企业服务端授权跳转页）。
func stubOAuthBrowser(t *testing.T, check func(authURL *url.URL)) *oauthBrowserSpy {
	t.Helper()
	origOpen, origSave := openBrowser, savePlatformToken
	t.Cleanup(func() { openBrowser = origOpen; savePlatformToken = origSave })

	spy := &oauthBrowserSpy{}
	openBrowser = func(rawURL string) error {
		spy.Opened = true
		spy.URL = rawURL
		u, err := url.Parse(rawURL)
		if err != nil {
			return err
		}
		if check != nil {
			check(u)
		}
		return nil
	}
	savePlatformToken = func(token string) error { spy.Saved = token; return nil }
	return spy
}

// acceleratePolling 缩短轮询间隔与登录超时，避免测试等待真实的秒级睡眠。
func acceleratePolling(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	origInterval, origTimeout := oauthPollInterval, oauthLoginTimeout
	t.Cleanup(func() { oauthPollInterval, oauthLoginTimeout = origInterval, origTimeout })
	oauthPollInterval, oauthLoginTimeout = interval, timeout
}

func TestFetchOAuthProviders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/oauth/providers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"providers":[{"name":"google","client_id":"g-cid"}]}`))
	}))
	defer ts.Close()
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	providers, err := svc.FetchOAuthProviders()
	if err != nil {
		t.Fatalf("FetchOAuthProviders() error: %v", err)
	}
	if len(providers) != 1 || providers[0].Name != "google" || providers[0].ClientID != "g-cid" {
		t.Fatalf("unexpected providers: %+v", providers)
	}
}

func TestFetchOAuthProviders_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"oauth is not enabled"}`))
	}))
	defer ts.Close()
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	if _, err := svc.FetchOAuthProviders(); err == nil || err.Error() != "oauth is not enabled" {
		t.Fatalf("expected server error passthrough, got %v", err)
	}
}

// TestFetchOAuthProviders_EnterpriseNotConfigured 未配置企业服务端时应返回空列表（前端据此隐藏按钮）。
func TestFetchOAuthProviders_EnterpriseNotConfigured(t *testing.T) {
	t.Setenv("ENTERPRISE_API_BASE_URL", "")

	svc := &Service{}
	providers, err := svc.FetchOAuthProviders()
	if err != nil {
		t.Fatalf("FetchOAuthProviders() error: %v", err)
	}
	if len(providers) != 0 {
		t.Fatalf("expected no providers when enterprise api is not configured, got %+v", providers)
	}
}

// TestFetchOAuthProviders_UsesEnterpriseNotPlatform 第三方登录必须走企业服务端，不得回落到平台 API。
func TestFetchOAuthProviders_UsesEnterpriseNotPlatform(t *testing.T) {
	var platformHit int32
	platform := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&platformHit, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer platform.Close()

	enterprise := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"providers":[{"name":"github","client_id":"h-cid"}]}`))
	}))
	defer enterprise.Close()

	t.Setenv("PLATFORM_API_BASE_URL", platform.URL)
	t.Setenv("ENTERPRISE_API_BASE_URL", enterprise.URL)

	svc := &Service{}
	providers, err := svc.FetchOAuthProviders()
	if err != nil {
		t.Fatalf("FetchOAuthProviders() error: %v", err)
	}
	if len(providers) != 1 || providers[0].Name != "github" {
		t.Fatalf("expected providers from enterprise server, got %+v", providers)
	}
	if atomic.LoadInt32(&platformHit) != 0 {
		t.Error("oauth providers must not be fetched from the platform API")
	}
}

func TestStartOAuthLogin_Google(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 2*time.Second)

	var polls int32
	var ts *httptest.Server
	ts, authorizeBody, pollBody := startOAuthEnterpriseStub(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"session_id":"sess-google-1","authorize_url":"%s/auth/oauth/start?session_id=sess-google-1"}`, ts.URL)
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if atomic.AddInt32(&polls, 1) == 1 {
				_, _ = w.Write([]byte(`{"status":"pending"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"completed","access_token":"fake-jwt-token-123","refresh_token":"r-1","expires_in":86400}`))
		},
	)

	spy := stubOAuthBrowser(t, func(authURL *url.URL) {
		base, err := url.Parse(ts.URL)
		if err != nil {
			t.Errorf("parse stub server url: %v", err)
			return
		}
		if authURL.Host != base.Host {
			t.Errorf("browser should open the enterprise start page, got host: %s", authURL.Host)
		}
		if authURL.Path != "/auth/oauth/start" {
			t.Errorf("unexpected start page path: %s", authURL.Path)
		}
		if q := authURL.Query(); q.Get("session_id") != "sess-google-1" {
			t.Errorf("unexpected session_id in start URL: %q", q.Get("session_id"))
		}
	})
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	if err := svc.StartOAuthLogin("google"); err != nil {
		t.Fatalf("StartOAuthLogin() error: %v", err)
	}
	if !svc.IsLoggedIn() {
		t.Fatal("expected logged in after oauth flow")
	}
	if svc.GetPlatformToken() != "fake-jwt-token-123" {
		t.Fatalf("unexpected token: %q", svc.GetPlatformToken())
	}
	if spy.Saved != "fake-jwt-token-123" {
		t.Errorf("expected keychain save of %q, got %q", "fake-jwt-token-123", spy.Saved)
	}
	state := svc.GetAuthState()
	if state.UserID == nil || *state.UserID != "fake-jwt" {
		t.Errorf("unexpected user id: %v", state.UserID)
	}

	if got := authorizeBody.get()["provider"]; got != "google" {
		t.Errorf("authorize payload provider = %v, want google", got)
	}
	if got := pollBody.get()["session_id"]; got != "sess-google-1" {
		t.Errorf("poll payload session_id = %v, want sess-google-1", got)
	}
}

func TestStartOAuthLogin_GitHub(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 2*time.Second)

	var ts *httptest.Server
	ts, authorizeBody, pollBody := startOAuthEnterpriseStub(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"session_id":"sess-github-1","authorize_url":"%s/auth/oauth/start?session_id=sess-github-1"}`, ts.URL)
		},
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"completed","access_token":"gh-jwt-456","refresh_token":"r-2","expires_in":86400}`))
		},
	)

	base, _ := url.Parse(ts.URL)
	spy := stubOAuthBrowser(t, func(authURL *url.URL) {
		if authURL.Host != base.Host {
			t.Errorf("browser should open the enterprise start page, got host: %s", authURL.Host)
		}
		if authURL.Path != "/auth/oauth/start" {
			t.Errorf("unexpected start page path: %s", authURL.Path)
		}
	})
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)

	svc := &Service{}
	if err := svc.StartOAuthLogin("github"); err != nil {
		t.Fatalf("StartOAuthLogin() error: %v", err)
	}
	if svc.GetPlatformToken() != "gh-jwt-456" {
		t.Fatalf("unexpected token: %q", svc.GetPlatformToken())
	}
	if spy.Saved != "gh-jwt-456" {
		t.Errorf("expected keychain save of %q, got %q", "gh-jwt-456", spy.Saved)
	}

	if got := authorizeBody.get()["provider"]; got != "github" {
		t.Errorf("authorize payload provider = %v, want github", got)
	}
	if got := pollBody.get()["session_id"]; got != "sess-github-1" {
		t.Errorf("poll payload session_id = %v, want sess-github-1", got)
	}
}

func TestStartOAuthLogin_UnsupportedProvider(t *testing.T) {
	svc := &Service{}
	err := svc.StartOAuthLogin("microsoft")
	if err == nil || !strings.Contains(err.Error(), "unsupported oauth provider") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestStartOAuthLogin_AlreadyRunning(t *testing.T) {
	svc := &Service{}
	svc.mu.Lock()
	svc.oauthRunning = true
	svc.mu.Unlock()
	if err := svc.StartOAuthLogin("google"); err == nil || err.Error() != "oauth login already in progress" {
		t.Fatalf("expected already-in-progress error, got %v", err)
	}
}

// TestStartOAuthLogin_EnterpriseNotConfigured 未配置企业服务端时应报错，且不打开浏览器。
func TestStartOAuthLogin_EnterpriseNotConfigured(t *testing.T) {
	t.Setenv("ENTERPRISE_API_BASE_URL", "")
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	err := svc.StartOAuthLogin("google")
	if err == nil || err.Error() != "enterprise api base url is not configured" {
		t.Fatalf("expected enterprise-not-configured error, got %v", err)
	}
	if spy.Opened {
		t.Error("browser should not open when enterprise api is not configured")
	}
}

func TestStartOAuthLogin_ProviderNotConfigured(t *testing.T) {
	ts, _, _ := startOAuthEnterpriseStub(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"oauth provider google is not configured"}`))
		},
		nil,
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	err := svc.StartOAuthLogin("google")
	if err == nil || err.Error() != "oauth provider google is not configured" {
		t.Fatalf("expected server error passthrough, got %v", err)
	}
	if spy.Opened {
		t.Error("browser should not open when authorize fails")
	}
	if svc.IsLoggedIn() {
		t.Error("expected not logged in")
	}
}

func TestStartOAuthLogin_AuthorizeServerError(t *testing.T) {
	ts, _, _ := startOAuthEnterpriseStub(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"internal"}`))
		},
		nil,
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	if err := svc.StartOAuthLogin("google"); err == nil || err.Error() != "internal" {
		t.Fatalf("expected authorize server error passthrough, got %v", err)
	}
	if spy.Opened {
		t.Error("browser should not open when authorize fails")
	}
}

func TestStartOAuthLogin_AuthorizeEmptySession(t *testing.T) {
	ts, _, _ := startOAuthEnterpriseStub(t,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"session_id":"","authorize_url":""}`))
		},
		nil,
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	if err := svc.StartOAuthLogin("google"); err == nil || err.Error() != "oauth authorize returned empty session" {
		t.Fatalf("expected empty session error, got %v", err)
	}
	if spy.Opened {
		t.Error("browser should not open when authorize returns empty session")
	}
}

func TestStartOAuthLogin_PollFailed(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 2*time.Second)
	ts, _, _ := startOAuthEnterpriseStub(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"failed","error":"auth.oauthDenied"}`))
		},
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	err := svc.StartOAuthLogin("google")
	if err == nil || err.Error() != "auth.oauthDenied" {
		t.Fatalf("expected poll failure passthrough, got %v", err)
	}
	if !spy.Opened {
		t.Error("browser should have been opened before polling")
	}
	if svc.IsLoggedIn() {
		t.Error("expected not logged in after failed poll")
	}
}

func TestStartOAuthLogin_PollCompletedEmptyToken(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 2*time.Second)
	ts, _, _ := startOAuthEnterpriseStub(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"completed","access_token":""}`))
		},
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	stubOAuthBrowser(t, nil)

	svc := &Service{}
	if err := svc.StartOAuthLogin("google"); err == nil || err.Error() != "platform returned empty access token" {
		t.Fatalf("expected empty access token error, got %v", err)
	}
}

func TestStartOAuthLogin_PollServerError(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 2*time.Second)
	ts, _, _ := startOAuthEnterpriseStub(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"oauth session not found or expired"}`))
		},
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	stubOAuthBrowser(t, nil)

	svc := &Service{}
	if err := svc.StartOAuthLogin("google"); err == nil || err.Error() != "oauth session not found or expired" {
		t.Fatalf("expected poll server error passthrough, got %v", err)
	}
}

func TestStartOAuthLogin_PollTimeout(t *testing.T) {
	acceleratePolling(t, 5*time.Millisecond, 30*time.Millisecond)
	ts, _, _ := startOAuthEnterpriseStub(t, nil,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"pending"}`))
		},
	)
	t.Setenv("ENTERPRISE_API_BASE_URL", ts.URL)
	spy := stubOAuthBrowser(t, nil)

	svc := &Service{}
	err := svc.StartOAuthLogin("google")
	if err == nil || err.Error() != "oauth login timed out" {
		t.Fatalf("expected timeout error, got %v", err)
	}
	if !spy.Opened {
		t.Error("browser should have been opened before polling")
	}
	if svc.IsLoggedIn() {
		t.Error("expected not logged in after timeout")
	}
}
