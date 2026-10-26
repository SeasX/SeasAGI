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

// ParseRequestAs 按指定的源格式解析请求体，跳过自动探测。用于网关为特定协议路径
// （如 /v1/messages、/v1/responses）注册的专用路由——这些路径的 wire 格式已知，
// 自动探测可能因 "messages" 字段把 Anthropic 请求误判为 OpenAI Chat。
func ParseRequestAs(body []byte, format Format) (*CanonicalRequest, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid request body")
	}
	switch format {
	case FormatAnthropic:
		return anthropicTranslator{}.ToCanonical(payload)
	case FormatOpenAIResponses:
		// Responses API 的 input 允许为字符串；归一为数组后再解析。
		if s, ok := payload["input"].(string); ok {
			payload["input"] = []any{map[string]any{"role": "user", "content": s}}
		}
		return openAIResponsesTranslator{}.ToCanonical(payload)
	case FormatOpenAIChat:
		return openAIChatTranslator{}.ToCanonical(payload)
	case FormatGemini:
		return GeminiTranslator{}.ToCanonical(payload)
	default:
		return ParseRequest(body)
	}
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

// ToAnthropicMessages 把 OpenAI Chat 消息转换为 Anthropic Messages。
// 除文本外还完成工具调用双向映射：
//   - assistant.tool_calls → assistant content 中的 tool_use 块
//   - role=tool（tool_call_id + content）→ user content 中的 tool_result 块
func ToAnthropicMessages(messages []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		role, _ := message["role"].(string)
		switch role {
		case "system":
			if content := normalizeAnthropicContent(message["content"]); content != nil {
				result = append(result, map[string]any{"role": "user", "content": content})
			}
		case "tool":
			// OpenAI 工具结果 → Anthropic tool_result；连续工具结果合并到同一条 user 消息。
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": firstNonEmptyString(message["tool_call_id"], message["tool_use_id"]),
				"content":     anthropicToolResultContent(message["content"]),
			}
			appendAnthropicBlock(&result, "user", block)
		case "assistant":
			toolCalls := toAnySlice(message["tool_calls"])
			// 纯文本回复保持字符串形式，避免无谓改变上游契约。
			if len(toolCalls) == 0 {
				if text, ok := message["content"].(string); ok {
					if trimmed := strings.TrimSpace(text); trimmed != "" {
						result = append(result, map[string]any{"role": "assistant", "content": trimmed})
					}
					continue
				}
			}
			blocks := anthropicAssistantBlocks(message["content"])
			for _, raw := range toolCalls {
				tc, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				fn, _ := tc["function"].(map[string]any)
				name, _ := fn["name"].(string)
				if name == "" {
					continue
				}
				id, _ := tc["id"].(string)
				if id == "" {
					id = fmt.Sprintf("toolu_%s", name)
				}
				args, _ := fn["arguments"].(string)
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    id,
					"name":  name,
					"input": parseAnthropicToolInput(args),
				})
			}
			if len(blocks) > 0 {
				result = append(result, map[string]any{"role": "assistant", "content": blocks})
			}
		default:
			content := normalizeAnthropicContent(message["content"])
			if content == nil {
				continue
			}
			result = append(result, map[string]any{
				"role":    role,
				"content": content,
			})
		}
	}
	return result
}

// appendAnthropicBlock 追加内容块；若上一条消息角色相同且内容为块数组，则直接合并，
// 避免 Anthropic 对连续同角色消息的额外约束。
func appendAnthropicBlock(result *[]map[string]any, role string, block map[string]any) {
	if len(*result) > 0 {
		last := (*result)[len(*result)-1]
		if lastRole, _ := last["role"].(string); lastRole == role {
			if blocks, ok := last["content"].([]map[string]any); ok {
				last["content"] = append(blocks, block)
				return
			}
		}
	}
	*result = append(*result, map[string]any{
		"role":    role,
		"content": []map[string]any{block},
	})
}

// anthropicAssistantBlocks 提取 assistant 消息内容块，保留已有的 tool_use 块。
func anthropicAssistantBlocks(content any) []map[string]any {
	blocks := make([]map[string]any, 0, 1)
	switch value := content.(type) {
	case string:
		if text := strings.TrimSpace(value); text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": text})
		}
	case []any:
		for _, raw := range value {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			blocks = append(blocks, normalizeAnthropicBlock(part)...)
		}
	case []map[string]any:
		for _, part := range value {
			blocks = append(blocks, normalizeAnthropicBlock(part)...)
		}
	}
	return blocks
}

func normalizeAnthropicBlock(part map[string]any) []map[string]any {
	partType, _ := part["type"].(string)
	switch partType {
	case "tool_use":
		// 已是 Anthropic 格式，原样保留
		return []map[string]any{part}
	case "", "text":
		text, _ := part["text"].(string)
		if text = strings.TrimSpace(text); text == "" {
			return nil
		}
		return []map[string]any{{"type": "text", "text": text}}
	default:
		return nil
	}
}

// anthropicToolResultContent 归一工具结果内容：字符串/块数组直接透传，其余序列化为 JSON。
func anthropicToolResultContent(value any) any {
	switch content := value.(type) {
	case nil:
		return ""
	case string:
		return content
	case []any:
		return content
	case []map[string]any:
		return content
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

// parseAnthropicToolInput 把 OpenAI arguments JSON 字符串解析为 Anthropic tool_use.input 对象。
func parseAnthropicToolInput(args string) any {
	if strings.TrimSpace(args) == "" {
		return map[string]any{}
	}
	var parsed any
	if err := json.Unmarshal([]byte(args), &parsed); err == nil {
		if parsed == nil {
			return map[string]any{}
		}
		return parsed
	}
	return map[string]any{}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	return ""
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
