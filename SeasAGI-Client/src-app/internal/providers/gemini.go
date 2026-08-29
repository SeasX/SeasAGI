package providers

import (
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
	client := &http.Client{Timeout: 120 * time.Second}

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

	action := "generateContent"
	if req.Stream {
		action = "streamGenerateContent"
	}

	apiKey := config.APIKey
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	endpoint := fmt.Sprintf("%s/v1/models/%s:%s?key=%s",
		strings.TrimRight(baseURL, "/"), req.Model, action, apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
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
	client := &http.Client{Timeout: 30 * time.Second}

	apiKey := config.APIKey
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/models?key=%s", strings.TrimRight(baseURL, "/"), apiKey), nil)
	if err != nil {
		return nil, err
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
			Name       string `json:"name"`
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

	text := ""
	if len(payload.Candidates) > 0 && len(payload.Candidates[0].Content.Parts) > 0 {
		text = payload.Candidates[0].Content.Parts[0].Text
	}

	finishReason := "stop"
	if len(payload.Candidates) > 0 && payload.Candidates[0].FinishReason == "MAX_TOKENS" {
		finishReason = "length"
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

func translateGeminiStream(body io.ReadCloser, model string) (io.ReadCloser, http.Header) {
	reader, writer := io.Pipe()
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")

	go func() {
		defer body.Close()
		defer writer.Close()

		dec := json.NewDecoder(body)
		sentRole := false

		for {
			var payload struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}
			if err := dec.Decode(&payload); err != nil {
				return
			}

			if !sentRole {
				_ = writeSSEChunk(writer, map[string]any{
					"id":      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
					"object":  "chat.completion.chunk",
					"created": time.Now().Unix(),
					"model":   model,
					"choices": []map[string]any{
						{"index": 0, "delta": map[string]any{"role": "assistant"}},
					},
				})
				sentRole = true
			}

			if len(payload.Candidates) > 0 && len(payload.Candidates[0].Content.Parts) > 0 {
				text := payload.Candidates[0].Content.Parts[0].Text
				if text != "" {
					_ = writeSSEChunk(writer, map[string]any{
						"id":      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
						"object":  "chat.completion.chunk",
						"created": time.Now().Unix(),
						"model":   model,
						"choices": []map[string]any{
							{"index": 0, "delta": map[string]any{"content": text}},
						},
					})
				}
			}
		}
	}()

	return reader, headers
}
