package tls

import (
	"fmt"
	"sync"
	"time"
)

// BreakerState 熔断器状态类型。
type BreakerState int

const (
	BreakerClosed   BreakerState = iota // 正常，允许请求
	BreakerOpen                         // 开路，拒绝请求
	BreakerHalfOpen                     // 半开，允许探测
)

// CircuitBreaker 是一个简单的熔断器实现。
// 连续失败 maxFailures 次后开路，冷却时间从 minCooldown 指数增长到 maxCooldown。
type CircuitBreaker struct {
	mu              sync.Mutex
	state           BreakerState
	failureCount    int
	maxFailures     int
	minCooldown     time.Duration
	maxCooldown     time.Duration
	currentCooldown time.Duration
	openUntil       time.Time
}

// NewCircuitBreaker 创建熔断器。
func NewCircuitBreaker(maxFailures int, minCooldown, maxCooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:           BreakerClosed,
		maxFailures:     maxFailures,
		minCooldown:     minCooldown,
		maxCooldown:     maxCooldown,
		currentCooldown: minCooldown,
	}
}

// Allow 检查是否允许请求通过。
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case BreakerClosed:
		return true
	case BreakerOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = BreakerHalfOpen
			return true
		}
		return false
	case BreakerHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess 记录成功，重置熔断器。
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = BreakerClosed
	cb.currentCooldown = cb.minCooldown
}

// RecordFailure 记录失败，可能触发开路。
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++

	if cb.state == BreakerHalfOpen {
		// 半开状态下失败，重新开路
		cb.trip()
		return
	}

	if cb.failureCount >= cb.maxFailures {
		cb.trip()
	}
}

// trip 开路并计算冷却时间（指数退避）。
func (cb *CircuitBreaker) trip() {
	cb.state = BreakerOpen
	cb.failureCount = 0 // 重置失败计数，下次从 0 开始
	// 用当前冷却时间设置开路时长
	cb.openUntil = time.Now().Add(cb.currentCooldown)
	// 指数退避：下次开路时翻倍，上限 maxCooldown
	if cb.currentCooldown < cb.maxCooldown {
		cb.currentCooldown *= 2
		if cb.currentCooldown > cb.maxCooldown {
			cb.currentCooldown = cb.maxCooldown
		}
	}
}

// State 返回当前状态描述字符串。
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.state {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return fmt.Sprintf("open (retry in %v)", time.Until(cb.openUntil).Round(time.Second))
	case BreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// FailureCount 返回当前连续失败次数。
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failureCount
}

// CurrentCooldown 返回当前冷却时间。
func (cb *CircuitBreaker) CurrentCooldown() time.Duration {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentCooldown
}
