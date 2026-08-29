package providers

import "strings"

// FamilyFallbackMap 模型家族回退映射
type FamilyFallbackMap struct {
	families map[string][]string
}

// NewFamilyFallbackMap 创建内置模型家族回退映射
func NewFamilyFallbackMap() *FamilyFallbackMap {
	m := &FamilyFallbackMap{families: make(map[string][]string)}
	// OpenAI 家族
	m.AddFamily("gpt-4", []string{"gpt-4-turbo", "gpt-4o"})
	m.AddFamily("gpt-4o", []string{"gpt-4o-mini", "gpt-4-turbo"})
	m.AddFamily("gpt-4o-mini", []string{"gpt-4o", "gpt-5-nano"})
	m.AddFamily("gpt-5", []string{"gpt-5.4", "gpt-5.4-mini", "gpt-4.1", "gpt-4o-mini"})
	m.AddFamily("gpt-5-mini", []string{"gpt-5-nano", "gpt-4o-mini"})
	m.AddFamily("gpt-5.4", []string{"gpt-5.4-mini", "gpt-4.1", "gpt-4o-mini"})
	m.AddFamily("gpt-5.4-mini", []string{"gpt-5.4-nano", "gpt-4o-mini"})
	m.AddFamily("gpt-5.5", []string{"gpt-5.4", "gpt-5.4-mini"})
	m.AddFamily("gpt-5.6-sol", []string{"gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5"})
	m.AddFamily("gpt-5.6-terra", []string{"gpt-5.6-luna", "gpt-5.5", "gpt-5.4"})
	m.AddFamily("gpt-5.6-luna", []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini"})
	m.AddFamily("gpt-5.3-codex", []string{"gpt-5.4", "gpt-4.1"})
	m.AddFamily("gpt-oss-120b", []string{"gpt-oss-20b", "gpt-4o-mini"})
	// Anthropic 家族
	m.AddFamily("claude-3-opus", []string{"claude-3-5-sonnet", "claude-3-sonnet"})
	m.AddFamily("claude-4-opus", []string{"claude-4-sonnet", "claude-3-5-sonnet"})
	m.AddFamily("claude-4-sonnet", []string{"claude-3-5-sonnet", "claude-3-haiku"})
	m.AddFamily("claude-3-5-sonnet", []string{"claude-3-haiku", "claude-3-sonnet"})
	m.AddFamily("claude-3-sonnet", []string{"claude-3-haiku"})
	m.AddFamily("claude-3-haiku", []string{"claude-3-5-sonnet"})
	m.AddFamily("claude-5", []string{"claude-sonnet-5", "claude-opus-4.8", "claude-haiku-4.5", "claude-sonnet-4-5"})
	m.AddFamily("claude-fable-5", []string{"claude-opus-5", "claude-opus-4.8", "claude-sonnet-5"})
	m.AddFamily("claude-opus-5", []string{"claude-opus-4.8", "claude-sonnet-5", "claude-3-5-sonnet"})
	m.AddFamily("claude-opus-4.8", []string{"claude-sonnet-5", "claude-sonnet-4-5", "claude-3-5-sonnet"})
	m.AddFamily("claude-sonnet-5", []string{"claude-sonnet-4-5", "claude-haiku-4.5", "claude-3-5-sonnet"})
	m.AddFamily("claude-haiku-4.5", []string{"claude-3-5-haiku", "claude-sonnet-5"})
	m.AddFamily("claude-sonnet-4-5", []string{"claude-3-5-sonnet", "claude-haiku-4.5"})
	// Google 家族
	m.AddFamily("gemini-1.5-pro", []string{"gemini-1.5-flash", "gemini-2.0-flash"})
	m.AddFamily("gemini-2.0-flash", []string{"gemini-1.5-flash", "gemini-2.5-flash"})
	m.AddFamily("gemini-2.5-pro", []string{"gemini-2.5-flash", "gemini-1.5-pro"})
	m.AddFamily("gemini-2.5-flash", []string{"gemini-2.0-flash", "gemini-1.5-flash"})
	m.AddFamily("gemini-3", []string{"gemini-3.5-flash", "gemini-3-flash", "gemini-2.5-flash"})
	m.AddFamily("gemini-3.1-pro", []string{"gemini-3.5-flash", "gemini-3-flash", "gemini-2.5-pro"})
	m.AddFamily("gemini-3.5-flash", []string{"gemini-3-flash", "gemini-2.5-flash"})
	m.AddFamily("gemini-3-flash", []string{"gemini-2.5-flash", "gemini-2.0-flash"})
	// DeepSeek 家族
	m.AddFamily("deepseek-chat", []string{"deepseek-v4-flash", "deepseek-coder"})
	m.AddFamily("deepseek-v4-pro", []string{"deepseek-v4-flash", "deepseek-chat", "deepseek-reasoner"})
	m.AddFamily("deepseek-reasoner", []string{"deepseek-v4-pro", "deepseek-chat"})
	m.AddFamily("deepseek-coder", []string{"deepseek-v4-flash", "deepseek-chat"})
	// 通义千问家族
	m.AddFamily("qwen-max", []string{"qwen-plus"})
	m.AddFamily("qwen-plus", []string{"qwen-turbo", "qwen-max"})
	m.AddFamily("qwen3.8-max", []string{"qwen3.7-max", "qwen3.6-plus", "qwen-plus"})
	m.AddFamily("qwen3.7-max", []string{"qwen3.6-plus", "qwen-plus", "qwen-turbo"})
	m.AddFamily("qwen3.6-plus", []string{"qwen-plus", "qwen-turbo"})
	m.AddFamily("qwen3-vl-plus", []string{"qwen3.6-plus", "qwen-plus"})
	// Zhipu GLM 家族
	m.AddFamily("glm-5", []string{"glm-4.5", "glm-4.5-flash", "glm-4-flash"})
	m.AddFamily("glm-4.5", []string{"glm-4.5-flash", "glm-4-flash"})
	m.AddFamily("glm-4.5-flash", []string{"glm-4-flash"})
	m.AddFamily("glm-4-flash", []string{"glm-4.5-flash"})
	// Kimi 家族
	m.AddFamily("kimi-k3", []string{"kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5"})
	m.AddFamily("kimi-k2.7-code", []string{"kimi-k2.6", "kimi-k2.5"})
	m.AddFamily("kimi-k2.6", []string{"kimi-k2.5"})
	// MiniMax 家族
	m.AddFamily("minimax-m3", []string{"minimax-m2.7"})
	m.AddFamily("minimax-m2.7", []string{"minimax-m3"})
	// Xiaomi MiMo 家族
	m.AddFamily("mimo-v2.5-pro", []string{"mimo-v2-flash", "mimo-7b-instruct"})
	m.AddFamily("mimo-v2.5-omni", []string{"mimo-v2-flash", "mimo-v2.5-pro"})
	m.AddFamily("mimo-v2-flash", []string{"mimo-7b-instruct"})
	// ByteDance Doubao 家族
	m.AddFamily("doubao-seed-2.1-pro", []string{"doubao-seed-2.1-turbo", "doubao-seed-2.0-lite"})
	m.AddFamily("doubao-seed-2.1-turbo", []string{"doubao-seed-2.0-lite"})
	m.AddFamily("doubao-seed-evolving", []string{"doubao-seed-2.1-pro", "doubao-seed-2.1-turbo"})
	// Tencent Hunyuan 家族
	m.AddFamily("hunyuan-hy3", []string{"hunyuan-turbo-s", "hunyuan-t1"})
	m.AddFamily("hunyuan-turbo-s", []string{"hunyuan-t1", "hunyuan-lite"})
	m.AddFamily("hunyuan-t1", []string{"hunyuan-lite"})
	m.AddFamily("hunyuan-lite", []string{"hunyuan-t1", "hunyuan-turbo-s"})
	// Mistral 家族
	m.AddFamily("mistral-large", []string{"mistral-medium", "mistral-small"})
	// Llama 家族
	m.AddFamily("llama-3.1-405b", []string{"llama-3.1-70b"})
	m.AddFamily("llama-3.1-70b", []string{"llama-3.1-8b"})
	return m
}

// AddFamily 添加模型家族映射
func (m *FamilyFallbackMap) AddFamily(model string, fallbacks []string) {
	model = strings.ToLower(model)
	for i, f := range fallbacks {
		fallbacks[i] = strings.ToLower(f)
	}
	m.families[model] = fallbacks
}

// GetFallbacks 获取模型的家族回退列表
func (m *FamilyFallbackMap) GetFallbacks(model string) []string {
	model = strings.ToLower(model)
	if fallbacks, ok := m.families[model]; ok {
		return fallbacks
	}
	return nil
}

// HasFamily 检查模型是否有家族回退
func (m *FamilyFallbackMap) HasFamily(model string) bool {
	model = strings.ToLower(model)
	_, ok := m.families[model]
	return ok
}

// FindClosestMatch 从可用模型列表中找到最接近的家族成员
func (m *FamilyFallbackMap) FindClosestMatch(model string, available []string) string {
	fallbacks := m.GetFallbacks(model)
	if fallbacks == nil {
		return ""
	}
	availMap := make(map[string]bool)
	for _, a := range available {
		availMap[strings.ToLower(a)] = true
	}
	for _, f := range fallbacks {
		if availMap[f] {
			return f
		}
	}
	return ""
}
