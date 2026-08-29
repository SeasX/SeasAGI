package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TranslateStream 将 Chat Completions SSE 流转换为 Responses API SSE 事件流
// 输入：Chat Completions SSE 格式（data: {...}\n\n）
// 输出：Responses API SSE 格式（event: {...}\n\n）
func TranslateStream(chatSSELine string) (string, bool) {
	chatSSELine = strings.TrimSpace(chatSSELine)
	if chatSSELine == "" {
		return "", false
	}
	if chatSSELine == "data: [DONE]" {
		return "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n", true
	}
	if !strings.HasPrefix(chatSSELine, "data: ") {
		return "", false
	}

	dataStr := strings.TrimPrefix(chatSSELine, "data: ")
	var chunk map[string]any
	if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
		return "", false
	}

	// 提取 delta
	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return "", false
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	content, _ := delta["content"].(string)

	if content == "" {
		return "", false
	}

	// 转换为 Responses API 事件
	event := map[string]any{
		"type": "response.output_text.delta",
		"delta": content,
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: response.output_text.delta\ndata: %s\n\n", string(eventJSON)), true
}

// CreateResponseCreatedEvent 创建 response.created 事件
func CreateResponseCreatedEvent(model string) string {
	event := map[string]any{
		"type":  "response.created",
		"model": model,
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: response.created\ndata: %s\n\n", string(eventJSON))
}

// CreateResponseCompletedEvent 创建 response.completed 事件
func CreateResponseCompletedEvent(model string, inputTokens, outputTokens int) string {
	event := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"status": "completed",
			"model":  model,
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"total_tokens":  inputTokens + outputTokens,
			},
		},
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: response.completed\ndata: %s\n\n", string(eventJSON))
}
