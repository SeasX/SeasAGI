package protocol

import (
	"encoding/json"
	"fmt"
	"time"
)

// TranslateToChatCompletions 将 Responses API 请求转换为 Chat Completions 请求
func TranslateToChatCompletions(req *ResponsesRequest) (map[string]any, error) {
	chatReq := map[string]any{
		"model": req.Model,
	}

	// instructions → system message
	var messages []map[string]any
	if req.Instructions != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": req.Instructions,
		})
	}

	// input → messages
	inputMessages, err := translateInputToMessages(req.Input)
	if err != nil {
		return nil, fmt.Errorf("translate input: %w", err)
	}
	messages = append(messages, inputMessages...)
	chatReq["messages"] = messages

	// 参数映射
	if req.Temperature != nil {
		chatReq["temperature"] = *req.Temperature
	}
	if req.MaxOutputTokens != nil {
		chatReq["max_tokens"] = *req.MaxOutputTokens
	}
	if req.TopP != nil {
		chatReq["top_p"] = *req.TopP
	}
	if req.Stream {
		chatReq["stream"] = true
	}
	if len(req.Tools) > 0 {
		chatReq["tools"] = req.Tools
	}

	return chatReq, nil
}

// translateInputToMessages 将 input 字段转换为 messages
func translateInputToMessages(input json.RawMessage) ([]map[string]any, error) {
	if len(input) == 0 {
		return nil, nil
	}

	// 尝试解析为字符串
	var inputStr string
	if err := json.Unmarshal(input, &inputStr); err == nil {
		return []map[string]any{
			{"role": "user", "content": inputStr},
		}, nil
	}

	// 尝试解析为数组
	var inputItems []map[string]any
	if err := json.Unmarshal(input, &inputItems); err != nil {
		return nil, fmt.Errorf("input must be string or array, got: %s", string(input))
	}

	var messages []map[string]any
	for _, item := range inputItems {
		role, _ := item["role"].(string)
		if role == "" {
			role = "user"
		}
		content := item["content"]
		messages = append(messages, map[string]any{
			"role":    role,
			"content": content,
		})
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages found in input")
	}
	return messages, nil
}

// TranslateFromChatCompletions 将 Chat Completions 响应转换为 Responses API 响应
func TranslateFromChatCompletions(chatResp map[string]any, model string) (*ResponsesResponse, error) {
	id, _ := chatResp["id"].(string)
	if id == "" {
		id = fmt.Sprintf("resp_%d", time.Now().UnixNano())
	}

	// 提取 choices[0].message
	choices, _ := chatResp["choices"].([]any)
	var outputText string
	if len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok {
					outputText = content
				}
			}
		}
	}

	// 提取 usage
	usage, _ := chatResp["usage"].(map[string]any)
	inputTokens, _ := usage["prompt_tokens"].(float64)
	outputTokens, _ := usage["completion_tokens"].(float64)
	totalTokens, _ := usage["total_tokens"].(float64)

	return &ResponsesResponse{
		ID:     id,
		Object: "response",
		Status: "completed",
		Model:  model,
		Output: []ResponseOutputItem{
			{
				Type:    "message",
				ID:      "msg_0",
				Role:    "assistant",
				Status:  "completed",
				Content: map[string]any{
					"type": "output_text",
					"text": outputText,
				},
			},
		},
		Usage: ResponsesUsage{
			InputTokens:  int(inputTokens),
			OutputTokens: int(outputTokens),
			TotalTokens:  int(totalTokens),
		},
		CreatedAt: time.Now().Unix(),
	}, nil
}

// TranslateFromChatCompletionsBytes 将 Chat Completions JSON 响应转换为 Responses JSON
func TranslateFromChatCompletionsBytes(chatRespJSON []byte, model string) ([]byte, error) {
	var chatResp map[string]any
	if err := json.Unmarshal(chatRespJSON, &chatResp); err != nil {
		return nil, err
	}
	resp, err := TranslateFromChatCompletions(chatResp, model)
	if err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}
