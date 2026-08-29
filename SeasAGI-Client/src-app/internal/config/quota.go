package config

import (
	"sync"
	"time"
)

// KeyQuota per-key USD 配额
type KeyQuota struct {
	mu       sync.Mutex
	quotas   map[string]*quotaEntry
}

type quotaEntry struct {
	MonthlyLimitUSD float64   `json:"monthly_limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	ResetAt         time.Time `json:"reset_at"`
}

// NewKeyQuota 创建配额管理器
func NewKeyQuota() *KeyQuota {
	return &KeyQuota{quotas: make(map[string]*quotaEntry)}
}

// SetQuota 设置 key 的月度配额
func (q *KeyQuota) SetQuota(keyID string, monthlyLimitUSD float64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.quotas[keyID] = &quotaEntry{
		MonthlyLimitUSD: monthlyLimitUSD,
		UsedUSD:         0,
		ResetAt:         nextMonthStart(time.Now()),
	}
}

// GetQuota 获取 key 的配额信息
func (q *KeyQuota) GetQuota(keyID string) (monthlyLimit, usedUSD float64, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.quotas[keyID]
	if !exists {
		return 0, 0, false
	}
	q.checkReset(keyID, entry)
	return entry.MonthlyLimitUSD, entry.UsedUSD, true
}

// Deduct 扣减配额，返回是否成功
func (q *KeyQuota) Deduct(keyID string, costUSD float64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.quotas[keyID]
	if !exists {
		return true // 无配额限制
	}
	q.checkReset(keyID, entry)
	if entry.MonthlyLimitUSD > 0 && entry.UsedUSD+costUSD > entry.MonthlyLimitUSD {
		return false // 超额
	}
	entry.UsedUSD += costUSD
	return true
}

// RemoveQuota 删除配额
func (q *KeyQuota) RemoveQuota(keyID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.quotas, keyID)
}

// checkReset 检查是否需要月度重置
func (q *KeyQuota) checkReset(keyID string, entry *quotaEntry) {
	if time.Now().After(entry.ResetAt) {
		entry.UsedUSD = 0
		entry.ResetAt = nextMonthStart(time.Now())
	}
}

// nextMonthStart 返回下个月的第一天
func nextMonthStart(t time.Time) time.Time {
	year := t.Year()
	month := t.Month() + 1
	if month > 12 {
		year++
		month = 1
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
}
