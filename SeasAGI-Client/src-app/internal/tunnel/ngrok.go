package tunnel

import (
	"fmt"
	"strings"
	"sync"
)

// TunnelPhase 隧道状态。
type TunnelPhase string

const (
	PhaseUnsupported TunnelPhase = "unsupported"
	PhaseNotInstalled TunnelPhase = "not_installed"
	PhaseStopped      TunnelPhase = "stopped"
	PhaseNeedsAuth    TunnelPhase = "needs_auth"
	PhaseStarting     TunnelPhase = "starting"
	PhaseRunning      TunnelPhase = "running"
	PhaseError        TunnelPhase = "error"
)

// NgrokTunnelStatus ngrok 隧道状态。
type NgrokTunnelStatus struct {
	Phase    TunnelPhase
	PublicURL string
	APIURL    string
	Error     string
}

// NgrokTunnelManager ngrok 隧道管理器。
type NgrokTunnelManager struct {
	mu      sync.Mutex
	status  NgrokTunnelStatus
	running bool
}

// NewNgrokTunnelManager 创建 ngrok 隧道管理器。
func NewNgrokTunnelManager() *NgrokTunnelManager {
	return &NgrokTunnelManager{
		status: NgrokTunnelStatus{Phase: PhaseStopped},
	}
}

// Start 启动 ngrok 隧道。
// addr 是本地监听地址（如 "8080"），authtoken 是 ngrok authtoken。
func (m *NgrokTunnelManager) Start(addr, authtoken string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("tunnel already running")
	}

	if authtoken == "" {
		m.status = NgrokTunnelStatus{Phase: PhaseNeedsAuth, Error: "authtoken required"}
		return fmt.Errorf("ngrok authtoken required")
	}

	// 模拟启动过程（实际实现需调用 ngrok SDK 或 CLI）
	m.status = NgrokTunnelStatus{
		Phase:    PhaseRunning,
		PublicURL: fmt.Sprintf("https://random-tunnel.ngrok.io"),
		APIURL:    fmt.Sprintf("http://127.0.0.1:4040"),
	}
	m.running = true
	return nil
}

// Stop 停止 ngrok 隧道。
func (m *NgrokTunnelManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	m.status = NgrokTunnelStatus{Phase: PhaseStopped}
	m.running = false
	return nil
}

// GetStatus 返回当前隧道状态。
func (m *NgrokTunnelManager) GetStatus() NgrokTunnelStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// IsRunning 检查隧道是否正在运行。
func (m *NgrokTunnelManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// ParseTunnelURL 从隧道状态中解析公共 URL。
func ParseTunnelURL(status NgrokTunnelStatus) string {
	if status.Phase != PhaseRunning {
		return ""
	}
	return strings.TrimPrefix(status.PublicURL, "https://")
}
