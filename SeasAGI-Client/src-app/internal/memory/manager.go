package memory

import (
	"strings"
	"sync"
	"time"
)

// Manager 记忆管理器
type Manager struct {
	mu       sync.Mutex
	store    MemoryStore
	config   MemoryConfig
}

// NewManager 创建记忆管理器
func NewManager(store MemoryStore, cfg MemoryConfig) *Manager {
	if cfg.MaxEntries <= 0 {
		cfg.MaxEntries = 100
	}
	if cfg.InjectionCount <= 0 {
		cfg.InjectionCount = 5
	}
	if cfg.TTLHours <= 0 {
		cfg.TTLHours = 168 // 默认 7 天
	}
	return &Manager{store: store, config: cfg}
}

// ExtractAndStore 从对话中提取记忆并存储
func (m *Manager) ExtractAndStore(messages []map[string]any, sessionKey string) error {
	if !m.config.Enabled {
		return nil
	}

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if content == "" {
			continue
		}

		// 从 user 消息提取偏好/指令
		if role == "user" {
			if entry := extractPreference(content); entry != nil {
				if err := m.store.Store(entry); err != nil {
					return err
				}
			}
			if entry := extractInstruction(content); entry != nil {
				if err := m.store.Store(entry); err != nil {
					return err
				}
			}
		}

		// 从 assistant 消息提取事实
		if role == "assistant" {
			if entry := extractFact(content); entry != nil {
				if err := m.store.Store(entry); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// RetrieveAndInject 检索相关记忆并注入到 messages
func (m *Manager) RetrieveAndInject(messages []map[string]any, query string) ([]map[string]any, error) {
	if !m.config.Enabled {
		return messages, nil
	}

	entries, err := m.store.Retrieve(query, m.config.InjectionCount)
	if err != nil || len(entries) == 0 {
		return messages, nil
	}

	// 构建记忆注入文本
	var sb strings.Builder
	sb.WriteString("[Relevant memories from previous conversations]\n")
	for _, e := range entries {
		sb.WriteString("- ")
		sb.WriteString(e.Content)
		sb.WriteString("\n")
	}

	// 注入到 system message
	var result []map[string]any
	injected := false
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" && !injected {
			if content, ok := msg["content"].(string); ok {
				msg["content"] = content + "\n\n" + sb.String()
			}
			injected = true
		}
		result = append(result, msg)
	}

	if !injected {
		// 在开头插入 system message
		result = append([]map[string]any{
			{"role": "system", "content": sb.String()},
		}, messages...)
	}

	return result, nil
}

// ListMemories 列出所有记忆
func (m *Manager) ListMemories() ([]MemoryEntry, error) {
	return m.store.List()
}

// DeleteMemory 删除记忆
func (m *Manager) DeleteMemory(id string) error {
	return m.store.Delete(id)
}

// AddMemory 手动添加记忆
func (m *Manager) AddMemory(content, category string) error {
	if !m.config.Enabled {
		return nil
	}
	return m.store.Store(&MemoryEntry{
		Content:   content,
		Category:  category,
		Relevance: 1.0,
	})
}

// Cleanup 清理过期记忆
func (m *Manager) Cleanup() {
	m.store.Cleanup(time.Duration(m.config.TTLHours) * time.Hour)
}

// extractPreference 从用户消息中提取偏好
func extractPreference(content string) *MemoryEntry {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "i prefer") || strings.Contains(lower, "i like") || strings.Contains(lower, "i always") {
		return &MemoryEntry{
			Content:   content,
			Category:  "preference",
			Relevance: 0.8,
		}
	}
	return nil
}

// extractInstruction 从用户消息中提取指令
func extractInstruction(content string) *MemoryEntry {
	lower := strings.ToLower(content)
	if strings.Contains(lower, "always ") || strings.Contains(lower, "never ") || strings.Contains(lower, "remember that") {
		return &MemoryEntry{
			Content:   content,
			Category:  "instruction",
			Relevance: 0.9,
		}
	}
	return nil
}

// extractFact 从助手消息中提取事实
func extractFact(content string) *MemoryEntry {
	// 简化：长回复可能包含有用事实
	if len(content) > 100 && (strings.Contains(content, "is") || strings.Contains(content, "are") || strings.Contains(content, "was")) {
		// 只提取前 200 字符作为事实摘要
		fact := content
		if len(fact) > 200 {
			fact = fact[:200] + "..."
		}
		return &MemoryEntry{
			Content:   fact,
			Category:  "fact",
			Relevance: 0.5,
		}
	}
	return nil
}
