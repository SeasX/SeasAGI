package providers

import (
	"sync"
	"time"
)

type PenaltyEntry struct {
	Penalty int
	LastHit time.Time
	Count   int
}

type PenaltyManager struct {
	mu            sync.Mutex
	entries       map[string]*PenaltyEntry
	penaltyPer429 int
	maxPenalty    int
	decayInterval time.Duration
	decayAmount   int
}

func NewPenaltyManager() *PenaltyManager {
	return &PenaltyManager{
		entries:       make(map[string]*PenaltyEntry),
		penaltyPer429: 3,
		maxPenalty:    10,
		decayInterval: 2 * time.Minute,
		decayAmount:   1,
	}
}

func (pm *PenaltyManager) RecordRateLimit(key string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	entry, exists := pm.entries[key]
	if exists {
		entry.Count++
		entry.LastHit = now
		entry.Penalty += pm.penaltyPer429
		if entry.Penalty > pm.maxPenalty {
			entry.Penalty = pm.maxPenalty
		}
	} else {
		pm.entries[key] = &PenaltyEntry{
			Penalty: pm.penaltyPer429,
			LastHit: now,
			Count:   1,
		}
	}
}

func (pm *PenaltyManager) RecordSuccess(key string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	entry, exists := pm.entries[key]
	if !exists {
		return
	}
	entry.Penalty--
	if entry.Penalty <= 0 {
		delete(pm.entries, key)
	}
}

func (pm *PenaltyManager) GetPenalty(key string) int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	entry, exists := pm.entries[key]
	if !exists {
		return 0
	}

	now := time.Now()
	elapsed := now.Sub(entry.LastHit)
	decaySteps := int(elapsed / pm.decayInterval)
	if decaySteps > 0 {
		entry.Penalty -= decaySteps * pm.decayAmount
		entry.LastHit = now
		if entry.Penalty <= 0 {
			delete(pm.entries, key)
			return 0
		}
	}

	return entry.Penalty
}

func (pm *PenaltyManager) Reset(key string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.entries, key)
}

func (pm *PenaltyManager) GetAllPenalties() map[string]int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	result := make(map[string]int)
	for key, entry := range pm.entries {
		now := time.Now()
		elapsed := now.Sub(entry.LastHit)
		decaySteps := int(elapsed / pm.decayInterval)
		penalty := entry.Penalty - (decaySteps * pm.decayAmount)
		if penalty > 0 {
			result[key] = penalty
		}
	}
	return result
}

func (pm *PenaltyManager) SetDecayInterval(d time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if d > 0 {
		pm.decayInterval = d
	}
}

func (pm *PenaltyManager) SetPenaltyPer429(n int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if n > 0 {
		pm.penaltyPer429 = n
	}
}

type CooldownEntry struct {
	ExpiresAt time.Time
	Reason    string
}

type CooldownManager struct {
	mu       sync.Mutex
	entries  map[string]*CooldownEntry
	duration time.Duration
	stopCh   chan struct{}
}

func NewCooldownManager(duration time.Duration) *CooldownManager {
	cm := &CooldownManager{
		entries:  make(map[string]*CooldownEntry),
		duration: duration,
		stopCh:   make(chan struct{}),
	}
	go cm.cleanupLoop()
	return cm
}

func (cm *CooldownManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for {
		select {
		case <-ticker.C:
			cm.mu.Lock()
			now := time.Now()
			for key, entry := range cm.entries {
				if now.After(entry.ExpiresAt) {
					delete(cm.entries, key)
				}
			}
			cm.mu.Unlock()
		case <-cm.stopCh:
			ticker.Stop()
			return
		}
	}
}

func (cm *CooldownManager) Stop() {
	close(cm.stopCh)
}

func (cm *CooldownManager) SetCooldown(key string) {
	cm.SetCooldownWithReason(key, "rate_limited")
}

func (cm *CooldownManager) SetCooldownWithReason(key, reason string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.entries[key] = &CooldownEntry{
		ExpiresAt: time.Now().Add(cm.duration),
		Reason:    reason,
	}
}

func (cm *CooldownManager) IsOnCooldown(key string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	entry, exists := cm.entries[key]
	if !exists {
		return false
	}
	if time.Now().After(entry.ExpiresAt) {
		delete(cm.entries, key)
		return false
	}
	return true
}

func (cm *CooldownManager) GetRemaining(key string) time.Duration {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	entry, exists := cm.entries[key]
	if !exists {
		return 0
	}
	remaining := time.Until(entry.ExpiresAt)
	if remaining <= 0 {
		delete(cm.entries, key)
		return 0
	}
	return remaining
}

func (cm *CooldownManager) Clear(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	delete(cm.entries, key)
}

func (cm *CooldownManager) GetAll() map[string]time.Time {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	result := make(map[string]time.Time)
	now := time.Now()
	for key, entry := range cm.entries {
		if now.Before(entry.ExpiresAt) {
			result[key] = entry.ExpiresAt
		} else {
			delete(cm.entries, key)
		}
	}
	return result
}
