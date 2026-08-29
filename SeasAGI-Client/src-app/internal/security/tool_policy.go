package security

import (
	"strings"
	"sync"
)

// ToolPolicyMode 工具策略模式。
type ToolPolicyMode int

const (
	// ToolPolicyDisabled 不做限制（默认）
	ToolPolicyDisabled ToolPolicyMode = iota
	// ToolPolicyAllowlist 仅允许白名单中的工具
	ToolPolicyAllowlist
	// ToolPolicyDenylist 禁止黑名单中的工具
	ToolPolicyDenylist
)

// ToolPolicy 工具策略配置。
type ToolPolicy struct {
	mu        sync.RWMutex
	mode      ToolPolicyMode
	allowlist map[string]bool
	denylist  map[string]bool
}

// NewToolPolicy 创建默认策略（disabled）。
func NewToolPolicy() *ToolPolicy {
	return &ToolPolicy{
		mode:      ToolPolicyDisabled,
		allowlist: make(map[string]bool),
		denylist:  make(map[string]bool),
	}
}

// SetMode 设置策略模式。
func (p *ToolPolicy) SetMode(mode ToolPolicyMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode = mode
}

// SetAllowlist 设置白名单。
func (p *ToolPolicy) SetAllowlist(tools []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowlist = make(map[string]bool)
	for _, t := range tools {
		p.allowlist[strings.ToLower(t)] = true
	}
}

// SetDenylist 设置黑名单。
func (p *ToolPolicy) SetDenylist(tools []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.denylist = make(map[string]bool)
	for _, t := range tools {
		p.denylist[strings.ToLower(t)] = true
	}
}

// ToolPolicyResult 策略评估结果。
type ToolPolicyResult struct {
	Allowed bool
	Denied  []string // 被拒绝的工具名
}

// Evaluate 评估工具名列表是否通过策略。
func (p *ToolPolicy) Evaluate(toolNames []string) ToolPolicyResult {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := ToolPolicyResult{Allowed: true}

	switch p.mode {
	case ToolPolicyDisabled:
		// 不限制
		return result

	case ToolPolicyAllowlist:
		for _, name := range toolNames {
			lower := strings.ToLower(name)
			if !p.allowlist[lower] {
				result.Denied = append(result.Denied, name)
			}
		}

	case ToolPolicyDenylist:
		for _, name := range toolNames {
			lower := strings.ToLower(name)
			if p.denylist[lower] {
				result.Denied = append(result.Denied, name)
			}
		}
	}

	if len(result.Denied) > 0 {
		result.Allowed = false
	}
	return result
}

// ValidateToolsInRequest 从 OpenAI 请求体提取工具名并评估。
// body 是解析后的 JSON map。
func (p *ToolPolicy) ValidateToolsInRequest(body map[string]interface{}) ToolPolicyResult {
	toolNames := ExtractToolNames(body)
	return p.Evaluate(toolNames)
}

// ExtractToolNames 从 OpenAI 请求体提取工具名。
// 支持 tools 数组、functions 数组、tool_choice。
func ExtractToolNames(body map[string]interface{}) []string {
	var names []string

	// 从 tools 数组提取
	if tools, ok := body["tools"].([]interface{}); ok {
		for _, tool := range tools {
			if m, ok := tool.(map[string]interface{}); ok {
				if fn, ok := m["function"].(map[string]interface{}); ok {
					if name, ok := fn["name"].(string); ok {
						names = append(names, name)
					}
				}
				// 直接取 type 作为 tool name（如 code_interpreter）
				if typ, ok := m["type"].(string); ok && typ != "function" {
					names = append(names, typ)
				}
			}
		}
	}

	// 从 functions 数组提取（旧格式）
	if functions, ok := body["functions"].([]interface{}); ok {
		for _, fn := range functions {
			if m, ok := fn.(map[string]interface{}); ok {
				if name, ok := m["name"].(string); ok {
					names = append(names, name)
				}
			}
		}
	}

	// 从 tool_choice 提取
	if tc, ok := body["tool_choice"].(map[string]interface{}); ok {
		if fn, ok := tc["function"].(map[string]interface{}); ok {
			if name, ok := fn["name"].(string); ok {
				names = append(names, name)
			}
		}
	}

	return names
}
