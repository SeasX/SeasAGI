// Package cache 提供响应缓存与推理内容缓存的实现。
//
// 状态：experimental（实验性），尚未接入生产路径：gateway 与路由当前都不读写本包，
// 全仓仅有本包测试引用。不要把它当作「已可用的缓存层」直接挂到热路径上。
// 复用前需先补齐：
//  1. 持久化后端（当前仅 MemoryStore，重启即失效）；
//  2. 跨进程/多实例一致性（多开网关时各自缓存会互相不可见）；
//  3. 命中率、节省额等指标接入真实计费口径（EstimatedSavings 目前恒为 0）。
package cache

import (
	"time"
)

// CacheConfig 缓存配置（experimental，未接入生产路径）。
type CacheConfig struct {
	Enabled            bool `json:"enabled"`
	TTLSeconds         int  `json:"ttl_seconds"`
	MaxEntries         int  `json:"max_entries"`
	IdempotentWindowMs int  `json:"idempotent_window_ms"` // 幂等窗口
}

// CacheEntry 缓存条目。
type CacheEntry struct {
	Key       string    `json:"key"`
	Response  []byte    `json:"response"`
	CreatedAt time.Time `json:"created_at"`
	HitCount  int       `json:"hit_count"`
}

// CacheStats 缓存统计。EstimatedSavings 尚未接入真实计费，恒为 0。
type CacheStats struct {
	TotalEntries     int     `json:"total_entries"`
	HitCount         int     `json:"hit_count"`
	MissCount        int     `json:"miss_count"`
	HitRate          float64 `json:"hit_rate"`
	EstimatedSavings float64 `json:"estimated_savings"` // 估算节省 USD
}

// Store 缓存存储接口。
type Store interface {
	Get(key string) (*CacheEntry, error)
	Set(key string, entry *CacheEntry) error
	Delete(key string) error
	Count() (int, error)
	Flush() error
}

// EvictableStore 可选接口：支持按最近最少使用顺序驱逐条目。
// Manager 用它实现 MaxEntries 容量上限；未实现该接口的 Store 无法被强制裁剪。
type EvictableStore interface {
	// EvictOldest 驱逐 n 个最久未使用的条目，返回实际删除数量。
	EvictOldest(n int) int
}

// ExpirableStore 可选接口：支持按 TTL 清理过期条目。
type ExpirableStore interface {
	// PurgeExpired 删除创建时间早于 now-ttl 的条目，返回实际删除数量。
	PurgeExpired(ttl time.Duration) int
}
