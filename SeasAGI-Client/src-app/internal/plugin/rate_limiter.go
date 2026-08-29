package plugin

import (
	"sync"
	"time"
)

// RateLimiter 每插件限流器。
type RateLimiter struct {
	mu        sync.Mutex
	maxCount  int
	windowMs  int64
	states    map[string]*rateLimitState
}

type rateLimitState struct {
	count      int
	windowStart int64 // unix milliseconds
}

// NewRateLimiter 创建限流器。
// maxCount 为窗口内最大调用次数，windowMs 为窗口大小（毫秒）。
func NewRateLimiter(maxCount, windowMs int) *RateLimiter {
	return &RateLimiter{
		maxCount: maxCount,
		windowMs: int64(windowMs),
		states:   make(map[string]*rateLimitState),
	}
}

// IsRateLimited 检查是否被限流。如果未被限流，则计数+1。
func (rl *RateLimiter) IsRateLimited(pluginName string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now().UnixMilli()
	state, ok := rl.states[pluginName]

	if !ok || now-state.windowStart >= rl.windowMs {
		// 新窗口
		rl.states[pluginName] = &rateLimitState{
			count:      1,
			windowStart: now,
		}
		return false
	}

	state.count++
	if state.count > rl.maxCount {
		return true
	}
	return false
}

// Reset 重置指定插件的限流状态。
func (rl *RateLimiter) Reset(pluginName string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.states, pluginName)
}

// ResetAll 重置所有限流状态。
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.states = make(map[string]*rateLimitState)
}
