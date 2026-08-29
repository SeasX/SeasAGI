package memory

import (
	"time"
)

// MemoryEntry 记忆条目
type MemoryEntry struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Category  string    `json:"category"` // fact / preference / instruction / context
	CreatedAt time.Time `json:"created_at"`
	Relevance float64   `json:"relevance"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	Enabled        bool `json:"enabled"`
	MaxEntries     int  `json:"max_entries"`
	InjectionCount int  `json:"injection_count"` // 每次注入条数
	TTLHours       int  `json:"ttl_hours"`
}

// MemoryStore 记忆存储接口
type MemoryStore interface {
	Store(entry *MemoryEntry) error
	Retrieve(query string, limit int) ([]MemoryEntry, error)
	List() ([]MemoryEntry, error)
	Delete(id string) error
	Cleanup(ttl time.Duration) error
}
