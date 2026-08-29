package protocol

import "encoding/json"

// ResponsesRequest OpenAI Responses API 请求格式
type ResponsesRequest struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`            // string | []ResponseInputItem
	Instructions    string          `json:"instructions,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Tools           []any           `json:"tools,omitempty"`
	PreviousResponseID string       `json:"previous_response_id,omitempty"`
}

// ResponseInputItem Responses API 输入条目
type ResponseInputItem struct {
	Type    string `json:"type"`
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
}

// ResponseOutputItem Responses API 输出条目
type ResponseOutputItem struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Status  string `json:"status,omitempty"`
	Content any    `json:"content,omitempty"`
}

// ResponsesUsage Responses API usage
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponsesResponse OpenAI Responses API 响应格式
type ResponsesResponse struct {
	ID        string              `json:"id"`
	Object    string              `json:"object"` // "response"
	Status    string              `json:"status"` // "completed"
	Model     string              `json:"model"`
	Output    []ResponseOutputItem `json:"output"`
	Usage     ResponsesUsage      `json:"usage"`
	CreatedAt int64               `json:"created_at"`
}

// IsResponsesRequest 检查是否为 Responses API 请求
func IsResponsesRequest(body map[string]any) bool {
	if body == nil {
		return false
	}
	_, hasInput := body["input"]
	return hasInput
}

// ParseResponsesRequest 从 JSON body 解析 Responses API 请求
func ParseResponsesRequest(body []byte) (*ResponsesRequest, error) {
	var req ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	return &req, nil
}
