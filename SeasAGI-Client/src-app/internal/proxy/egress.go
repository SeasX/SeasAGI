package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// EgressManager 管理出口 IP 探测和同组共享检测。
type EgressManager struct {
	mu          sync.RWMutex
	cache       map[string]*egressEntry // proxyAddr → entry
	cacheTTL    time.Duration
	echoURL     string
	httpClient  *http.Client
	rotationMap map[string]string // providerName → rotationGroup
}

type egressEntry struct {
	IP        string
	ProxyAddr string
	CheckedAt time.Time
}

// NewEgressManager 创建出口 IP 管理器。
func NewEgressManager(echoURL string, cacheTTL time.Duration) *EgressManager {
	if echoURL == "" {
		echoURL = "https://api64.ipify.org"
	}
	return &EgressManager{
		cache:       make(map[string]*egressEntry),
		cacheTTL:    cacheTTL,
		echoURL:     echoURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		rotationMap: make(map[string]string),
	}
}

// SetRotationGroup 设置 provider 的 rotation group。
// 同一 rotation group 的 provider 共享认证体系，不应共享出口 IP。
func (m *EgressManager) SetRotationGroup(providerName, group string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rotationMap[providerName] = group
}

// DetectEgressIP 探测指定代理的出口 IP。
// proxyAddr 为空表示直连。
func (m *EgressManager) DetectEgressIP(proxyAddr string) (string, error) {
	// 检查缓存
	m.mu.RLock()
	if entry, ok := m.cache[proxyAddr]; ok && time.Since(entry.CheckedAt) < m.cacheTTL {
		m.mu.RUnlock()
		return entry.IP, nil
	}
	m.mu.RUnlock()

	// 通过指定代理探测
	client := m.httpClient
	if proxyAddr != "" {
		u, err := url.Parse(proxyAddr)
		if err != nil {
			return "", fmt.Errorf("parse proxy addr: %w", err)
		}
		transport := &http.Transport{
			Proxy: http.ProxyURL(u),
		}
		client = &http.Client{Timeout: 10 * time.Second, Transport: transport}
	}

	resp, err := client.Get(m.echoURL)
	if err != nil {
		return "", fmt.Errorf("egress detection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("echo service returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read echo response: %w", err)
	}

	ip := string(body)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("invalid IP from echo: %s", ip)
	}

	// 更新缓存
	m.mu.Lock()
	m.cache[proxyAddr] = &egressEntry{IP: ip, ProxyAddr: proxyAddr, CheckedAt: time.Now()}
	m.mu.Unlock()

	return ip, nil
}

// EgressSharingResult 表示同组 IP 共享检测结果。
type EgressSharingResult struct {
	Group     string
	Providers []string
	ProxyIPs  map[string]string // providerName → egressIP
	Shared    bool              // 是否存在共享
	SharedIPs []string          // 被共享的 IP
}

// AnalyzeEgressSharing 分析同一 rotation group 内的出口 IP 共享情况。
// proxyMap: providerName → proxyAddr
func (m *EgressManager) AnalyzeEgressSharing(proxyMap map[string]string) []EgressSharingResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 按 group 分组
	groups := make(map[string][]string)
	for provider, group := range m.rotationMap {
		groups[group] = append(groups[group], provider)
	}

	var results []EgressSharingResult
	for group, providers := range groups {
		if len(providers) < 2 {
			continue
		}

		proxyIPs := make(map[string]string)
		ipToProviders := make(map[string][]string)

		for _, provider := range providers {
			proxyAddr := proxyMap[provider]
			if entry, ok := m.cache[proxyAddr]; ok {
				proxyIPs[provider] = entry.IP
				ipToProviders[entry.IP] = append(ipToProviders[entry.IP], provider)
			}
		}

		shared := false
		var sharedIPs []string
		for ip, provs := range ipToProviders {
			if len(provs) > 1 {
				shared = true
				sharedIPs = append(sharedIPs, ip)
			}
		}

		if shared {
			results = append(results, EgressSharingResult{
				Group:     group,
				Providers: providers,
				ProxyIPs:  proxyIPs,
				Shared:    true,
				SharedIPs: sharedIPs,
			})
		}
	}

	return results
}

// WarmEgressIP 预热出口 IP 缓存（fire-and-forget）。
func (m *EgressManager) WarmEgressIP(proxyAddr string) {
	go func() {
		_, _ = m.DetectEgressIP(proxyAddr)
	}()
}

// ClearCache 清除所有缓存。
func (m *EgressManager) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]*egressEntry)
}

// CacheStats 返回缓存统计。
func (m *EgressManager) CacheStats() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.cache)
}
