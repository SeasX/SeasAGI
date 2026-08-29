package compression

import (
	"encoding/json"
	"strings"
	"sync"
)

// Manager 压缩管理器
type Manager struct {
	mu       sync.Mutex
	registry *Registry
	config   CompressionConfig
	stats    CompressionStats
}

// NewManager 创建压缩管理器
func NewManager(cfg CompressionConfig) *Manager {
	return &Manager{
		registry: DefaultRegistry(),
		config:   cfg,
	}
}

// CompressRequest 执行压缩管道
// headerProfile 优先于 config 中的引擎列表，可选值：lite/standard/aggressive/ultra/off
func (m *Manager) CompressRequest(messages []map[string]any, headerProfile string) ([]map[string]any, *CompressionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 确定引擎列表
	engines := m.resolveEngines(headerProfile)
	if len(engines) == 0 {
		return messages, nil, nil
	}

	pipeline, err := m.registry.Pipeline(engines)
	if err != nil {
		return messages, nil, err
	}

	var lastResult *CompressionResult
	totalOriginal := estimateTokens(messages)

	for _, e := range pipeline {
		result, cr, err := e.Compress(messages, m.config)
		if err != nil {
			return messages, nil, err
		}
		messages = result
		lastResult = &cr
	}

	if lastResult != nil {
		totalCompressed := estimateTokens(messages)
		m.stats.TotalRequests++
		m.stats.TotalOriginalTokens += totalOriginal
		m.stats.TotalCompressedTokens += totalCompressed
		if totalOriginal > 0 {
			m.stats.TotalSavingsPct = float64(totalOriginal-totalCompressed) / float64(totalOriginal) * 100
		}
		lastResult.OriginalTokens = totalOriginal
		lastResult.CompressedTokens = totalCompressed
		lastResult.SavingsPct = savingsPct(totalOriginal, totalCompressed)
	}

	return messages, lastResult, nil
}

// resolveEngines 解析引擎列表
func (m *Manager) resolveEngines(headerProfile string) []EngineName {
	if headerProfile == "off" || headerProfile == "" {
		if !m.config.Enabled {
			return nil
		}
		return m.config.Engines
	}

	preset := PresetName(headerProfile)
	engines := PresetToEngines(preset)
	if engines == nil && headerProfile != "off" {
		// 自定义引擎名
		return []EngineName{EngineName(headerProfile)}
	}
	return engines
}

// GetStats 获取累计统计
func (m *Manager) GetStats() CompressionStats {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats
}

// SetConfig 更新配置
func (m *Manager) SetConfig(cfg CompressionConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = cfg
}

// GetConfig 获取当前配置
func (m *Manager) GetConfig() CompressionConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// estimateTokens 估算 token 数（~4 字符/token）
func estimateTokens(messages []map[string]any) int {
	total := 0
	for _, msg := range messages {
		total += 4 // role 开销
		if content, ok := msg["content"].(string); ok {
			total += len(content) / 4
		} else if parts, ok := msg["content"].([]any); ok {
			for _, p := range parts {
				if m, ok := p.(map[string]any); ok {
					if text, ok := m["text"].(string); ok {
						total += len(text) / 4
					}
				}
			}
		}
	}
	if total == 0 {
		return 1
	}
	return total
}

// savingsPct 计算节省百分比
func savingsPct(original, compressed int) float64 {
	if original <= 0 {
		return 0
	}
	return float64(original-compressed) / float64(original) * 100
}

// estimateTokensFromString 从字符串估算 token 数
func estimateTokensFromString(s string) int {
	return len(s) / 4
}

// messagesToJSON 将 messages 序列化为 JSON（用于调试）
func messagesToJSON(messages []map[string]any) string {
	b, _ := json.Marshal(messages)
	return string(b)
}

// isEmptyMessage 检查消息是否为空
func isEmptyMessage(msg map[string]any) bool {
	content, ok := msg["content"]
	if !ok {
		return true
	}
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}
