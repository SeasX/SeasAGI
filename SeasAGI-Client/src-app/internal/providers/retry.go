package providers

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

type RetryConfig struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Jitter         bool
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Jitter:         true,
	}
}

type RetryableFunc func() (*UpstreamResponse, error)
type IsRetryable func(error) bool

func RetryWithBackoff(ctx context.Context, cfg RetryConfig, fn RetryableFunc, isRetryable IsRetryable) (*UpstreamResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := calculateBackoff(attempt, cfg)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", cfg.MaxRetries, lastErr)
}

func calculateBackoff(attempt int, cfg RetryConfig) time.Duration {
	backoff := float64(cfg.InitialBackoff) * math.Pow(2, float64(attempt-1))
	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}

	if cfg.Jitter {
		backoff = backoff * (0.5 + rand.Float64()*0.5)
	}

	return time.Duration(backoff)
}

type CircuitBreaker struct {
	mu           sync.Mutex
	failures     int
	maxFailures  int
	openUntil    time.Time
	resetTimeout time.Duration
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.failures >= cb.maxFailures {
		if time.Now().Before(cb.openUntil) {
			return false
		}
		cb.failures = 0
	}
	return true
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	if cb.failures >= cb.maxFailures {
		cb.openUntil = time.Now().Add(cb.resetTimeout)
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
}

func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.failures >= cb.maxFailures && time.Now().Before(cb.openUntil) {
		return "open"
	}
	if cb.failures >= cb.maxFailures {
		return "half-open"
	}
	return "closed"
}
