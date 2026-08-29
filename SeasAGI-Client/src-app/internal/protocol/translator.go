package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type CanonicalRequest struct {
	Model        string
	Messages     []map[string]any
	Stream       bool
	Extra        map[string]any
	SourceFormat Format
}

type Translator interface {
	Match(body map[string]any) bool
	ToCanonical(body map[string]any) (*CanonicalRequest, error)
	SourceFormat() Format
}

var translators = []Translator{
	openAIChatTranslator{},
	openAIResponsesTranslator{},
	GeminiTranslator{},
	VertexTranslator{},
	anthropicTranslator{},
	passthroughTranslator{},
}

func ParseRequest(body []byte) (*CanonicalRequest, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid request body")
	}

	for _, translator := range translators {
		if translator.Match(payload) {
			return translator.ToCanonical(payload)
		}
	}

	return nil, fmt.Errorf("unsupported request format")
}

type openAIChatTranslator struct{}

func (openAIChatTranslator) Match(body map[string]any) bool {
	_, hasMessages := body["messages"]
	return hasMessages
}

func (openAIChatTranslator) SourceFormat() Format { return FormatOpenAIChat }

func (openAIChatTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	messages, err := normalizeMessages(body["messages"])
	if err != nil {
		return nil, err
	}

	req := &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "messages", "stream"),
		SourceFormat: FormatOpenAIChat,
	}
	return req, nil
}

type openAIResponsesTranslator struct{}

func (openAIResponsesTranslator) Match(body map[string]any) bool {
	_, hasInput := body["input"]
	return hasInput
}

func (openAIResponsesTranslator) SourceFormat() Format { return FormatOpenAIResponses }

func (openAIResponsesTranslator) ToCanonical(body map[string]any) (*CanonicalRequest, error) {
	model, _ := body["model"].(string)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	input, ok := body["input"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("input must be an array")
	}

	messages := make([]map[string]any, 0, len(input))
	for _, item := range input {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}

		role, _ := msg["role"].(string)
		if role == "" {
			role = "user"
		}

		switch content := msg["content"].(type) {
		case string:
			messages = append(messages, map[string]any{
				"role":    role,
				"content": content,
			})
		case []interface{}:
			text := collapseContent(content)
			if text == "" {
				continue
			}
			messages = append(messages, map[string]any{
				"role":    role,
				"content": text,
			})
		default:
			if text, ok := msg["text"].(string); ok && text != "" {
				messages = append(messages, map[string]any{
					"role":    role,
					"content": text,
				})
			}
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("input is empty")
	}

	req := &CanonicalRequest{
		Model:        model,
		Messages:     messages,
		Stream:       asBool(body["stream"]),
		Extra:        collectExtra(body, "model", "input", "stream"),
		SourceFormat: FormatOpenAIResponses,
	}
	return req, nil
}

func normalizeMessages(value any) ([]map[string]any, error) {
	items, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("messages must be an array")
	}

	messages := make([]map[string]any, 0, len(items))
	for _, item := range items {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		messages = append(messages, msg)
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages is empty")
	}
	return messages, nil
}

func collapseContent(parts []interface{}) string {
	text := ""
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := part["text"].(string); ok {
			text += value
		}
	}
	return text
}

func collectExtra(body map[string]any, ignoredKeys ...string) map[string]any {
	ignored := make(map[string]struct{}, len(ignoredKeys))
	for _, key := range ignoredKeys {
		ignored[key] = struct{}{}
	}

	extra := map[string]any{}
	for key, value := range body {
		if _, ignoredKey := ignored[key]; ignoredKey {
			continue
		}
		extra[key] = value
	}
	return extra
}

func asBool(value any) bool {
	boolean, _ := value.(bool)
	return boolean
}

func ToAnthropicMessages(messages []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role == "system" {
			role = "user"
		}

		content := normalizeAnthropicContent(message["content"])
		if content == nil {
			continue
		}

		result = append(result, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	return result
}

func normalizeAnthropicContent(value any) any {
	switch content := value.(type) {
	case string:
		trimmed := strings.TrimSpace(content)
		if trimmed == "" {
			return nil
		}
		return trimmed
	case []any:
		blocks := make([]map[string]any, 0, len(content))
		for _, raw := range content {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			partType, _ := part["type"].(string)
			if partType == "" || partType == "text" {
				text, _ := part["text"].(string)
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
		}
		if len(blocks) == 0 {
			return nil
		}
		return blocks
	default:
		return nil
	}
}
