package proxy

import (
	"net"
	"sync"
	"time"
)

// HealthStatus 表示代理健康状态。
type HealthStatus struct {
	Addr      string
	Reachable bool
	CheckedAt time.Time
}

// HealthChecker 通过 TCP 快速失败探测代理可达性。
type HealthChecker struct {
	mu             sync.Mutex
	cache          map[string]*HealthStatus
	healthyTTL     time.Duration
	unhealthyTTL   time.Duration
	timeout        time.Duration
	inflight       map[string]bool // 去重并发探测
}

// NewHealthChecker 创建健康检查器。
func NewHealthChecker(timeout, healthyTTL, unhealthyTTL time.Duration) *HealthChecker {
	return &HealthChecker{
		cache:        make(map[string]*HealthStatus),
		healthyTTL:   healthyTTL,
		unhealthyTTL: unhealthyTTL,
		timeout:      timeout,
		inflight:     make(map[string]bool),
	}
}

// IsReachable 通过 TCP 连接探测代理是否可达。
func (h *HealthChecker) IsReachable(addr string) bool {
	// 检查缓存
	h.mu.Lock()
	if status, ok := h.cache[addr]; ok {
		ttl := h.healthyTTL
		if !status.Reachable {
			ttl = h.unhealthyTTL
		}
		if time.Since(status.CheckedAt) < ttl {
			h.mu.Unlock()
			return status.Reachable
		}
	}
	// 去重并发探测
	if h.inflight[addr] {
		h.mu.Unlock()
		// 等待已有探测完成，返回缓存或 false
		if status, ok := h.cache[addr]; ok {
			h.mu.Unlock()
			return status.Reachable
		}
		h.mu.Unlock()
		return false
	}
	h.inflight[addr] = true
	h.mu.Unlock()

	// TCP 探测
	reachable := h.tcpProbe(addr)

	h.mu.Lock()
	h.cache[addr] = &HealthStatus{Addr: addr, Reachable: reachable, CheckedAt: time.Now()}
	delete(h.inflight, addr)
	h.mu.Unlock()

	return reachable
}

// tcpProbe 执行 TCP 连接探测。
func (h *HealthChecker) tcpProbe(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, h.timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// GetAllStatuses 返回所有已缓存的健康状态。
func (h *HealthChecker) GetAllStatuses() []HealthStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	statuses := make([]HealthStatus, 0, len(h.cache))
	for _, s := range h.cache {
		statuses = append(statuses, *s)
	}
	return statuses
}

// ClearCache 清除健康缓存。
func (h *HealthChecker) ClearCache() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache = make(map[string]*HealthStatus)
}
