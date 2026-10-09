package mitm

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

const (
	// defaultProxyHost 系统代理指向的本地地址（MITM 代理监听在所有网卡上）。
	defaultProxyHost = "127.0.0.1"
	// defaultProxyPort MITM 代理监听端口，同时也是写入系统代理的端口。
	defaultProxyPort = 8080
)

// Manager 编排 MITM 生命周期的启动和停止，确保失败时按逆序回滚。
type Manager struct {
	mu           sync.Mutex
	state        State
	ca           *CertificateAuthority
	proxy        *Proxy
	rules        *Rules
	trust        TrustInstaller    // Phase A2 注入，A1 为 nil
	sysProxy     SystemProxySetter // Phase A2 注入，A1 为 nil
	status       Status
	proxyAddr    string
	interceptLog InterceptLogger
	healthProbe  *HealthProbe
	healthCancel context.CancelFunc
}

// isOwnProxyAddr 判断 current 是否指向本客户端的 MITM 代理。
// 用于区分「SeasAGI 自身写入的系统代理」与「用户自有的代理配置」，避免误清后者。
func isOwnProxyAddr(current string) bool {
	current = strings.TrimSpace(current)
	if current == "" {
		return false
	}
	host, port, err := net.SplitHostPort(current)
	if err != nil {
		return false
	}
	if port != strconv.Itoa(defaultProxyPort) {
		return false
	}
	switch strings.ToLower(host) {
	case "", "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// NewManager 创建 Manager 实例。gatewayURL 为本地网关地址（如 http://127.0.0.1:4318）。
func NewManager(ca *CertificateAuthority, rules *Rules, gatewayURL string) *Manager {
	interceptLog := NewInterceptLogger(200)
	addr := fmt.Sprintf(":%d", defaultProxyPort)
	return &Manager{
		state:        StateStopped,
		ca:           ca,
		rules:        rules,
		proxyAddr:    addr,
		proxy:        NewProxy(addr, ca, rules, gatewayURL, interceptLog),
		interceptLog: interceptLog,
		status: Status{
			State:      StateStopped,
			ProxyPort:  defaultProxyPort,
			RulesCount: rules.Count(),
		},
	}
}

// Start 按 StartupStep 顺序逐步启动。任一步失败时逆序回滚已完成步骤，
// 并返回具体错误，供上层（前端）感知真实失败原因。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 幂等：已 running 直接返回
	if m.state == StateRunning {
		return nil
	}

	// 刷新基线域名（厂商 API + 受支持目标，含企业动态下发），
	// 再载入用户自定义增删。两者合并后即为本次运行的拦截清单。
	m.rules.SetBase(DefaultBaseDomains())
	if err := m.rules.Load(); err != nil {
		logging.Warningf("mitm: load persisted rules failed: %v", err)
	}

	// 残留系统代理清理（崩溃恢复场景）：
	// 仅当当前系统代理确实指向本客户端时才清除，避免误清用户自有的代理配置。
	if m.sysProxy != nil {
		if active, _ := m.sysProxy.IsActive(); active {
			cur, cerr := m.sysProxy.CurrentAddr()
			if cerr == nil && isOwnProxyAddr(cur) {
				logging.Infof("mitm: detected residual SeasAGI system proxy (%s), clearing before start", cur)
				_ = m.sysProxy.Clear()
			} else {
				logging.Warningf("mitm: system proxy is active but not owned by SeasAGI (%s), leaving it untouched", cur)
			}
		}
	}

	m.state = StateStarting
	m.status.State = StateStarting
	m.status.LastError = ""

	// Step 1: CA
	if m.ca == nil {
		werr := fmt.Errorf("CA not initialized")
		m.failWith(werr)
		return werr
	}
	if err := m.ca.EnsureCA(); err != nil {
		m.rollback([]string{"ca"})
		werr := fmt.Errorf("CA init: %w", err)
		m.failWith(werr)
		return werr
	}
	// CA 已在本地生成（不代表已被系统信任）。
	m.status.CAInstalled = true
	m.status.CATrusted = false

	// Step 2: Trust（如果有注入）
	if m.trust != nil {
		caPEM, err := m.ca.PEMBytes()
		if err != nil {
			m.rollback([]string{"ca"})
			werr := fmt.Errorf("export CA PEM: %w", err)
			m.failWith(werr)
			return werr
		}
		if err := m.trust.Install(caPEM); err != nil {
			m.rollback([]string{"ca"})
			werr := fmt.Errorf("install CA trust: %w", err)
			m.failWith(werr)
			return werr
		}
		if ok, err := m.trust.IsInstalled(); err == nil && !ok {
			m.rollback([]string{"ca"})
			werr := fmt.Errorf("CA trust installed but not verifiable")
			m.failWith(werr)
			return werr
		}
		m.status.CATrusted = true
	}

	// Step 3: Proxy
	if err := m.proxy.Start(ctx); err != nil {
		m.rollback([]string{"trust", "ca"})
		werr := fmt.Errorf("start proxy: %w", err)
		m.failWith(werr)
		return werr
	}

	// Step 4: SystemProxy（如果有注入）
	if m.sysProxy != nil {
		if !m.proxy.IsHealthy() {
			m.rollback([]string{"proxy", "trust", "ca"})
			werr := fmt.Errorf("proxy not healthy after start")
			m.failWith(werr)
			return werr
		}
		if err := m.sysProxy.Set(fmt.Sprintf("%s:%d", defaultProxyHost, defaultProxyPort)); err != nil {
			m.rollback([]string{"proxy", "trust", "ca"})
			werr := fmt.Errorf("set system proxy: %w", err)
			m.failWith(werr)
			return werr
		}
		m.status.SystemProxy = true
		m.status.SystemProxyOwned = true
	}

	m.state = StateRunning
	m.status.State = StateRunning
	m.status.RulesCount = m.rules.Count()

	// 启动健康探针 goroutine
	healthCtx, healthCancel := context.WithCancel(context.Background())
	m.healthCancel = healthCancel
	m.healthProbe = NewHealthProbe(m.proxy, 10*time.Second, func() {
		logging.Warning("mitm: proxy unhealthy, auto-stopping")
		_ = m.Stop()
	})
	go m.healthProbe.Run(healthCtx)

	return nil
}

// Stop 逆序停止所有组件。幂等。
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateStopped {
		return nil
	}

	m.state = StateStopping
	m.status.State = StateStopping

	// 停止健康探针
	if m.healthCancel != nil {
		m.healthCancel()
		m.healthCancel = nil
	}
	m.healthProbe = nil

	// 逆序停止
	// 仅清除指向本客户端的系统代理，避免把用户后来手动配置的代理一并清掉。
	if m.sysProxy != nil {
		cur, cerr := m.sysProxy.CurrentAddr()
		if cerr == nil && isOwnProxyAddr(cur) {
			_ = m.sysProxy.Clear()
		} else {
			logging.Warningf("mitm: system proxy not owned by SeasAGI (%s), not clearing on stop", cur)
		}
		m.status.SystemProxy = false
		m.status.SystemProxyOwned = false
	}

	if m.proxy != nil {
		_ = m.proxy.Stop()
	}

	// CA trust 保留（不卸载，避免反复安装/卸载）
	m.ca.Cleanup()

	m.state = StateStopped
	m.status.State = StateStopped
	return nil
}

