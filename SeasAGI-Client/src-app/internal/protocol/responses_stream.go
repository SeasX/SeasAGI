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

// ChatStreamUsage 从 Chat Completions 流式块中提取 usage（兼容 prompt/completion 与 input/output 命名）。
// 返回 (输入 token, 输出 token, 是否携带 usage)。
func ChatStreamUsage(chunk map[string]any) (int, int, bool) {
	raw, ok := chunk["usage"].(map[string]any)
	if !ok || len(raw) == 0 {
		return 0, 0, false
	}
	in := int(toFloat(raw["prompt_tokens"]))
	if in == 0 {
		in = int(toFloat(raw["input_tokens"]))
	}
	out := int(toFloat(raw["completion_tokens"]))
	if out == 0 {
		out = int(toFloat(raw["output_tokens"]))
	}
	return in, out, true
}

// CreateResponseCompletedEvent 创建 response.completed 事件
func CreateResponseCompletedEvent(model string, inputTokens, outputTokens int) string {
	return CreateResponseCompletedEventEx(model, inputTokens, outputTokens, false)
}

// CreateResponseCompletedEventEx 创建 response.completed 事件；estimated 表示 usage 为本地估算。
func CreateResponseCompletedEventEx(model string, inputTokens, outputTokens int, estimated bool) string {
	usage := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
		"total_tokens":  inputTokens + outputTokens,
	}
	if estimated {
		usage["cost_estimated"] = true
	}
	event := map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"status": "completed",
			"model":  model,
			"usage":  usage,
		},
	}
	eventJSON, _ := json.Marshal(event)
	return fmt.Sprintf("event: response.completed\ndata: %s\n\n", string(eventJSON))
}
