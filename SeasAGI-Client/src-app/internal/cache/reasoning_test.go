package cache

import (
	"testing"
	"time"
)

func TestReasoningCacheStore(t *testing.T) {
	c := NewReasoningCache(1)
	c.Put("session-1", "reasoning content 1")
	c.Put("session-1", "reasoning content 2")

	entries, ok := c.Get("session-1")
	if !ok {
		t.Fatal("expected entries to exist")
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0] != "reasoning content 1" {
		t.Errorf("expected 'reasoning content 1', got %s", entries[0])
	}
}

func TestReasoningReinject(t *testing.T) {
	c := NewReasoningCache(1)
	c.Put("session-1", "previous reasoning")

	messages := []map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "hi"},
		{"role": "user", "content": "follow up"},
	}

	result := c.Reinject(messages, "session-1")
	// 应在第一个 assistant 消息前注入 reasoning
	foundReasoning := false
	for _, msg := range result {
		if content, ok := msg["content"].(string); ok && content == "previous reasoning" {
			foundReasoning = true
			break
		}
	}
	if !foundReasoning {
		t.Error("expected reasoning to be reinjected")
	}
	if len(result) != len(messages)+1 {
		t.Errorf("expected %d messages, got %d", len(messages)+1, len(result))
	}
}

func TestReasoningCacheTTL(t *testing.T) {
	c := NewReasoningCache(0) // 0 → 默认 1 小时
	// 手动测试 TTL 逻辑
	c.mu.Lock()
	c.store["session-1"] = []ReasoningEntry{
		{Content: "old", Timestamp: time.Now().Add(-2 * time.Hour)},
	}
	c.mu.Unlock()

	_, ok := c.Get("session-1")
	if ok {
		t.Error("expected expired entries to be unavailable")
	}
}

func TestReasoningCacheNoSession(t *testing.T) {
	c := NewReasoningCache(1)
	_, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected false for nonexistent session")
	}
}

func TestReasoningCacheDelete(t *testing.T) {
	c := NewReasoningCache(1)
	c.Put("session-1", "reasoning")
	c.Delete("session-1")
	_, ok := c.Get("session-1")
	if ok {
		t.Error("expected false after delete")
	}
}

func TestReasoningReinjectNoAssistant(t *testing.T) {
	c := NewReasoningCache(1)
	c.Put("session-1", "reasoning")

	messages := []map[string]any{
		{"role": "user", "content": "hello"},
	}

	result := c.Reinject(messages, "session-1")
	// 没有 assistant 消息，应追加到末尾
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[1]["role"] != "assistant" {
		t.Errorf("expected last message role assistant, got %v", result[1]["role"])
	}
}

func TestReasoningReinjectNoCache(t *testing.T) {
	c := NewReasoningCache(1)
	messages := []map[string]any{
		{"role": "user", "content": "hello"},
	}
	result := c.Reinject(messages, "nonexistent")
	if len(result) != len(messages) {
		t.Errorf("expected unchanged messages, got %d", len(result))
	}
}

func TestReasoningCacheCleanup(t *testing.T) {
	c := NewReasoningCache(1)
	c.Put("session-1", "reasoning")
	c.Put("session-2", "reasoning")

	// 手动让 session-1 过期
	c.mu.Lock()
	c.store["session-1"][0].Timestamp = time.Now().Add(-2 * time.Hour)
	c.mu.Unlock()

	c.Cleanup()

	_, ok1 := c.Get("session-1")
	if ok1 {
		t.Error("expected session-1 to be cleaned up")
	}
	_, ok2 := c.Get("session-2")
	if !ok2 {
		t.Error("expected session-2 to still exist")
	}
}
