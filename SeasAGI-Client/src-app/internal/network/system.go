package network

import (
	"context"
	"sync"
	"time"
)

var (
	networkStateMu sync.RWMutex
	lastNetworkID  string
	onWakeCB       func()
	onNetworkCB    func()
)

func OnWake(cb func()) {
	networkStateMu.Lock()
	defer networkStateMu.Unlock()
	onWakeCB = cb
}

func OnNetworkChange(cb func()) {
	networkStateMu.Lock()
	defer networkStateMu.Unlock()
	onNetworkCB = cb
}

func WatchSystemEvents(ctx context.Context) {
	go watchSleepWake(ctx)
	go watchNetworkChange(ctx)
}

func watchSleepWake(ctx context.Context) {
	wasAwake := true
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		isAwake := isSystemAwake()
		if wasAwake && !isAwake {
			wasAwake = false
		} else if isAwake && !wasAwake {
			wasAwake = true
			networkStateMu.RLock()
			cb := onWakeCB
			networkStateMu.RUnlock()
			if cb != nil {
				go cb()
			}
		}
	}
}

func watchNetworkChange(ctx context.Context) {
	lastNetworkID = captureNetworkID()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		currentID := captureNetworkID()
		networkStateMu.RLock()
		prevID := lastNetworkID
		networkStateMu.RUnlock()

		if currentID != "" && currentID != prevID {
			networkStateMu.Lock()
			lastNetworkID = currentID
			cb := onNetworkCB
			networkStateMu.Unlock()
			if cb != nil {
				go cb()
			}
		}
	}
}
