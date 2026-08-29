package cache

import (
	"sync"
	"time"
)

// ReasoningCache reasoning_content 缓存，用于多轮对话重注入
type ReasoningCache struct {
	mu       sync.Mutex
	store    map[string][]ReasoningEntry
	ttl      time.Duration
}

// ReasoningEntry 推理缓存条目
type ReasoningEntry struct {
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// NewReasoningCache 创建推理缓存
func NewReasoningCache(ttlHours int) *ReasoningCache {
	if ttlHours <= 0 {
		ttlHours = 1
	}
	return &ReasoningCache{
		store: make(map[string][]ReasoningEntry),
		ttl:   time.Duration(ttlHours) * time.Hour,
	}
}

// Put 存储推理内容
func (c *ReasoningCache) Put(sessionKey string, reasoning string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[sessionKey] = append(c.store[sessionKey], ReasoningEntry{
		Content:   reasoning,
		Timestamp: time.Now(),
	})
}

// Get 获取推理内容
func (c *ReasoningCache) Get(sessionKey string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, ok := c.store[sessionKey]
	if !ok || len(entries) == 0 {
		return nil, false
	}
	// 过滤过期条目
	var result []string
	now := time.Now()
	for _, e := range entries {
		if now.Sub(e.Timestamp) <= c.ttl {
			result = append(result, e.Content)
		}
	}
	if len(result) == 0 {
		delete(c.store, sessionKey)
		return nil, false
	}
	return result, true
}

// Reinject 将缓存的推理内容重注入到 messages
func (c *ReasoningCache) Reinject(messages []map[string]any, sessionKey string) []map[string]any {
	reasonings, ok := c.Get(sessionKey)
	if !ok || len(reasonings) == 0 {
		return messages
	}

	// 找到第一个 assistant 消息位置，在其前注入 reasoning
	var result []map[string]any
	injected := false
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if !injected && role == "assistant" {
			// 注入 reasoning 作为 assistant 消息前缀
			for _, r := range reasonings {
				result = append(result, map[string]any{
					"role":    "assistant",
					"content": r,
				})
			}
			injected = true
		}
		result = append(result, msg)
	}
	if !injected {
		// 没有 assistant 消息，追加到末尾
		for _, r := range reasonings {
			result = append(messages, map[string]any{
				"role":    "assistant",
				"content": r,
			})
		}
		return result
	}
	return result
}

// Cleanup 清理过期条目
func (c *ReasoningCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for key, entries := range c.store {
		var valid []ReasoningEntry
		for _, e := range entries {
			if now.Sub(e.Timestamp) <= c.ttl {
				valid = append(valid, e)
			}
		}
		if len(valid) == 0 {
			delete(c.store, key)
		} else {
			c.store[key] = valid
		}
	}
}

// Delete 删除会话的推理缓存
func (c *ReasoningCache) Delete(sessionKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, sessionKey)
}
