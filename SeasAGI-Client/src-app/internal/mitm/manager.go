package mitm

import (
	"context"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
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

// NewManager 创建 Manager 实例。gatewayURL 为本地网关地址（如 http://127.0.0.1:4318）。
func NewManager(ca *CertificateAuthority, rules *Rules, gatewayURL string) *Manager {
	interceptLog := NewInterceptLogger(200)
	return &Manager{
		state:        StateStopped,
		ca:           ca,
		rules:        rules,
		proxyAddr:    ":8080",
		proxy:        NewProxy(":8080", ca, rules, gatewayURL, interceptLog),
		interceptLog: interceptLog,
		status: Status{
			State:      StateStopped,
			ProxyPort:  8080,
			RulesCount: rules.Count(),
		},
	}
}

// Start 按 StartupStep 顺序逐步启动。任一步失败时逆序回滚已完成步骤。
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 幂等：已 running 直接返回
	if m.state == StateRunning {
		return nil
	}

	// 残留系统代理清理（崩溃恢复场景）
	if m.sysProxy != nil {
		if active, _ := m.sysProxy.IsActive(); active {
			logging.Info("mitm: detected residual system proxy, clearing before start")
			_ = m.sysProxy.Clear()
		}
	}

	m.state = StateStarting
	m.status.State = StateStarting
	m.status.LastError = ""

	// Step 1: CA
	if err := m.ca.EnsureCA(); err != nil {
		m.rollback([]string{"ca"})
		m.failWith(fmt.Errorf("CA init: %w", err))
		return nil
	}
	m.status.CAInstalled = true

	// Step 2: Trust（如果有注入）
	if m.trust != nil {
		caPEM, err := m.ca.PEMBytes()
		if err != nil {
			m.rollback([]string{"ca"})
			m.failWith(fmt.Errorf("export CA PEM: %w", err))
			return nil
		}
		if err := m.trust.Install(caPEM); err != nil {
			m.rollback([]string{"ca"})
			m.failWith(fmt.Errorf("install CA trust: %w", err))
			return nil
		}
		m.status.CAInstalled = true
	}

	// Step 3: Proxy
	if err := m.proxy.Start(ctx); err != nil {
		m.rollback([]string{"trust", "ca"})
		m.failWith(fmt.Errorf("start proxy: %w", err))
		return nil
	}

	// Step 4: SystemProxy（如果有注入）
	if m.sysProxy != nil {
		if !m.proxy.IsHealthy() {
			m.rollback([]string{"proxy", "trust", "ca"})
			m.failWith(fmt.Errorf("proxy not healthy after start"))
			return nil
		}
		if err := m.sysProxy.Set("127.0.0.1:8080"); err != nil {
			m.rollback([]string{"proxy", "trust", "ca"})
			m.failWith(fmt.Errorf("set system proxy: %w", err))
			return nil
		}
		m.status.SystemProxy = true
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
	if m.sysProxy != nil {
		_ = m.sysProxy.Clear()
		m.status.SystemProxy = false
	}

	if m.proxy != nil {
		_ = m.proxy.Stop()
	}

	// CA trust 保留（不卸载，避免反复安装/卸载）

	m.ca.Cleanup()

	m.state = StateStopped
	m.status.State = StateStopped
	m.status.CAInstalled = false
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

// AddRule 动态新增拦截域名。
func (m *Manager) AddRule(domain string) error {
	m.mu.Lock()
	m.rules.Add(domain)
	m.status.RulesCount = m.rules.Count()
	m.mu.Unlock()
	return nil
}

// RemoveRule 动态移除拦截域名。
func (m *Manager) RemoveRule(domain string) error {
	m.mu.Lock()
	m.rules.Remove(domain)
	m.status.RulesCount = m.rules.Count()
	m.mu.Unlock()
	return nil
}

// SetTrustInstaller 注入 CA 信任安装器。
func (m *Manager) SetTrustInstaller(ti TrustInstaller) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.trust = ti
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
