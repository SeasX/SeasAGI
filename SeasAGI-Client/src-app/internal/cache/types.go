package cache

import (
	"time"
)

// CacheConfig 缓存配置
type CacheConfig struct {
	Enabled           bool `json:"enabled"`
	TTLSeconds        int  `json:"ttl_seconds"`
	MaxEntries        int  `json:"max_entries"`
	IdempotentWindowMs int  `json:"idempotent_window_ms"` // 幂等窗口
}

// CacheEntry 缓存条目
type CacheEntry struct {
	Key       string    `json:"key"`
	Response  []byte    `json:"response"`
	CreatedAt time.Time `json:"created_at"`
	HitCount  int       `json:"hit_count"`
}

// CacheStats 缓存统计
type CacheStats struct {
	TotalEntries      int     `json:"total_entries"`
	HitCount          int     `json:"hit_count"`
	MissCount         int     `json:"miss_count"`
	HitRate           float64 `json:"hit_rate"`
	EstimatedSavings  float64 `json:"estimated_savings"` // 估算节省 USD
}

// Store 缓存存储接口
type Store interface {
	Get(key string) (*CacheEntry, error)
	Set(key string, entry *CacheEntry) error
	Delete(key string) error
	Count() (int, error)
	Flush() error
}
