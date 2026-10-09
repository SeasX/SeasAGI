package mitm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestHTTPSInterceptInjectsLocalToken 端到端验证 MITM 拦截链路的关键修复：
// 被接管的 SDK 携带的是厂商自己的 key，代理必须在转发前改写为本地网关令牌，
// 否则网关校验收不到合法令牌必然 401（此为原缺陷 1）。
func TestHTTPSInterceptInjectsLocalToken(t *testing.T) {
	const localToken = "local-gateway-token"

	var gotAuth, gotAPIKey string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("x-api-key")
		if gotAuth != "Bearer "+localToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway-ok"))
	}))
	defer gateway.Close()

	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	rules := NewRules()
	rules.Add("api.test.com")

	proxy := NewProxy("127.0.0.1:0", ca, rules, gateway.URL, nil)
	proxy.SetAccessToken(localToken)
	defer func() { _ = proxy.Stop() }()

	if err := proxy.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	caPEM, _ := ca.PEMBytes()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxy.listener.Addr().String())
			},
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	req, err := http.NewRequest(http.MethodPost, "https://api.test.com/v1/messages", strings.NewReader(`{"model":"x"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// 模拟被接管的 Claude Code：携带厂商自己的 key
	req.Header.Set("Authorization", "Bearer vendor-key-must-be-overwritten")
	req.Header.Set("x-api-key", "vendor-anthropic-key")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body = %q, want 200 (auth injection failed)", resp.StatusCode, string(body))
	}
	if string(body) != "gateway-ok" {
		t.Fatalf("body = %q, want gateway-ok", string(body))
	}
	if gotAuth != "Bearer "+localToken {
		t.Fatalf("gateway received Authorization %q, want local token", gotAuth)
	}
	if gotAPIKey != "" {
		t.Fatalf("x-api-key should be cleared before forwarding, got %q", gotAPIKey)
	}
}

// TestHTTPSInterceptWithoutTokenPassesVendorAuth 回归保护：未设置本机令牌时，
// 代理不应改写鉴权头（保持原有透传语义）。
func TestHTTPSInterceptWithoutTokenPassesVendorAuth(t *testing.T) {
	var gotAuth string
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer gateway.Close()

	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	rules := NewRules()
	rules.Add("api.test.com")

	proxy := NewProxy("127.0.0.1:0", ca, rules, gateway.URL, nil)
	// 不调用 SetAccessToken
	defer func() { _ = proxy.Stop() }()

	if err := proxy.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	caPEM, _ := ca.PEMBytes()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxy.listener.Addr().String())
			},
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	req, _ := http.NewRequest(http.MethodGet, "https://api.test.com/v1/models", nil)
	req.Header.Set("Authorization", "Bearer vendor-key")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do through proxy: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer vendor-key" {
		t.Fatalf("Authorization without local token should pass through, got %q", gotAuth)
	}
}
