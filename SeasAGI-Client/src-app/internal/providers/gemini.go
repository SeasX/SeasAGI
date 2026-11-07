package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
)

type GeminiExecutor struct{}

func (g *GeminiExecutor) Name() string {
	return "gemini"
}

func (g *GeminiExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := sharedHTTPClient

	geminiBody := protocol.CanonicalToGemini(&protocol.CanonicalRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Extra:    req.Extra,
	})

	bodyReader, contentType, err := encodeJSONBody(geminiBody)
	if err != nil {
		return nil, err
	}

	apiKey := config.APIKey
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	// 密钥通过请求头传递，禁止拼进 URL（避免出现在日志/代理记录中）。
	action := "generateContent"
	query := ""
	if req.Stream {
		action = "streamGenerateContent"
		query = "?alt=sse"
	}
	endpoint := fmt.Sprintf("%s/v1/models/%s:%s%s",
		strings.TrimRight(baseURL, "/"), req.Model, action, query)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	if apiKey != "" {
		httpReq.Header.Set("x-goog-api-key", apiKey)
	}
	forwardProviderHeaders(httpReq, config.ProviderSpecificConfig, "header.")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, classifyStatusError(resp.StatusCode, resp.Body)
	}

	if req.Stream {
		translatedBody, headers := translateGeminiStream(resp.Body, req.Model)
		return &UpstreamResponse{
			Status:  http.StatusOK,
			Headers: headers,
			Body:    translatedBody,
		}, nil
	}

	translatedBody, headers, err := translateGeminiResponse(resp.Body, req.Model)
	if err != nil {
		resp.Body.Close()
		return nil, err
	}
	resp.Body.Close()

	return &UpstreamResponse{
		Status:  http.StatusOK,
		Headers: headers,
		Body:    translatedBody,
	}, nil
}

func (g *GeminiExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := metadataHTTPClient

	apiKey := config.APIKey
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/models", strings.TrimRight(baseURL, "/")), nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("x-goog-api-key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, classifyStatusError(resp.StatusCode, resp.Body)
	}

	var payload struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return []ModelInfo{}, nil
	}

	result := make([]ModelInfo, 0, len(payload.Models))
	for _, m := range payload.Models {
		name := strings.TrimPrefix(m.Name, "models/")
		result = append(result, ModelInfo{
			ID:      name,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "google",
		})
	}
	return result, nil
}

func (g *GeminiExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}

func translateGeminiResponse(body io.ReadCloser, model string) (io.ReadCloser, http.Header, error) {
	var payload struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		return nil, nil, err
	}

	var textBuilder strings.Builder
	if len(payload.Candidates) > 0 {
		for _, part := range payload.Candidates[0].Content.Parts {
			textBuilder.WriteString(part.Text)
		}
	}
	text := textBuilder.String()

	finishReason := "stop"
	if len(payload.Candidates) > 0 {
		finishReason = geminiFinishReason(payload.Candidates[0].FinishReason)
	}

	openAIResp := map[string]any{
		"id":      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     payload.UsageMetadata.PromptTokenCount,
			"completion_tokens": payload.UsageMetadata.CandidatesTokenCount,
			"total_tokens":      payload.UsageMetadata.TotalTokenCount,
		},
	}

	data, err := json.Marshal(openAIResp)
	if err != nil {
		return nil, nil, err
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return io.NopCloser(strings.NewReader(string(data))), headers, nil
}

// geminiFinishReason 将 Gemini finishReason 映射为 OpenAI finish_reason。
func geminiFinishReason(reason string) string {
	switch reason {
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT":
		return "content_filter"
	default:
		return "stop"
	}
}

// translateGeminiStream 将 Gemini（generateContent / streamGenerateContent?alt=sse）
// 流式响应转换为 OpenAI Chat SSE，并累计 usage；上游缺失 usage 时本地估算并标记。
func translateGeminiStream(body io.ReadCloser, model string) (io.ReadCloser, http.Header) {
	reader, writer := io.Pipe()
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")

	go func() {
		defer body.Close()
		defer writer.Close()

		id := fmt.Sprintf("gemini-%d", time.Now().UnixNano())
		created := time.Now().Unix()
		scanner := bufio.NewScanner(body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		sentRole := false
		finishReason := "stop"
		inputTokens, outputTokens := 0, 0
		usageSeen := false
		var textBuf strings.Builder

		writeRole := func() {
			if sentRole {
				return
			}
			sentRole = true
			_ = writeSSEChunk(writer, map[string]any{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   model,
				"choices": []map[string]any{
					{"index": 0, "delta": map[string]any{"role": "assistant"}},
				},
			})
		}

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || line == "data: [DONE]" {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if line == "" || line == "[" || line == "]" {
				continue
			}
			line = strings.TrimSuffix(line, ",")
			var payload struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
				UsageMetadata struct {
					PromptTokenCount     int `json:"promptTokenCount"`
					CandidatesTokenCount int `json:"candidatesTokenCount"`
					TotalTokenCount      int `json:"totalTokenCount"`
				} `json:"usageMetadata"`
			}
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				continue
			}

			writeRole()

			if payload.UsageMetadata.PromptTokenCount > 0 {
				inputTokens = payload.UsageMetadata.PromptTokenCount
				usageSeen = true
			}
			if payload.UsageMetadata.CandidatesTokenCount > 0 {
				outputTokens = payload.UsageMetadata.CandidatesTokenCount
				usageSeen = true
			}

			if len(payload.Candidates) > 0 {
				if payload.Candidates[0].FinishReason != "" {
					finishReason = geminiFinishReason(payload.Candidates[0].FinishReason)
				}
				var textBuilder strings.Builder
				for _, part := range payload.Candidates[0].Content.Parts {
					textBuilder.WriteString(part.Text)
				}
				if text := textBuilder.String(); text != "" {
					textBuf.WriteString(text)
					_ = writeSSEChunk(writer, map[string]any{
						"id":      id,
						"object":  "chat.completion.chunk",
						"created": created,
						"model":   model,
						"choices": []map[string]any{
							{"index": 0, "delta": map[string]any{"content": text}},
						},
					})
				}
			}
		}

		// 收尾：输出累计 usage（缺失时本地估算），随后结束流。
		outTokens := outputTokens
		estimated := false
		if outTokens <= 0 && textBuf.Len() > 0 {
			outTokens = protocol.EstimateTokens(textBuf.String())
			estimated = true
		}
		if !usageSeen && outTokens > 0 {
			estimated = true
		}
		_ = writeSSEChunk(writer, map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": created,
			"model":   model,
			"choices": []map[string]any{
				{"index": 0, "delta": map[string]any{}, "finish_reason": finishReason},
			},
			"usage": chatUsage(inputTokens, outTokens, estimated),
		})
		_, _ = io.WriteString(writer, "data: [DONE]\n\n")
	}()

	return reader, headers
}
