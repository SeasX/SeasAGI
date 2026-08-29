package ratelimit

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type bucket struct {
	tokens   float64
	maxToken float64
	rate     float64
	lastTime time.Time
}

type Policy struct {
	RPM int `json:"rpm"`
}

type Limiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	maxRate  float64
	policies map[string]Policy
}

func NewLimiter(maxRequestsPerMinute float64) *Limiter {
	if maxRequestsPerMinute <= 0 {
		maxRequestsPerMinute = 60
	}
	return &Limiter{
		buckets:  make(map[string]*bucket),
		maxRate:  maxRequestsPerMinute,
		policies: make(map[string]Policy),
	}
}

func (l *Limiter) SetPolicy(key string, rpm int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if rpm <= 0 {
		delete(l.policies, key)
		return
	}
	l.policies[key] = Policy{RPM: rpm}
}

func (l *Limiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		deviceID := c.GetString("device_id")
		subscriptionID := c.GetString("subscription_id")

		keys := []string{}
		if subscriptionID != "" {
			keys = append(keys, "subscription:"+subscriptionID)
		}
		if userID != "" {
			keys = append(keys, "user:"+userID)
		}
		if deviceID != "" {
			keys = append(keys, "device:"+deviceID)
		}
		if len(keys) == 0 {
			keys = append(keys, "ip:"+c.ClientIP())
		}

		for _, key := range keys {
			if !l.allow(key) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":       "rate limit exceeded",
					"type":        "rate_limit_error",
					"limited_key": key,
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

func (l *Limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	rate := l.maxRate
	if policy, ok := l.policies[key]; ok && policy.RPM > 0 {
		rate = float64(policy.RPM)
	}

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{
			tokens:   rate - 1,
			maxToken: rate,
			rate:     rate / 60.0,
			lastTime: now,
		}
		l.buckets[key] = b
		return true
	}

	if fmt.Sprintf("%.6f", b.maxToken) != fmt.Sprintf("%.6f", rate) {
		b.maxToken = rate
		b.rate = rate / 60.0
		if b.tokens > b.maxToken {
			b.tokens = b.maxToken
		}
	}

	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > b.maxToken {
		b.tokens = b.maxToken
	}
	b.lastTime = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (l *Limiter) CleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		now := time.Now()
		for k, b := range l.buckets {
			if now.Sub(b.lastTime) > 5*time.Minute {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}