// GetStatus 返回当前运行时状态快照。
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// GetRules 返回当前拦截域名列表。
func (m *Manager) GetRules() []string {
	return m.rules.List()
}

// AddRule 动态新增拦截域名，并持久化用户改动。
func (m *Manager) AddRule(domain string) error {
	m.mu.Lock()
	m.rules.Add(domain)
	m.status.RulesCount = m.rules.Count()
	m.mu.Unlock()
	if err := m.rules.Save(); err != nil {
		logging.Warningf("mitm: persist rules failed: %v", err)
		return err
	}
	return nil
}

// RemoveRule 动态移除拦截域名，并持久化用户改动。
func (m *Manager) RemoveRule(domain string) error {
	m.mu.Lock()
	m.rules.Remove(domain)
	m.status.RulesCount = m.rules.Count()
	m.mu.Unlock()
	if err := m.rules.Save(); err != nil {
		logging.Warningf("mitm: persist rules failed: %v", err)
		return err
	}
	return nil
}

// SetTrustInstaller 注入 CA 信任安装器。
func (m *Manager) SetTrustInstaller(ti TrustInstaller) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trust = ti
}

// SetAccessToken 设置拦截转发时注入的本地网关访问令牌。
func (m *Manager) SetAccessToken(token string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proxy != nil {
		m.proxy.SetAccessToken(token)
	}
}

// SetSystemProxySetter 注入系统代理设置器。
func (m *Manager) SetSystemProxySetter(sps SystemProxySetter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sysProxy = sps
}

// GetRecentIntercepts 返回最近 n 条拦截日志。
func (m *Manager) GetRecentIntercepts(n int) []InterceptEntry {
	if m.interceptLog == nil {
		return nil
	}
	return m.interceptLog.Recent(n)
}

// CertPool 返回 MITM CA 的证书池，用于客户端验证 MITM 签发的叶子证书。
func (m *Manager) CertPool() *x509.CertPool {
	if m.ca == nil {
		return nil
	}
	return m.ca.CertPool()
}

// failWith 记录错误并设置状态为 error。
func (m *Manager) failWith(err error) {
	m.state = StateError
	m.status.State = StateError
	m.status.LastError = err.Error()
}

// rollback 逆序回滚指定步骤（从后往前）。
func (m *Manager) rollback(steps []string) {
	for i := len(steps) - 1; i >= 0; i-- {
		switch steps[i] {
		case "system_proxy":
			if m.sysProxy != nil {
				_ = m.sysProxy.Clear()
				m.status.SystemProxy = false
			}
		case "proxy":
			if m.proxy != nil {
				_ = m.proxy.Stop()
			}
		case "trust":
			if m.trust != nil {
				_ = m.trust.Uninstall()
			}
		case "ca":
			m.ca.Cleanup()
			m.status.CAInstalled = false
		}
	}
}
