package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResponsesToChatBasic(t *testing.T) {
	input, _ := json.Marshal("hello world")
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: input,
	}

	chatReq, err := TranslateToChatCompletions(req)
	if err != nil {
		t.Fatalf("TranslateToChatCompletions: %v", err)
	}

	if chatReq["model"] != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %v", chatReq["model"])
	}

	messages, ok := chatReq["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("expected messages slice, got %T", chatReq["messages"])
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["role"] != "user" {
		t.Errorf("expected role user, got %v", messages[0]["role"])
	}
	if messages[0]["content"] != "hello world" {
		t.Errorf("expected content 'hello world', got %v", messages[0]["content"])
	}
}

func TestResponsesToChatWithInstructions(t *testing.T) {
	input, _ := json.Marshal("hello")
	req := &ResponsesRequest{
		Model:        "gpt-4o",
		Input:        input,
		Instructions: "You are a helpful assistant.",
	}

	chatReq, err := TranslateToChatCompletions(req)
	if err != nil {
		t.Fatalf("TranslateToChatCompletions: %v", err)
	}

	messages := chatReq["messages"].([]map[string]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(messages))
	}
	if messages[0]["role"] != "system" {
		t.Errorf("expected first message role system, got %v", messages[0]["role"])
	}
	if messages[0]["content"] != "You are a helpful assistant." {
		t.Errorf("expected system content, got %v", messages[0]["content"])
	}
}

func TestResponsesFromChatBasic(t *testing.T) {
	chatResp := map[string]any{
		"id":     "chatcmpl-123",
		"model":  "gpt-4o",
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": "Hello! How can I help you?",
				},
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     float64(10),
			"completion_tokens": float64(20),
			"total_tokens":      float64(30),
		},
	}

	resp, err := TranslateFromChatCompletions(chatResp, "gpt-4o")
	if err != nil {
		t.Fatalf("TranslateFromChatCompletions: %v", err)
	}

	if resp.Object != "response" {
		t.Errorf("expected object 'response', got %s", resp.Object)
	}
	if resp.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", resp.Status)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
	if resp.Output[0].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Output[0].Role)
	}
	if resp.Usage.InputTokens != 10 {
		t.Errorf("expected input tokens 10, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("expected output tokens 20, got %d", resp.Usage.OutputTokens)
	}
}

func TestResponsesStreamTransform(t *testing.T) {
	chatSSE := `data: {"choices":[{"delta":{"content":"Hello"}}]}`

	out, ok := TranslateStream(chatSSE)
	if !ok {
		t.Fatal("expected stream translation to succeed")
	}
	if !strings.Contains(out, "response.output_text.delta") {
		t.Errorf("expected response.output_text.delta event, got: %s", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected 'Hello' in output, got: %s", out)
	}
}

func TestResponsesInputString(t *testing.T) {
	input, _ := json.Marshal("test string")
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: input,
	}

	chatReq, err := TranslateToChatCompletions(req)
	if err != nil {
		t.Fatalf("TranslateToChatCompletions: %v", err)
	}

	messages := chatReq["messages"].([]map[string]any)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0]["content"] != "test string" {
		t.Errorf("expected 'test string', got %v", messages[0]["content"])
	}
}

func TestResponsesInputArray(t *testing.T) {
	input, _ := json.Marshal([]map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "hi there"},
		{"role": "user", "content": "how are you?"},
	})
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: input,
	}

	chatReq, err := TranslateToChatCompletions(req)
	if err != nil {
		t.Fatalf("TranslateToChatCompletions: %v", err)
	}

	messages := chatReq["messages"].([]map[string]any)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[2]["content"] != "how are you?" {
		t.Errorf("expected 'how are you?', got %v", messages[2]["content"])
	}
}

func TestResponsesUsageMapping(t *testing.T) {
	chatResp := map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{"content": "test"},
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
		},
	}

	resp, _ := TranslateFromChatCompletions(chatResp, "gpt-4o")

	if resp.Usage.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", resp.Usage.TotalTokens)
	}
}

func TestResponsesErrorFormat(t *testing.T) {
	// 无效 input 应返回错误
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`{invalid}`),
	}

	_, err := TranslateToChatCompletions(req)
	if err == nil {
		t.Error("expected error for invalid input")
	}
}

func TestResponsesIsResponsesRequest(t *testing.T) {
	body := map[string]any{"input": "hello"}
	if !IsResponsesRequest(body) {
		t.Error("expected true for input-based request")
	}

	body2 := map[string]any{"messages": []any{}}
	if IsResponsesRequest(body2) {
		t.Error("expected false for messages-based request")
	}
}

func TestResponsesStreamDone(t *testing.T) {
	out, ok := TranslateStream("data: [DONE]")
	if !ok {
		t.Fatal("expected true for [DONE]")
	}
	if !strings.Contains(out, "response.completed") {
		t.Errorf("expected response.completed, got: %s", out)
	}
}

func TestResponsesMaxOutputTokensMapping(t *testing.T) {
	maxTokens := 1000
	temp := 0.7
	input, _ := json.Marshal("test")
	req := &ResponsesRequest{
		Model:           "gpt-4o",
		Input:           input,
		MaxOutputTokens: &maxTokens,
		Temperature:     &temp,
	}

	chatReq, err := TranslateToChatCompletions(req)
	if err != nil {
		t.Fatalf("TranslateToChatCompletions: %v", err)
	}

	if chatReq["max_tokens"] != 1000 {
		t.Errorf("expected max_tokens 1000, got %v", chatReq["max_tokens"])
	}
	if chatReq["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", chatReq["temperature"])
	}
}

func TestResponsesCreatedEvent(t *testing.T) {
	event := CreateResponseCreatedEvent("gpt-4o")
	if !strings.Contains(event, "response.created") {
		t.Errorf("expected response.created event, got: %s", event)
	}
	if !strings.Contains(event, "gpt-4o") {
		t.Errorf("expected model in event, got: %s", event)
	}
}

func TestResponsesCompletedEvent(t *testing.T) {
	event := CreateResponseCompletedEvent("gpt-4o", 10, 20)
	if !strings.Contains(event, "response.completed") {
		t.Errorf("expected response.completed event, got: %s", event)
	}
	if !strings.Contains(event, "completed") {
		t.Errorf("expected status completed, got: %s", event)
	}
}
