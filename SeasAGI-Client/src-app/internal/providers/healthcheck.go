package providers

import (
	"context"
	"sync"
	"time"
)

type channelHealthState struct {
	consecutiveFailures int
	lastResult          string
	lastCheck           time.Time
}

type HealthChecker struct {
	mu            sync.Mutex
	listFn        func() []HealthChannelInfo
	updateFn      func(channelID, health string) error
	states        map[string]*channelHealthState
	maxFailures   int
	checkInterval time.Duration
	stopCh        chan struct{}
}

type HealthChannelInfo struct {
	ChannelID    string
	ChannelType  string
	ProviderType string
	BaseURL      string
	APIKey       string
	Enabled      bool
	HealthStatus string
}

func NewHealthChecker(listFn func() []HealthChannelInfo, updateFn func(channelID, health string) error) *HealthChecker {
	return &HealthChecker{
		listFn:        listFn,
		updateFn:      updateFn,
		states:        make(map[string]*channelHealthState),
		maxFailures:   3,
		checkInterval: 5 * time.Minute,
		stopCh:        make(chan struct{}),
	}
}

func (hc *HealthChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(hc.checkInterval)
	defer ticker.Stop()

	hc.runOnce()

	for {
		select {
		case <-ticker.C:
			hc.runOnce()
		case <-ctx.Done():
			return
		case <-hc.stopCh:
			return
		}
	}
}

func (hc *HealthChecker) Stop() {
	close(hc.stopCh)
}

func (hc *HealthChecker) runOnce() {
	channels := hc.listFn()
	if len(channels) == 0 {
		return
	}

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}

		healthy := hc.checkChannel(ch)

		hc.mu.Lock()
		state, exists := hc.states[ch.ChannelID]
		if !exists {
			state = &channelHealthState{}
			hc.states[ch.ChannelID] = state
		}
		state.lastCheck = time.Now()

		if healthy {
			state.consecutiveFailures = 0
			state.lastResult = "healthy"
			if ch.HealthStatus != "healthy" {
				_ = hc.updateFn(ch.ChannelID, "healthy")
			}
		} else {
			state.consecutiveFailures++
			state.lastResult = "unhealthy"
			if state.consecutiveFailures >= hc.maxFailures && ch.HealthStatus != "unhealthy" {
				_ = hc.updateFn(ch.ChannelID, "unhealthy")
			}
		}
		hc.mu.Unlock()
	}
}

func (hc *HealthChecker) checkChannel(ch HealthChannelInfo) bool {
	cfg := ProviderConfig{
		ChannelType:  ch.ChannelType,
		ProviderType: ch.ProviderType,
		BaseURL:      ch.BaseURL,
		APIKey:       ch.APIKey,
	}

	adapter := ResolveExecutor(&cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := adapter.ListModels(ctx, &cfg)
	if err != nil {
		return false
	}
	return len(result) > 0
}

func (hc *HealthChecker) GetChannelState(channelID string) (consecutiveFailures int, lastResult string, lastCheck time.Time) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	state, exists := hc.states[channelID]
	if !exists {
		return 0, "", time.Time{}
	}
	return state.consecutiveFailures, state.lastResult, state.lastCheck
}

func (hc *HealthChecker) GetAllStates() map[string]int {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	states := make(map[string]int)
	for id, state := range hc.states {
		states[id] = state.consecutiveFailures
	}
	return states
}

func (hc *HealthChecker) SetCheckInterval(d time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if d > 0 {
		hc.checkInterval = d
	}
}

func (hc *HealthChecker) SetMaxFailures(n int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if n > 0 {
		hc.maxFailures = n
	}
}
