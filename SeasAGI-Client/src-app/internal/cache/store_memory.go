package cache

import (
	"sync"
	"time"
)

// MemoryStore 内存缓存存储（LRU 简化实现）
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]*CacheEntry
	order   []string // 按插入顺序，用于 LRU
}

// NewMemoryStore 创建内存存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]*CacheEntry),
	}
}

func (s *MemoryStore) Get(key string) (*CacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok {
		return nil, nil
	}
	return entry, nil
}

func (s *MemoryStore) Set(key string, entry *CacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[key]; !exists {
		s.order = append(s.order, key)
	}
	s.entries[key] = entry
	return nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	for i, k := range s.order {
		if k == key {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return nil
}

func (s *MemoryStore) Count() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries), nil
}

func (s *MemoryStore) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[string]*CacheEntry)
	s.order = nil
	return nil
}

// EvictOldest 驱逐最旧条目
func (s *MemoryStore) EvictOldest(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := 0; i < n && i < len(s.order); i++ {
		key := s.order[i]
		if entry, ok := s.entries[key]; ok {
			if now.Sub(entry.CreatedAt) > time.Minute {
				delete(s.entries, key)
				s.order = append(s.order[:i], s.order[i+1:]...)
				i--
			}
		}
	}
}
