package cache

import (
	"testing"
	"time"
)

func TestCacheHitSamePrompt(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, TTLSeconds: 60})

	key := GenerateKey("gpt-4o", []map[string]any{
		{"role": "user", "content": "hello"},
	}, nil)

	// 第一次 miss
	_, hit := mgr.Get(key)
	if hit {
		t.Error("expected miss on first get")
	}

	// 写入
	mgr.Set(key, []byte(`{"response":"hi"}`))

	// 第二次 hit
	entry, hit := mgr.Get(key)
	if !hit {
		t.Fatal("expected hit on second get")
	}
	if string(entry.Response) != `{"response":"hi"}` {
		t.Errorf("unexpected response: %s", string(entry.Response))
	}
}

func TestCacheMissDifferentModel(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, TTLSeconds: 60})

	key1 := GenerateKey("gpt-4o", []map[string]any{
		{"role": "user", "content": "hello"},
	}, nil)
	key2 := GenerateKey("claude-3", []map[string]any{
		{"role": "user", "content": "hello"},
	}, nil)

	mgr.Set(key1, []byte("response1"))

	_, hit := mgr.Get(key2)
	if hit {
		t.Error("expected miss for different model")
	}
}

func TestCacheTTLExpiry(t *testing.T) {
	store := NewMemoryStore()
	mgr := NewManager(store, CacheConfig{Enabled: true, TTLSeconds: 1})

	key := "test-key"
	mgr.Set(key, []byte("response"))

	// 立即读取应命中
	_, hit := mgr.Get(key)
	if !hit {
		t.Error("expected hit before TTL expiry")
	}

	// 等待 TTL 过期
	time.Sleep(1100 * time.Millisecond)

	_, hit = mgr.Get(key)
	if hit {
		t.Error("expected miss after TTL expiry")
	}
}

func TestCacheFlush(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, TTLSeconds: 60})

	mgr.Set("key1", []byte("r1"))
	mgr.Set("key2", []byte("r2"))

	mgr.Flush()

	stats := mgr.GetStats()
	if stats.TotalEntries != 0 {
		t.Errorf("expected 0 entries after flush, got %d", stats.TotalEntries)
	}
}

func TestIdempotentDedup(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, IdempotentWindowMs: 1000})

	key := "idem-key"

	// 第一次不是重复
	if mgr.IsIdempotentHit(key) {
		t.Error("expected false on first idempotent check")
	}

	// 第二次在窗口内是重复
	if !mgr.IsIdempotentHit(key) {
		t.Error("expected true on second idempotent check")
	}
}

func TestIdempotentWindowExpiry(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, IdempotentWindowMs: 100})

	key := "idem-key2"
	mgr.IsIdempotentHit(key)

	time.Sleep(150 * time.Millisecond)

	// 窗口过期后不是重复
	if mgr.IsIdempotentHit(key) {
		t.Error("expected false after idempotent window expiry")
	}
}

func TestCacheStatsAccumulation(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: true, TTLSeconds: 60})

	// 2 misses
	mgr.Get("miss1")
	mgr.Get("miss2")

	// 1 hit
	mgr.Set("hit1", []byte("r"))
	mgr.Get("hit1")

	stats := mgr.GetStats()
	if stats.HitCount != 1 {
		t.Errorf("expected 1 hit, got %d", stats.HitCount)
	}
	if stats.MissCount != 2 {
		t.Errorf("expected 2 misses, got %d", stats.MissCount)
	}
	if stats.HitRate <= 0 {
		t.Errorf("expected hit rate > 0, got %.1f", stats.HitRate)
	}
}

func TestCacheKeyGeneration(t *testing.T) {
	msgs := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	key1 := GenerateKey("gpt-4o", msgs, nil)
	key2 := GenerateKey("gpt-4o", msgs, nil)

	if key1 != key2 {
		t.Error("expected same key for same input")
	}

	msgs2 := []map[string]any{
		{"role": "user", "content": "world"},
	}
	key3 := GenerateKey("gpt-4o", msgs2, nil)
	if key1 == key3 {
		t.Error("expected different key for different input")
	}
}

func TestCacheMaxEntries(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{
		Enabled:    true,
		TTLSeconds: 60,
		MaxEntries: 3,
	})

	// 写入超过最大条目
	for i := 0; i < 5; i++ {
		mgr.Set(GenerateKey("model", []map[string]any{
			{"role": "user", "content": string(rune('a' + i))},
		}, nil), []byte("r"))
	}

	stats := mgr.GetStats()
	if stats.TotalEntries != 3 {
		t.Errorf("expected exactly 3 entries (MaxEntries), got %d", stats.TotalEntries)
	}
}

// TestCacheMaxEntriesEvictsLeastRecentlyUsed 验证容量上限触发的是真实 LRU 淘汰：
// 被访问过的条目应保留，最久未使用且未被访问的条目应被驱逐。
func TestCacheMaxEntriesEvictsLeastRecentlyUsed(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{
		Enabled:    true,
		TTLSeconds: 60,
		MaxEntries: 3,
	})

	mgr.Set("a", []byte("a"))
	mgr.Set("b", []byte("b"))
	mgr.Set("c", []byte("c"))
	// 访问 a，使其成为最近使用；此时 b 是最久未使用。
	if _, hit := mgr.Get("a"); !hit {
		t.Fatal("expected hit for a before eviction")
	}

	mgr.Set("d", []byte("d"))

	if stats := mgr.GetStats(); stats.TotalEntries != 3 {
		t.Fatalf("entry count should stay at 3, got %d", stats.TotalEntries)
	}
	if _, hit := mgr.Get("b"); hit {
		t.Fatal("b should have been evicted as least recently used")
	}
	for _, key := range []string{"a", "c", "d"} {
		if _, hit := mgr.Get(key); !hit {
			t.Fatalf("key %q should still be cached", key)
		}
	}
}

// TestCacheCleanupRemovesExpiredEntries 验证 Cleanup 按 TTL 清理过期条目。
func TestCacheCleanupRemovesExpiredEntries(t *testing.T) {
	store := NewMemoryStore()
	mgr := NewManager(store, CacheConfig{Enabled: true, TTLSeconds: 1, MaxEntries: 10})

	mgr.Set("fresh", []byte("fresh"))
	// 直接写入一条 2 秒前的旧条目（超过 1 秒 TTL）。
	if err := store.Set("stale", &CacheEntry{
		Key:       "stale",
		Response:  []byte("stale"),
		CreatedAt: time.Now().Add(-2 * time.Second),
	}); err != nil {
		t.Fatalf("seed stale entry: %v", err)
	}

	mgr.Cleanup()

	if stats := mgr.GetStats(); stats.TotalEntries != 1 {
		t.Fatalf("expected only fresh entry after cleanup, got %d entries", stats.TotalEntries)
	}
	if _, hit := mgr.Get("stale"); hit {
		t.Fatal("stale entry should be purged by TTL cleanup")
	}
}

func TestCacheDisabled(t *testing.T) {
	mgr := NewManager(NewMemoryStore(), CacheConfig{Enabled: false})

	mgr.Set("key", []byte("r"))
	_, hit := mgr.Get("key")
	if hit {
		t.Error("expected no hit when cache disabled")
	}
}
