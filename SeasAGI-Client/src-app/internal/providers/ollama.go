package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type OllamaExecutor struct{}

func (o *OllamaExecutor) Name() string {
	return "ollama"
}

func (o *OllamaExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := &http.Client{Timeout: 300 * time.Second}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   req.Stream,
	}
	for k, v := range req.Extra {
		body[k] = v
	}

	bodyReader, contentType, err := encodeJSONBody(body)
	if err != nil {
		return nil, err
	}

	endpoint := joinURL(baseURL, "/v1/chat/completions")

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

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}

func (o *OllamaExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(baseURL, "/api/tags"), nil)
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
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return []ModelInfo{}, nil
	}

	result := make([]ModelInfo, 0, len(payload.Models))
	for _, m := range payload.Models {
		result = append(result, ModelInfo{
			ID:      m.Name,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "ollama",
		})
	}
	return result, nil
}

func (o *OllamaExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	client := &http.Client{Timeout: 300 * time.Second}
	endpoint := joinURL(baseURL, path)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	forwardProviderHeaders(httpReq, config.ProviderSpecificConfig, "header.")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, classifyError(err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, classifyStatusError(resp.StatusCode, resp.Body)
	}

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}
