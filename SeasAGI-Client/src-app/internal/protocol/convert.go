package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// ConvertOpenAIResponseToSource 把上游返回的 OpenAI Chat Completions 响应体（非流式）
// 转换为请求方所期望的源格式。网关内部统一用 OpenAI Chat 与上游交互，因此当客户端
// 使用 Anthropic Messages 或 OpenAI Responses 协议时，需要在此转换回去。
func ConvertOpenAIResponseToSource(body []byte, sourceFormat Format, model string) ([]byte, error) {
	if sourceFormat == FormatOpenAIChat || len(body) == 0 {
		return body, nil
	}
	var chat map[string]any
	if err := json.Unmarshal(body, &chat); err != nil {
		return body, err // 非 JSON（错误体等）原样透传
	}
	switch sourceFormat {
	case FormatAnthropic:
		out, err := json.Marshal(convertOpenAIToAnthropic(chat, model))
		if err != nil {
			return body, err
		}
		return out, nil
	case FormatOpenAIResponses:
		return TranslateFromChatCompletionsBytes(body, model)
	default:
		return body, nil
	}
}

// ConvertStreamToSource 把上游的 OpenAI Chat Completions SSE 流转换为源格式的 SSE 流。
// 返回的 ReadCloser 会在消费完毕或关闭时结束底层转换 goroutine。
func ConvertStreamToSource(r io.Reader, sourceFormat Format, model string) io.ReadCloser {
	if sourceFormat == FormatOpenAIChat {
		return io.NopCloser(r)
	}
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		switch sourceFormat {
		case FormatAnthropic:
			writeAnthropicStream(pw, r, model)
		case FormatOpenAIResponses:
			writeResponsesStream(pw, r, model)
		default:
			_, _ = io.Copy(pw, r)
		}
	}()
	return pr
}

// --- Anthropic (Messages API) ---

func convertOpenAIToAnthropic(chat map[string]any, model string) map[string]any {
	id, _ := chat["id"].(string)
	if id == "" {
		id = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if m, ok := chat["model"].(string); ok && m != "" {
		model = m
	}

	content := make([]map[string]any, 0, 1)
	stopReason := "end_turn"
	hasToolUse := false

	if choices, ok := chat["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if text, ok := msg["content"].(string); ok && text != "" {
					content = append(content, map[string]any{"type": "text", "text": text})
				}
				if tcs, ok := msg["tool_calls"].([]any); ok {
					for _, raw := range tcs {
						tc, _ := raw.(map[string]any)
						if tc == nil {
							continue
						}
						fn, _ := tc["function"].(map[string]any)
						name, _ := fn["name"].(string)
						argsStr, _ := fn["arguments"].(string)
						input := any(map[string]any{})
						if argsStr != "" {
							var parsed any
							if json.Unmarshal([]byte(argsStr), &parsed) == nil {
								input = parsed
							}
						}
						content = append(content, map[string]any{
							"type":  "tool_use",
							"id":    tc["id"],
							"name":  name,
							"input": input,
						})
						hasToolUse = true
					}
				}
			}
			if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
				stopReason = anthropicStopReason(fr, hasToolUse)
			}
		}
	}

	usage, _ := chat["usage"].(map[string]any)
	in := int64(toFloat(usage["prompt_tokens"]))
	out := int64(toFloat(usage["completion_tokens"]))

	return map[string]any{
		"id":            id,
		"type":          "message",
		"role":          "assistant",
		"model":         model,
		"content":       content,
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  in,
			"output_tokens": out,
		},
	}
}

func anthropicStopReason(openAIFinish string, hasToolUse bool) string {
	if hasToolUse {
		return "tool_use"
	}
	switch openAIFinish {
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "stop":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// anthropicStreamWriter 把 OpenAI chat SSE 增量转换成 Anthropic Messages SSE。
type anthropicStreamWriter struct {
	w           io.Writer
	id          string
	model       string
	messageSent bool
	nextIndex   int
	// 当前打开的 content block
	blockOpen  bool
	blockType  string // "text" | "tool_use"
	toolIndex  int    // OpenAI tool_calls[index]
	stopReason string
	usageIn    int64
	usageOut   int64
}

func writeAnthropicStream(w io.Writer, r io.Reader, model string) {
	s := &anthropicStreamWriter{w: w, model: model}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			s.finish()
			return
		}
		var chunk map[string]any
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		s.handleChunk(chunk)
	}
	s.finish()
}

