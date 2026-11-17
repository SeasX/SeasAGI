package protocol

// Anthropic Messages → OpenAI Chat Completions 响应翻译（非流式 + SSE 流式）。
//
// 这是该方向的唯一实现：providers 的 Anthropic 执行器与协议注册表
// （Registry 的 OpenAIChat→Anthropic 响应转换）都调用这里，避免两套行为不一致的翻译。
// 行为由 internal/providers/testdata/anthropic_golden 下的金样测试锁定。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// AnthropicResponseToOpenAI 把 Anthropic Messages 非流式响应翻译为 OpenAI chat.completion JSON。
// text 块拼接为 message.content，tool_use 块转为 message.tool_calls；model 写入响应的 model 字段。
func AnthropicResponseToOpenAI(body io.Reader, model string) ([]byte, error) {
	var payload struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}

	textParts := make([]string, 0, len(payload.Content))
	toolCalls := make([]map[string]any, 0)
	for _, part := range payload.Content {
		switch part.Type {
		case "text":
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		case "tool_use":
			// Anthropic tool_use → OpenAI message.tool_calls
			id := part.ID
			if id == "" {
				id = fmt.Sprintf("toolu_%d", time.Now().UnixNano())
			}
			toolCalls = append(toolCalls, map[string]any{
				"id":   id,
				"type": "function",
				"function": map[string]any{
					"name":      part.Name,
					"arguments": anthropicToolArguments(part.Input),
				},
			})
		}
	}

	message := map[string]any{
		"role":    "assistant",
		"content": strings.Join(textParts, ""),
	}
	finishReason := anthropicFinishReason(payload.StopReason)
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		if finishReason == "stop" {
			finishReason = "tool_calls"
		}
	}

	openAIResponse := map[string]any{
		"id":      payload.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": ChatUsage(payload.Usage.InputTokens, payload.Usage.OutputTokens, false),
	}

	return json.Marshal(openAIResponse)
}

// anthropicToolArguments 把 Anthropic tool_use.input 转为 OpenAI arguments 字符串。
func anthropicToolArguments(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return "{}"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	if buf.Len() == 0 {
		return "{}"
	}
	return buf.String()
}

// ChatUsage 构造 OpenAI 风格 usage。estimated 表示 token 数由本地估算得出。
func ChatUsage(inputTokens, outputTokens int, estimated bool) map[string]any {
	usage := map[string]any{
		"prompt_tokens":     inputTokens,
		"completion_tokens": outputTokens,
		"total_tokens":      inputTokens + outputTokens,
	}
	if estimated {
		usage["cost_estimated"] = true
	}
	return usage
}

