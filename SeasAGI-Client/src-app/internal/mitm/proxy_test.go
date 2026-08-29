package mitm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// setupTestProxy 创建一个测试用 Proxy + 测试用 gateway 服务器。
func setupTestProxy(t *testing.T, rules *Rules) (*Proxy, *httptest.Server, func()) {
	t.Helper()

	// 创建 CA
	dir := t.TempDir()
	ca, err := NewCA(dir)
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	if err := ca.EnsureCA(); err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	// 创建测试 gateway（返回固定响应）
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("gateway-response"))
	}))

	// 创建 Proxy，使用随机端口
	proxy := NewProxy("127.0.0.1:0", ca, rules, gateway.URL, nil)

	cleanup := func() {
		_ = proxy.Stop()
		gateway.Close()
	}
	return proxy, gateway, cleanup
}

func TestProxyStartStopHealth(t *testing.T) {
	rules := NewRules()
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	if proxy.IsHealthy() {
		t.Error("should not be healthy before Start")
	}

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 等待一小段时间让服务器完全就绪
	time.Sleep(50 * time.Millisecond)

	if !proxy.IsHealthy() {
		t.Error("should be healthy after Start")
	}

	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if proxy.IsHealthy() {
		t.Error("should not be healthy after Stop")
	}
}

func TestProxyStopIdempotent(t *testing.T) {
	rules := NewRules()
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	_ = proxy.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// 连续 Stop 两次不应 panic
	if err := proxy.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := proxy.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestProxyStartDoubleError(t *testing.T) {
	rules := NewRules()
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 第二次 Start 应报错
	if err := proxy.Start(ctx); err == nil {
		t.Error("second Start should return error")
	}
}

func TestHTTPPassthrough(t *testing.T) {
	// 创建一个目标服务器
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("target-ok"))
	}))
	defer target.Close()

	targetURL, _ := url.Parse(target.URL)
	targetHost := targetURL.Host

	rules := NewRules() // 空规则，不拦截任何域名
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// 通过 proxy 发送 HTTP 请求到目标
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxy.listener.Addr().String())
			},
		},
	}

	resp, err := client.Get("http://" + targetHost + "/test")
	if err != nil {
		t.Fatalf("Get through proxy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "target-ok" {
		t.Errorf("body: got %q, want %q", string(body), "target-ok")
	}
}

func TestHTTPSIntercept(t *testing.T) {
	rules := NewRules()
	rules.Add("api.test.com")

	proxy, gateway, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	proxyAddr := proxy.listener.Addr().String()

	// 创建 TLS 客户端，信任我们的 CA
	caPEM, _ := proxy.ca.PEMBytes()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxyAddr)
			},
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
	}

	resp, err := client.Get("https://api.test.com/v1/models")
	if err != nil {
		t.Fatalf("Get through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "gateway-response" {
		t.Errorf("body: got %q, want %q", string(body), "gateway-response")
	}

	_ = gateway
}

func TestHTTPSPassthrough(t *testing.T) {
	// 创建一个真实的 HTTPS 目标服务器
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("https-passthrough-ok"))
	}))
	defer target.Close()

	targetURL, _ := url.Parse(target.URL)
	targetHost := targetURL.Host

	rules := NewRules() // 空规则，不拦截
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	proxyAddr := proxy.listener.Addr().String()

	// 客户端信任目标服务器的自签证书
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxyAddr)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get("https://" + targetHost + "/test")
	if err != nil {
		t.Fatalf("Get through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "https-passthrough-ok" {
		t.Errorf("body: got %q, want %q", string(body), "https-passthrough-ok")
	}
}

func TestNonAIDomainPassthrough(t *testing.T) {
	// 创建一个目标 HTTP 服务器
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("non-ai-ok"))
	}))
	defer target.Close()

	targetURL, _ := url.Parse(target.URL)
	targetHost := targetURL.Host

	// 规则只包含 AI 域名，不包含目标域名
	rules := NewDefaultRules()
	proxy, _, cleanup := setupTestProxy(t, rules)
	defer cleanup()

	ctx := context.Background()
	if err := proxy.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse("http://" + proxy.listener.Addr().String())
			},
		},
	}

	resp, err := client.Get("http://" + targetHost + "/test")
	if err != nil {
		t.Fatalf("Get through proxy: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "non-ai-ok" {
		t.Errorf("body: got %q, want %q", string(body), "non-ai-ok")
	}
}
