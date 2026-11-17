package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

// KeyQuota per-key USD 配额。
//
// 配额状态落盘到 JSON 文件，进程重启后仍然有效；未指定路径时退化为纯内存模式
// （用于测试或未启用持久化的场景）。
type KeyQuota struct {
	mu     sync.Mutex
	quotas map[string]*quotaEntry
	path   string
}

type quotaEntry struct {
	MonthlyLimitUSD float64   `json:"monthly_limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	ResetAt         time.Time `json:"reset_at"`
}

// DefaultQuotaPath 返回默认配额持久化路径：~/.seasagi/key_quotas.json。
func DefaultQuotaPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".seasagi", "key_quotas.json")
}

// NewKeyQuota 创建纯内存配额管理器（不落盘）。
func NewKeyQuota() *KeyQuota {
	return &KeyQuota{quotas: make(map[string]*quotaEntry)}
}

// NewPersistentKeyQuota 创建带持久化的配额管理器。
// path 为空时等价于 NewKeyQuota；读取失败不会阻断启动，只会记录告警。
func NewPersistentKeyQuota(path string) *KeyQuota {
	q := &KeyQuota{quotas: make(map[string]*quotaEntry), path: path}
	q.load()
	return q
}

// load 从磁盘恢复配额状态。
func (q *KeyQuota) load() {
	if q.path == "" {
		return
	}
	data, err := os.ReadFile(q.path)
	if err != nil {
		if !os.IsNotExist(err) {
			logging.Warningf("key quota: read %s failed: %v", q.path, err)
		}
		return
	}
	var loaded map[string]*quotaEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		logging.Warningf("key quota: decode %s failed: %v", q.path, err)
		return
	}
	for key, entry := range loaded {
		if entry != nil {
			q.quotas[key] = entry
		}
	}
}

// persistLocked 原子写入配额状态；调用方必须已持有 q.mu。
func (q *KeyQuota) persistLocked() {
	if q.path == "" {
		return
	}
	data, err := json.MarshalIndent(q.quotas, "", "  ")
	if err != nil {
		logging.Warningf("key quota: encode failed: %v", err)
		return
	}
	dir := filepath.Dir(q.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		logging.Warningf("key quota: create dir %s failed: %v", dir, err)
		return
	}
	tmp := q.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		logging.Warningf("key quota: write %s failed: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, q.path); err != nil {
		logging.Warningf("key quota: rename %s failed: %v", q.path, err)
	}
}

// SetQuota 设置 key 的月度配额。
func (q *KeyQuota) SetQuota(keyID string, monthlyLimitUSD float64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.quotas[keyID] = &quotaEntry{
		MonthlyLimitUSD: monthlyLimitUSD,
		UsedUSD:         0,
		ResetAt:         nextMonthStart(time.Now()),
	}
	q.persistLocked()
}

// GetQuota 获取 key 的配额信息。
func (q *KeyQuota) GetQuota(keyID string) (monthlyLimit, usedUSD float64, ok bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.quotas[keyID]
	if !exists {
		return 0, 0, false
	}
	if q.checkReset(keyID, entry) {
		q.persistLocked()
	}
	return entry.MonthlyLimitUSD, entry.UsedUSD, true
}

// Deduct 扣减配额，返回是否成功。
func (q *KeyQuota) Deduct(keyID string, costUSD float64) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	entry, exists := q.quotas[keyID]
	if !exists {
		return true // 无配额限制
	}
	if q.checkReset(keyID, entry) {
		defer q.persistLocked()
	}
	if entry.MonthlyLimitUSD > 0 && entry.UsedUSD+costUSD > entry.MonthlyLimitUSD {
		return false // 超额
	}
	entry.UsedUSD += costUSD
	q.persistLocked()
	return true
}

// RemoveQuota 删除配额。
func (q *KeyQuota) RemoveQuota(keyID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.quotas, keyID)
	q.persistLocked()
}

// checkReset 检查是否需要月度重置，返回是否发生了重置。
func (q *KeyQuota) checkReset(keyID string, entry *quotaEntry) bool {
	if time.Now().After(entry.ResetAt) {
		entry.UsedUSD = 0
		entry.ResetAt = nextMonthStart(time.Now())
		return true
	}
	return false
}

// nextMonthStart 返回下个月的第一天。
func nextMonthStart(t time.Time) time.Time {
	year := t.Year()
	month := t.Month() + 1
	if month > 12 {
		year++
		month = 1
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, t.Location())
}
