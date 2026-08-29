package mitm

import (
	"context"
	"time"
)

// HealthProbe 定期检查 Proxy 是否健康，不健康时触发回调。
type HealthProbe struct {
	proxy       *Proxy
	interval    time.Duration
	onUnhealthy func()
}

// NewHealthProbe 创建健康探针。interval 为检查间隔，onUnhealthy 为不健康时回调。
func NewHealthProbe(proxy *Proxy, interval time.Duration, onUnhealthy func()) *HealthProbe {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &HealthProbe{
		proxy:       proxy,
		interval:    interval,
		onUnhealthy: onUnhealthy,
	}
}

// Run 在 goroutine 中运行健康探针，直到 ctx 被取消。
func (h *HealthProbe) Run(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !h.proxy.IsHealthy() {
				if h.onUnhealthy != nil {
					h.onUnhealthy()
				}
				return
			}
		}
	}
}
