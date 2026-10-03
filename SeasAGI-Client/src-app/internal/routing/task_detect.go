package routing

import (
	"strings"
)

// messageScanText 汇总消息文本并标记多模态/工具调用信号，供任务类型与意图检测共用。
func messageScanText(messages []map[string]interface{}) (text string, hasImage, hasToolCalls bool) {
	var b strings.Builder
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			b.WriteString(content)
			b.WriteString(" ")
		} else if arr, ok := msg["content"].([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["text"].(string); ok {
						b.WriteString(t)
						b.WriteString(" ")
					}
					if typ, ok := m["type"].(string); ok && typ == "image_url" {
						hasImage = true
					}
				}
			}
		}
		if role, ok := msg["role"].(string); ok && role == "assistant" {
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				hasToolCalls = true
			}
		}
	}
	return b.String(), hasImage, hasToolCalls
}

// DetectTaskType analyzes request messages to auto-detect the task type.
// This uses lightweight heuristic rules (no model inference needed for the heuristic path).
func DetectTaskType(messages []map[string]interface{}) string {
	if len(messages) == 0 {
		return "chat"
	}

	textContent, hasImage, hasToolCalls := messageScanText(messages)
	if hasImage {
		return "vision" // Contains images
	}

	content := strings.ToLower(textContent)

	if hasToolCalls {
		return "tools"
	}

	// Check for tool/function requests
	if strings.Contains(content, "\"tools\"") || strings.Contains(content, "\"functions\"") {
		return "tools"
	}

	// Check for JSON/structured output requests
	if strings.Contains(content, "json") && (strings.Contains(content, "schema") || strings.Contains(content, "format") || strings.Contains(content, "parse")) {
		return "json"
	}
	if strings.Contains(content, "structured output") || strings.Contains(content, "structured data") {
		return "json"
	}

	// Check for long context (rough heuristic: > 8000 chars in messages)
	if len(content) > 8000 {
		return "long_context"
	}

	// Check for batch/low-cost patterns
	if strings.Contains(content, "batch") || strings.Contains(content, "bulk") || strings.Contains(content, "classify") {
		return "batch_low_cost"
	}

	return "chat"
}