func (s *anthropicStreamWriter) event(name string, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, data)
}

func (s *anthropicStreamWriter) ensureMessageStart(chunk map[string]any) {
	if s.messageSent {
		return
	}
	if id, ok := chunk["id"].(string); ok && id != "" {
		s.id = id
	}
	if s.id == "" {
		s.id = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if m, ok := chunk["model"].(string); ok && m != "" {
		s.model = m
	}
	s.event("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            s.id,
			"type":          "message",
			"role":          "assistant",
			"model":         s.model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 0, "output_tokens": 0},
		},
	})
	s.messageSent = true
}

func (s *anthropicStreamWriter) closeBlock() {
	if !s.blockOpen {
		return
	}
	s.event("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": s.nextIndex - 1,
	})
	s.blockOpen = false
}

func (s *anthropicStreamWriter) handleChunk(chunk map[string]any) {
	s.ensureMessageStart(chunk)
	if u, ok := chunk["usage"].(map[string]any); ok {
		if v := int64(toFloat(u["prompt_tokens"])); v > 0 {
			s.usageIn = v
		}
		if v := int64(toFloat(u["completion_tokens"])); v > 0 {
			s.usageOut = v
		}
	}

	choices, _ := chunk["choices"].([]any)
	if len(choices) == 0 {
		return
	}
	choice, _ := choices[0].(map[string]any)
	if choice == nil {
		return
	}
	if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
		s.stopReason = fr
	}
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return
	}

	// 文本增量
	if text, ok := delta["content"].(string); ok && text != "" {
		if !s.blockOpen || s.blockType != "text" {
			s.closeBlock()
			s.event("content_block_start", map[string]any{
				"type":          "content_block_start",
				"index":         s.nextIndex,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			s.nextIndex++
			s.blockOpen = true
			s.blockType = "text"
		}
		s.event("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": s.nextIndex - 1,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})
	}

	// 工具调用增量
	if tcs, ok := delta["tool_calls"].([]any); ok {
		for _, raw := range tcs {
			tc, _ := raw.(map[string]any)
			if tc == nil {
				continue
			}
			idx := int(toFloat(tc["index"]))
			if !s.blockOpen || s.blockType != "tool_use" || idx != s.toolIndex {
				s.closeBlock()
				fn, _ := tc["function"].(map[string]any)
				name, _ := fn["name"].(string)
				id, _ := tc["id"].(string)
				if id == "" {
					id = fmt.Sprintf("toolu_%d", time.Now().UnixNano())
				}
				s.event("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": s.nextIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    id,
						"name":  name,
						"input": map[string]any{},
					},
				})
				s.nextIndex++
				s.blockOpen = true
				s.blockType = "tool_use"
				s.toolIndex = idx
			}
			fn, _ := tc["function"].(map[string]any)
			if args, ok := fn["arguments"].(string); ok && args != "" {
				s.event("content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": s.nextIndex - 1,
					"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
				})
			}
		}
	}
}

func (s *anthropicStreamWriter) finish() {
	s.ensureMessageStart(nil)
	s.closeBlock()
	s.event("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   anthropicStopReason(s.stopReason, s.stopReason == "tool_calls"),
			"stop_sequence": nil,
		},
		"usage": map[string]any{"output_tokens": s.usageOut},
	})
	s.event("message_stop", map[string]any{"type": "message_stop"})
}

// --- OpenAI Responses API ---

func writeResponsesStream(w io.Writer, r io.Reader, model string) {
	_, _ = io.WriteString(w, CreateResponseCreatedEvent(model))
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var outTokens int
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == "data: [DONE]" {
			_, _ = io.WriteString(w, CreateResponseCompletedEvent(model, 0, outTokens))
			return
		}
		if converted, ok := TranslateStream(line); ok {
			_, _ = io.WriteString(w, converted)
		}
	}
	_, _ = io.WriteString(w, CreateResponseCompletedEvent(model, 0, outTokens))
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}