// AnthropicStreamToOpenAI 把 Anthropic Messages SSE 流翻译为 OpenAI chat.completion.chunk SSE 流。
// 返回的 reader 读完即结束；body 由后台协程负责关闭，上游读取错误会通过 reader 返回给调用方。
func AnthropicStreamToOpenAI(body io.ReadCloser, model string) io.ReadCloser {
	reader, writer := io.Pipe()

	go func() {
		defer body.Close()
		defer writer.Close()

		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		id := fmt.Sprintf("anthropic-%d", time.Now().UnixNano())
		created := time.Now().Unix()
		eventName := ""
		var dataLines []string
		sentRole := false
		finished := false
		inputTokens, outputTokens := 0, 0
		usageSeen := false
		toolCalls := 0
		toolIndexByBlock := map[int]int{}
		var contentBuf strings.Builder

		chunk := func(delta map[string]any, finishReason any) error {
			choice := map[string]any{"index": 0, "delta": delta}
			if finishReason != nil {
				choice["finish_reason"] = finishReason
			}
			return writeChatChunk(writer, map[string]any{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{choice},
			})
		}
		ensureRole := func() error {
			if sentRole {
				return nil
			}
			sentRole = true
			return chunk(map[string]any{"role": "assistant"}, nil)
		}
		finish := func(stopReason string) error {
			if finished {
				return nil
			}
			finished = true
			out := outputTokens
			estimated := false
			if out == 0 && contentBuf.Len() > 0 {
				// 上游未返回 usage 时本地估算，避免计量恒为 0。
				out = EstimateTokens(contentBuf.String())
				estimated = true
			}
			if !usageSeen && (out > 0 || inputTokens > 0) {
				estimated = true
			}
			return chunk(map[string]any{
				"usage": ChatUsage(inputTokens, out, estimated),
			}, anthropicFinishReason(stopReason))
		}

		flush := func() error {
			if len(dataLines) == 0 {
				eventName = ""
				return nil
			}
			data := strings.Join(dataLines, "\n")
			dataLines = nil
			switch eventName {
			case "message_start":
				var payload struct {
					Message struct {
						Usage struct {
							InputTokens  int `json:"input_tokens"`
							OutputTokens int `json:"output_tokens"`
						} `json:"usage"`
					} `json:"message"`
				}
				if json.Unmarshal([]byte(data), &payload) == nil {
					if payload.Message.Usage.InputTokens > 0 {
						inputTokens = payload.Message.Usage.InputTokens
						usageSeen = true
					}
				}
			case "content_block_start":
				var payload struct {
					Index        int `json:"index"`
					ContentBlock struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
				}
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					eventName = ""
					return nil
				}
				// Anthropic 工具块开始 → OpenAI tool_calls 首个增量（携带 id/name）
				if payload.ContentBlock.Type == "tool_use" {
					if err := ensureRole(); err != nil {
						return err
					}
					callID := payload.ContentBlock.ID
					if callID == "" {
						callID = fmt.Sprintf("toolu_%d", time.Now().UnixNano())
					}
					idx := toolCalls
					toolCalls++
					toolIndexByBlock[payload.Index] = idx
					if err := chunk(map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": idx,
								"id":    callID,
								"type":  "function",
								"function": map[string]any{
									"name":      payload.ContentBlock.Name,
									"arguments": "",
								},
							},
						},
					}, nil); err != nil {
						return err
					}
				}
			case "content_block_delta":
				var payload struct {
					Index int `json:"index"`
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				}
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					eventName = ""
					return nil
				}
				switch payload.Delta.Type {
				case "text_delta":
					if payload.Delta.Text == "" {
						break
					}
					if err := ensureRole(); err != nil {
						return err
					}
					contentBuf.WriteString(payload.Delta.Text)
					if err := chunk(map[string]any{"content": payload.Delta.Text}, nil); err != nil {
						return err
					}
				case "input_json_delta":
					// 工具参数增量 → OpenAI tool_calls.function.arguments 增量
					idx, ok := toolIndexByBlock[payload.Index]
					if !ok {
						if err := ensureRole(); err != nil {
							return err
						}
						idx = toolCalls
						toolCalls++
						toolIndexByBlock[payload.Index] = idx
						if err := chunk(map[string]any{
							"tool_calls": []map[string]any{
								{
									"index":    idx,
									"id":       fmt.Sprintf("toolu_%d", time.Now().UnixNano()),
									"type":     "function",
									"function": map[string]any{"name": "", "arguments": ""},
								},
							},
						}, nil); err != nil {
							return err
						}
					}
					contentBuf.WriteString(payload.Delta.PartialJSON)
					if err := chunk(map[string]any{
						"tool_calls": []map[string]any{
							{
								"index":    idx,
								"function": map[string]any{"arguments": payload.Delta.PartialJSON},
							},
						},
					}, nil); err != nil {
						return err
					}
				}
			case "message_delta":
				var payload struct {
					Delta struct {
						StopReason string `json:"stop_reason"`
					} `json:"delta"`
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				}
				if json.Unmarshal([]byte(data), &payload) != nil {
					eventName = ""
					return nil
				}
				if payload.Usage.InputTokens > 0 {
					inputTokens = payload.Usage.InputTokens
					usageSeen = true
				}
				if payload.Usage.OutputTokens > 0 {
					outputTokens = payload.Usage.OutputTokens
					usageSeen = true
				}
				if err := finish(payload.Delta.StopReason); err != nil {
					return err
				}
			case "message_stop":
				if err := finish(""); err != nil {
					return err
				}
				if _, err := io.WriteString(writer, "data: [DONE]\n\n"); err != nil {
					return err
				}
			}
			eventName = ""
			return nil
		}

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				if err := flush(); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
				continue
			}
			if value, ok := strings.CutPrefix(line, "event:"); ok {
				eventName = strings.TrimSpace(value)
				continue
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				dataLines = append(dataLines, strings.TrimSpace(value))
			}
		}

		if err := scanner.Err(); err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		if err := finish(""); err != nil {
			_ = writer.CloseWithError(err)
		}
	}()

	return reader
}

// writeChatChunk 以 SSE data 行写出一个 OpenAI chat.completion.chunk。
func writeChatChunk(w io.Writer, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

// anthropicFinishReason 把 Anthropic stop_reason 映射为 OpenAI finish_reason。
func anthropicFinishReason(stopReason string) string {
	switch stopReason {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}
