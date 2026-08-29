package providers

import (
	"sync"
	"sync/atomic"
)

// ConcurrencyLimiter tracks per-channel in-flight request counts and enforces
// per-channel concurrency limits using a counting-semaphore approach.
// A limit of 0 means unlimited (no blocking).
type ConcurrencyLimiter struct {
	mu       sync.RWMutex
	limits   map[string]int    // channelID → max concurrent (0=unlimited)
	inflight map[string]*int32  // channelID → current in-flight count
}

// NewConcurrencyLimiter creates a new limiter with empty state.
func NewConcurrencyLimiter() *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		limits:   make(map[string]int),
		inflight: make(map[string]*int32),
	}
}

// SetLimit sets the per-channel concurrency limit.
// A value of 0 means unlimited for that channel.
func (cl *ConcurrencyLimiter) SetLimit(channelID string, limit int) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	cl.limits[channelID] = limit
	if _, ok := cl.inflight[channelID]; !ok {
		var c int32
		cl.inflight[channelID] = &c
	}
}

// GetLimit returns the configured limit for a channel (0=unlimited).
func (cl *ConcurrencyLimiter) GetLimit(channelID string) int {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	return cl.limits[channelID]
}

// Acquire tries to acquire a concurrency slot for the channel.
// Returns true if the request can proceed, false if the channel is at capacity.
// For channels with no limit configured (0), always returns true.
func (cl *ConcurrencyLimiter) Acquire(channelID string) bool {
	cl.mu.RLock()
	limit := cl.limits[channelID]
	counter := cl.inflight[channelID]
	cl.mu.RUnlock()

	if limit == 0 {
		return true
	}

	if counter == nil {
		cl.mu.Lock()
		if counter = cl.inflight[channelID]; counter == nil {
			var c int32
			counter = &c
			cl.inflight[channelID] = counter
			cl.limits[channelID] = limit
		}
		cl.mu.Unlock()
	}

	current := atomic.LoadInt32(counter)
	if int(current) >= limit {
		return false
	}
	if !atomic.CompareAndSwapInt32(counter, current, current+1) {
		return false
	}
	return true
}

// Release decrements the in-flight counter for the channel.
// Safe to call even if Acquire returned false (no-op in that case).
func (cl *ConcurrencyLimiter) Release(channelID string) {
	cl.mu.RLock()
	counter := cl.inflight[channelID]
	cl.mu.RUnlock()

	if counter == nil {
		return
	}

	for {
		current := atomic.LoadInt32(counter)
		if current <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(counter, current, current-1) {
			return
		}
	}
}

// Inflight returns the current in-flight count for a channel.
func (cl *ConcurrencyLimiter) Inflight(channelID string) int {
	cl.mu.RLock()
	counter := cl.inflight[channelID]
	cl.mu.RUnlock()
	if counter == nil {
		return 0
	}
	return int(atomic.LoadInt32(counter))
}

// RemoveChannel cleans up state for a deleted channel.
func (cl *ConcurrencyLimiter) RemoveChannel(channelID string) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	delete(cl.limits, channelID)
	delete(cl.inflight, channelID)
}
