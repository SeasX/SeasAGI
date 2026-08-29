package protocol

import (
	"encoding/json"
	"testing"
)

func TestParseRequest_openAIChatFormat(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SourceFormat != FormatOpenAIChat {
		t.Errorf("expected SourceFormat %q, got %q", FormatOpenAIChat, req.SourceFormat)
	}
	if req.Model != "gpt-4" {
		t.Errorf("expected Model %q, got %q", "gpt-4", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "user" {
		t.Errorf("expected role 'user', got %v", req.Messages[0]["role"])
	}
	if req.Messages[0]["content"] != "hello" {
		t.Errorf("expected content 'hello', got %v", req.Messages[0]["content"])
	}
}

func TestParseRequest_openAIResponsesFormat(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{"role": "user", "content": "hi"},
		},
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.SourceFormat != FormatOpenAIResponses {
		t.Errorf("expected SourceFormat %q, got %q", FormatOpenAIResponses, req.SourceFormat)
	}
	if req.Model != "gpt-4o" {
		t.Errorf("expected Model %q, got %q", "gpt-4o", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "user" {
		t.Errorf("expected role 'user', got %v", req.Messages[0]["role"])
	}
	if req.Messages[0]["content"] != "hi" {
		t.Errorf("expected content 'hi', got %v", req.Messages[0]["content"])
	}
}

func TestParseRequest_unknownFormat(t *testing.T) {
	// None of the translators match: no "messages", "input", "contents", "instances", or "endpoint"
	body := map[string]any{
		"unknown_key": "value",
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
	if err.Error() != "unsupported request format" {
		t.Errorf("expected 'unsupported request format', got %q", err.Error())
	}
}

func TestParseRequest_streamingFlag(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-4",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		"stream":   true,
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !req.Stream {
		t.Error("expected Stream to be true")
	}
}

func TestParseRequest_streamingFlagFalse(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-4",
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
		"stream":   false,
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Stream {
		t.Error("expected Stream to be false")
	}
}

func TestParseRequest_extraFields(t *testing.T) {
	body := map[string]any{
		"model":      "gpt-4",
		"messages":   []any{map[string]any{"role": "user", "content": "hello"}},
		"temperature": 0.7,
		"max_tokens":  100,
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Extra == nil {
		t.Fatal("expected Extra to be non-nil")
	}
	if temp, ok := req.Extra["temperature"]; !ok {
		t.Error("expected Extra to contain 'temperature'")
	} else if temp != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", temp)
	}
	if maxTokens, ok := req.Extra["max_tokens"]; !ok {
		t.Error("expected Extra to contain 'max_tokens'")
	} else if maxTokens != float64(100) {
		t.Errorf("expected max_tokens 100, got %v", maxTokens)
	}
	// Extra should not contain the ignored keys
	if _, ok := req.Extra["model"]; ok {
		t.Error("Extra should not contain 'model'")
	}
	if _, ok := req.Extra["messages"]; ok {
		t.Error("Extra should not contain 'messages'")
	}
	if _, ok := req.Extra["stream"]; ok {
		t.Error("Extra should not contain 'stream'")
	}
}

func TestParseRequest_invalidJSON(t *testing.T) {
	_, err := ParseRequest([]byte(`{invalid json}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if err.Error() != "invalid request body" {
		t.Errorf("expected 'invalid request body', got %q", err.Error())
	}
}

func TestToAnthropicMessages_simpleMessages(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "world"},
	}

	result := ToAnthropicMessages(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0]["role"] != "user" {
		t.Errorf("expected role 'user', got %v", result[0]["role"])
	}
	if result[0]["content"] != "hello" {
		t.Errorf("expected content 'hello', got %v", result[0]["content"])
	}
	if result[1]["role"] != "assistant" {
		t.Errorf("expected role 'assistant', got %v", result[1]["role"])
	}
	if result[1]["content"] != "world" {
		t.Errorf("expected content 'world', got %v", result[1]["content"])
	}
}

func TestToAnthropicMessages_systemRoleConvertsToUser(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "you are a helpful assistant"},
		{"role": "user", "content": "hello"},
	}

	result := ToAnthropicMessages(messages)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	// system role should be converted to user
	if result[0]["role"] != "user" {
		t.Errorf("expected role 'user' (converted from system), got %v", result[0]["role"])
	}
	if result[0]["content"] != "you are a helpful assistant" {
		t.Errorf("expected content 'you are a helpful assistant', got %v", result[0]["content"])
	}
	if result[1]["role"] != "user" {
		t.Errorf("expected role 'user', got %v", result[1]["role"])
	}
}

func TestToAnthropicMessages_contentArrays(t *testing.T) {
	messages := []map[string]any{
		{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
				map[string]any{"type": "text", "text": "world"},
			},
		},
	}

	result := ToAnthropicMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	content, ok := result[0]["content"].([]map[string]any)
	if !ok {
		t.Fatalf("expected content to be []map[string]any, got %T", result[0]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(content))
	}
	if content[0]["type"] != "text" || content[0]["text"] != "hello" {
		t.Errorf("expected first block {text: hello}, got %v", content[0])
	}
	if content[1]["type"] != "text" || content[1]["text"] != "world" {
		t.Errorf("expected second block {text: world}, got %v", content[1])
	}
}

func TestToAnthropicMessages_emptyContent(t *testing.T) {
	messages := []map[string]any{
		{"role": "user", "content": ""},
		{"role": "user", "content": "   "},
		{"role": "user", "content": "valid"},
	}

	result := ToAnthropicMessages(messages)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (empty/whitespace content filtered out), got %d", len(result))
	}
	if result[0]["content"] != "valid" {
		t.Errorf("expected content 'valid', got %v", result[0]["content"])
	}
}

func TestToAnthropicMessages_emptyContentArray(t *testing.T) {
	messages := []map[string]any{
		{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": ""},
				map[string]any{"type": "text", "text": "   "},
			},
		},
	}

	result := ToAnthropicMessages(messages)

	if len(result) != 0 {
		t.Fatalf("expected 0 messages (all content blocks filtered out), got %d", len(result))
	}
}

func TestNormalizeAnthropicContent_stringContent(t *testing.T) {
	result := normalizeAnthropicContent("hello world")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %v", result)
	}
}

func TestNormalizeAnthropicContent_stringContentTrimmed(t *testing.T) {
	result := normalizeAnthropicContent("  hello  ")
	if result != "hello" {
		t.Errorf("expected 'hello', got %v", result)
	}
}

func TestNormalizeAnthropicContent_emptyString(t *testing.T) {
	result := normalizeAnthropicContent("")
	if result != nil {
		t.Errorf("expected nil for empty string, got %v", result)
	}
}

func TestNormalizeAnthropicContent_whitespaceString(t *testing.T) {
	result := normalizeAnthropicContent("   ")
	if result != nil {
		t.Errorf("expected nil for whitespace string, got %v", result)
	}
}

func TestNormalizeAnthropicContent_contentArray(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "text", "text": "world"},
	}

	result := normalizeAnthropicContent(content)

	blocks, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0]["type"] != "text" || blocks[0]["text"] != "hello" {
		t.Errorf("expected first block {text: hello}, got %v", blocks[0])
	}
	if blocks[1]["type"] != "text" || blocks[1]["text"] != "world" {
		t.Errorf("expected second block {text: world}, got %v", blocks[1])
	}
}

func TestNormalizeAnthropicContent_contentArrayWithEmptyBlocks(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "hello"},
		map[string]any{"type": "text", "text": ""},
		map[string]any{"type": "text", "text": "world"},
	}

	result := normalizeAnthropicContent(content)

	blocks, ok := result.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", result)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks (empty block filtered), got %d", len(blocks))
	}
	if blocks[0]["text"] != "hello" {
		t.Errorf("expected first block text 'hello', got %v", blocks[0]["text"])
	}
	if blocks[1]["text"] != "world" {
		t.Errorf("expected second block text 'world', got %v", blocks[1]["text"])
	}
}

func TestNormalizeAnthropicContent_contentArrayAllEmpty(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": ""},
	}

	result := normalizeAnthropicContent(content)

	if result != nil {
		t.Errorf("expected nil for all-empty content array, got %v", result)
	}
}

func TestNormalizeAnthropicContent_nonStringNonArray(t *testing.T) {
	result := normalizeAnthropicContent(123)
	if result != nil {
		t.Errorf("expected nil for non-string non-array, got %v", result)
	}
}

func TestCollapseContent_mixedContentTypes(t *testing.T) {
	parts := []any{
		map[string]any{"type": "text", "text": "hello "},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "..."}},
		map[string]any{"text": "world"},
	}

	result := collapseContent(parts)

	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestCollapseContent_noTextParts(t *testing.T) {
	parts := []any{
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": "..."}},
	}

	result := collapseContent(parts)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestCollapseContent_nonMapParts(t *testing.T) {
	parts := []any{
		"string part",
		42,
		map[string]any{"text": "valid"},
	}

	result := collapseContent(parts)

	if result != "valid" {
		t.Errorf("expected 'valid', got %q", result)
	}
}

func TestCollapseContent_emptyParts(t *testing.T) {
	parts := []any{}

	result := collapseContent(parts)

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestParseRequest_openAIChatMissingModel(t *testing.T) {
	body := map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
	if err.Error() != "model is required" {
		t.Errorf("expected 'model is required', got %q", err.Error())
	}
}

func TestParseRequest_openAIResponsesMissingModel(t *testing.T) {
	body := map[string]any{
		"input": []any{map[string]any{"role": "user", "content": "hello"}},
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for missing model, got nil")
	}
	if err.Error() != "model is required" {
		t.Errorf("expected 'model is required', got %q", err.Error())
	}
}

func TestParseRequest_openAIResponsesEmptyInput(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{},
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if err.Error() != "input is empty" {
		t.Errorf("expected 'input is empty', got %q", err.Error())
	}
}

func TestParseRequest_openAIResponsesInputNotArray(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": "not an array",
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for non-array input, got nil")
	}
	if err.Error() != "input must be an array" {
		t.Errorf("expected 'input must be an array', got %q", err.Error())
	}
}

func TestParseRequest_openAIResponsesDefaultRole(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{"content": "no role"},
		},
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["role"] != "user" {
		t.Errorf("expected default role 'user', got %v", req.Messages[0]["role"])
	}
}

func TestParseRequest_openAIResponsesTextFallback(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{"role": "user", "text": "text field"},
		},
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["content"] != "text field" {
		t.Errorf("expected content 'text field', got %v", req.Messages[0]["content"])
	}
}

func TestParseRequest_openAIResponsesContentArray(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "part1 "},
					map[string]any{"type": "text", "text": "part2"},
				},
			},
		},
	}
	data, _ := json.Marshal(body)

	req, err := ParseRequest(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(req.Messages))
	}
	if req.Messages[0]["content"] != "part1 part2" {
		t.Errorf("expected content 'part1 part2', got %v", req.Messages[0]["content"])
	}
}

func TestParseRequest_messagesNotArray(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-4",
		"messages": "not an array",
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for non-array messages, got nil")
	}
	if err.Error() != "messages must be an array" {
		t.Errorf("expected 'messages must be an array', got %q", err.Error())
	}
}

func TestParseRequest_emptyMessages(t *testing.T) {
	body := map[string]any{
		"model":    "gpt-4",
		"messages": []any{},
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for empty messages, got nil")
	}
	if err.Error() != "messages is empty" {
		t.Errorf("expected 'messages is empty', got %q", err.Error())
	}
}

func TestParseRequest_openAIResponsesMixedContentArrayAllEmpty(t *testing.T) {
	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{
				"role":    "user",
				"content": []any{},
			},
		},
	}
	data, _ := json.Marshal(body)

	_, err := ParseRequest(data)
	if err == nil {
		t.Fatal("expected error for all-empty content, got nil")
	}
	if err.Error() != "input is empty" {
		t.Errorf("expected 'input is empty', got %q", err.Error())
	}
}