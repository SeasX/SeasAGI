package cache

import (
	"container/list"
	"sync"
	"time"
)

// MemoryStore 内存缓存存储（LRU 实现）。
//
// experimental：未接入生产路径，进程退出即丢失全部条目；重新接入前需要先补
// 持久化后端与多实例一致性（见包注释）。
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]*list.Element // key → LRU 节点
	lru     *list.List               // 前端为最近使用，后端为最久未使用
}

// memoryItem 是 LRU 链表中保存的条目。
type memoryItem struct {
	key   string
	entry *CacheEntry
}

// NewMemoryStore 创建内存存储。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		entries: make(map[string]*list.Element),
		lru:     list.New(),
	}
}

func (s *MemoryStore) Get(key string) (*CacheEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elem, ok := s.entries[key]
	if !ok {
		return nil, nil
	}
	// 命中即提升为最近使用，保证淘汰顺序是真正的 LRU。
	s.lru.MoveToFront(elem)
	return elem.Value.(*memoryItem).entry, nil
}

func (s *MemoryStore) Set(key string, entry *CacheEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if elem, exists := s.entries[key]; exists {
		elem.Value.(*memoryItem).entry = entry
		s.lru.MoveToFront(elem)
		return nil
	}
	s.entries[key] = s.lru.PushFront(&memoryItem{key: key, entry: entry})
	return nil
}

func (s *MemoryStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if elem, ok := s.entries[key]; ok {
		s.lru.Remove(elem)
		delete(s.entries, key)
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
	s.entries = make(map[string]*list.Element)
	s.lru.Init()
	return nil
}

// EvictOldest 驱逐 n 个最久未使用的条目，返回实际删除数量。
func (s *MemoryStore) EvictOldest(n int) int {
	if n <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	deleted := 0
	for deleted < n {
		elem := s.lru.Back()
		if elem == nil {
			break
		}
		item := elem.Value.(*memoryItem)
		s.lru.Remove(elem)
		delete(s.entries, item.key)
		deleted++
	}
	return deleted
}

// PurgeExpired 删除创建时间早于 now-ttl 的条目，返回实际删除数量。
func (s *MemoryStore) PurgeExpired(ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	deleted := 0
	for key, elem := range s.entries {
		if now.Sub(elem.Value.(*memoryItem).entry.CreatedAt) > ttl {
			s.lru.Remove(elem)
			delete(s.entries, key)
			deleted++
		}
	}
	return deleted
}
