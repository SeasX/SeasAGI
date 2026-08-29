package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Manager 缓存管理器
type Manager struct {
	mu             sync.Mutex
	store          Store
	config         CacheConfig
	stats          CacheStats
	idempotentMu   sync.Mutex
	idempotentMap  map[string]int64 // key → timestamp ms
}

// NewManager 创建缓存管理器
func NewManager(store Store, cfg CacheConfig) *Manager {
	if cfg.TTLSeconds <= 0 {
		cfg.TTLSeconds = 300 // 默认 5 分钟
	}
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 1000
	}
	if cfg.IdempotentWindowMs <= 0 {
		cfg.IdempotentWindowMs = 5000
	}
	return &Manager{
		store:         store,
		config:        cfg,
		idempotentMap: make(map[string]int64),
	}
}

// GenerateKey 根据请求参数生成缓存 key
func GenerateKey(model string, messages []map[string]any, params map[string]any) string {
	h := sha256.New()
	h.Write([]byte(model))
	h.Write([]byte("|"))
	msgsJSON, _ := json.Marshal(messages)
	h.Write(msgsJSON)
	h.Write([]byte("|"))
	paramsJSON, _ := json.Marshal(params)
	h.Write(paramsJSON)
	return hex.EncodeToString(h.Sum(nil))
}

// Get 查询缓存
func (m *Manager) Get(key string) (*CacheEntry, bool) {
	if !m.config.Enabled {
		return nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	entry, err := m.store.Get(key)
	if err != nil || entry == nil {
		m.stats.MissCount++
		m.updateHitRate()
		return nil, false
	}

	// TTL 检查
	if time.Since(entry.CreatedAt) > time.Duration(m.config.TTLSeconds)*time.Second {
		_ = m.store.Delete(key)
		m.stats.MissCount++
		m.updateHitRate()
		return nil, false
	}

	entry.HitCount++
	_ = m.store.Set(key, entry)
	m.stats.HitCount++
	m.updateHitRate()
	return entry, true
}

// Set 写入缓存
func (m *Manager) Set(key string, response []byte) {
	if !m.config.Enabled {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 超过最大条目时清理最旧条目
	count, _ := m.store.Count()
	if count >= m.config.MaxEntries {
		m.cleanupOldest(count - m.config.MaxEntries + 1)
	}

	_ = m.store.Set(key, &CacheEntry{
		Key:       key,
		Response:  response,
		CreatedAt: time.Now(),
	})
}

// Flush 清空缓存
func (m *Manager) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = m.store.Flush()
	m.stats = CacheStats{}
}

// GetStats 获取统计
func (m *Manager) GetStats() CacheStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	count, _ := m.store.Count()
	m.stats.TotalEntries = count
	return m.stats
}

// Cleanup TTL 驱逐
func (m *Manager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Store 实现负责 TTL 驱逐
}

// IsIdempotentHit 检查幂等窗口内是否重复
func (m *Manager) IsIdempotentHit(key string) bool {
	if !m.config.Enabled || m.config.IdempotentWindowMs <= 0 {
		return false
	}

	m.idempotentMu.Lock()
	defer m.idempotentMu.Unlock()

	now := time.Now().UnixMilli()
	window := int64(m.config.IdempotentWindowMs)

	// 清理过期条目
	for k, ts := range m.idempotentMap {
		if now-ts > window {
			delete(m.idempotentMap, k)
		}
	}

	if ts, ok := m.idempotentMap[key]; ok {
		if now-ts < window {
			return true
		}
	}

	m.idempotentMap[key] = now
	return false
}

// updateHitRate 更新命中率
func (m *Manager) updateHitRate() {
	total := m.stats.HitCount + m.stats.MissCount
	if total > 0 {
		m.stats.HitRate = float64(m.stats.HitCount) / float64(total) * 100
	}
}

// cleanupOldest 清理最旧条目（由 Store 实现）
func (m *Manager) cleanupOldest(n int) {
	// 简化实现：Store.Flush() 或由 Store 自行管理 LRU
}
