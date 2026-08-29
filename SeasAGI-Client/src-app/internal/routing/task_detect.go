package routing

import (
	"strings"
)

// DetectTaskType analyzes request messages to auto-detect the task type.
// This uses lightweight heuristic rules (no model inference needed for the heuristic path).
func DetectTaskType(messages []map[string]interface{}) string {
	if len(messages) == 0 {
		return "chat"
	}

	// Collect all text content
	var textContent strings.Builder
	for _, msg := range messages {
		if content, ok := msg["content"].(string); ok {
			textContent.WriteString(content)
			textContent.WriteString(" ")
		} else if arr, ok := msg["content"].([]interface{}); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["text"].(string); ok {
						textContent.WriteString(t)
						textContent.WriteString(" ")
					}
					if typ, ok := m["type"].(string); ok && typ == "image_url" {
						return "vision" // Contains images
					}
				}
			}
		}
		if role, ok := msg["role"].(string); ok && role == "assistant" {
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				return "tools"
			}
		}
	}

	content := strings.ToLower(textContent.String())

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
