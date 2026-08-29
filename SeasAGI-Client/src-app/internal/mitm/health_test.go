package mitm

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthProbeHealthy(t *testing.T) {
	proxy := NewProxy(":0", nil, nil, "http://127.0.0.1:9999", nil)
	if err := proxy.Start(context.Background()); err != nil {
		t.Fatalf("start proxy: %v", err)
	}
	defer proxy.Stop()

	var triggered int32
	probe := NewHealthProbe(proxy, 50*time.Millisecond, func() {
		atomic.StoreInt32(&triggered, 1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	probe.Run(ctx)

	if atomic.LoadInt32(&triggered) != 0 {
		t.Error("health probe should not trigger on healthy proxy")
	}
}

func TestHealthProbeUnhealthy(t *testing.T) {
	// 创建一个未启动的 proxy，IsHealthy 返回 false
	proxy := NewProxy(":0", nil, nil, "http://127.0.0.1:9999", nil)

	var triggered int32
	probe := NewHealthProbe(proxy, 50*time.Millisecond, func() {
		atomic.StoreInt32(&triggered, 1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	probe.Run(ctx)

	if atomic.LoadInt32(&triggered) != 1 {
		t.Error("health probe should trigger on unhealthy proxy")
	}
}
