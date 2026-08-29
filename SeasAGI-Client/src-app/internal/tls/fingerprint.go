package tls

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// FingerprintProfile 表示一种 TLS 指纹伪装配置。
type FingerprintProfile struct {
	Name       string // "chrome_124" / "firefox_120"
	ClientSpec ClientHelloSpec
}

// ClientHelloSpec 描述 ClientHello 指纹参数（简化模型，可对接 utls）。
type ClientHelloSpec struct {
	Browser      string
	OS           string
	MinVersion   uint16
	MaxVersion   uint16
	CipherSuites []uint16
}

// 预置 profile — Chrome 124
var Chrome124 = FingerprintProfile{
	Name: "chrome_124",
	ClientSpec: ClientHelloSpec{
		Browser:    "chrome",
		OS:         "macos",
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
	},
}

// 预置 profile — Firefox 120
var Firefox120 = FingerprintProfile{
	Name: "firefox_120",
	ClientSpec: ClientHelloSpec{
		Browser:    "firefox",
		OS:         "macos",
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
	},
}

// FingerprintManager 管理 TLS 指纹伪装的 profile 选择和 HTTP client 创建。
type FingerprintManager struct {
	mu       sync.RWMutex
	profile  FingerprintProfile
	breaker  *CircuitBreaker
	proxyURL string // 可选的代理地址
}

// NewFingerprintManager 创建指定 profile 的管理器。
func NewFingerprintManager(profile FingerprintProfile) *FingerprintManager {
	return &FingerprintManager{
		profile: profile,
		breaker: NewCircuitBreaker(3, 30*time.Second, 600*time.Second),
	}
}

// SetProfile 切换 TLS 指纹 profile。
func (m *FingerprintManager) SetProfile(profile FingerprintProfile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.profile = profile
}

// GetProfile 返回当前 profile。
func (m *FingerprintManager) GetProfile() FingerprintProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.profile
}

// SetProxy 设置可选的代理地址。
func (m *FingerprintManager) SetProxy(proxyURL string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxyURL = proxyURL
}

// NewHTTPClient 创建一个使用当前 profile 的 *http.Client。
// 如果熔断器开路则返回错误。
func (m *FingerprintManager) NewHTTPClient(timeout time.Duration) (*http.Client, error) {
	if !m.breaker.Allow() {
		return nil, fmt.Errorf("circuit breaker open: %s", m.breaker.State())
	}

	m.mu.RLock()
	profile := m.profile
	proxyURL := m.proxyURL
	m.mu.RUnlock()

	tlsConfig := &tls.Config{
		MinVersion:         profile.ClientSpec.MinVersion,
		MaxVersion:         profile.ClientSpec.MaxVersion,
		CipherSuites:       profile.ClientSpec.CipherSuites,
		InsecureSkipVerify: false,
	}

	transport := &http.Transport{
		TLSClientConfig:    tlsConfig,
		ForceAttemptHTTP2:  true,
		DisableCompression: false,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

// RecordSuccess 记录一次成功请求（重置熔断器）。
func (m *FingerprintManager) RecordSuccess() {
	m.breaker.RecordSuccess()
}

// RecordFailure 记录一次失败请求（可能触发熔断）。
func (m *FingerprintManager) RecordFailure() {
	m.breaker.RecordFailure()
}

// BreakerState 返回熔断器状态描述。
func (m *FingerprintManager) BreakerState() string {
	return m.breaker.State()
}
