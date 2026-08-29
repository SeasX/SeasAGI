package memory

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore 内存存储实现
type InMemoryStore struct {
	mu      sync.Mutex
	entries map[string]*MemoryEntry
	counter int
}

// NewInMemoryStore 创建内存存储
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{entries: make(map[string]*MemoryEntry)}
}

func (s *InMemoryStore) Store(entry *MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		s.counter++
		entry.ID = "mem_" + time.Now().Format("20060102150405") + "_" + intToStr(s.counter)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	s.entries[entry.ID] = entry
	return nil
}

func (s *InMemoryStore) Retrieve(query string, limit int) ([]MemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	queryLower := strings.ToLower(query)
	var results []MemoryEntry

	for _, entry := range s.entries {
		// 简单关键词匹配
		contentLower := strings.ToLower(entry.Content)
		if strings.Contains(contentLower, queryLower) || strings.Contains(queryLower, contentLower) {
			results = append(results, *entry)
		}
	}

	// 按 relevance 降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Relevance > results[j].Relevance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (s *InMemoryStore) List() ([]MemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var results []MemoryEntry
	for _, entry := range s.entries {
		results = append(results, *entry)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	return results, nil
}

func (s *InMemoryStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
	return nil
}

func (s *InMemoryStore) Cleanup(ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, entry := range s.entries {
		if now.Sub(entry.CreatedAt) > ttl {
			delete(s.entries, id)
		}
	}
	return nil
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
