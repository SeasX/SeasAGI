package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type GrokExecutor struct{}

func (g *GrokExecutor) Name() string {
	return "grok"
}

func (g *GrokExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := &http.Client{Timeout: 120 * time.Second}

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

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.x.ai"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(baseURL, "/v1/chat/completions"), bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	applyStandardAuth(httpReq, config.APIKey)
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

func (g *GrokExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.x.ai"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(baseURL, "/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	applyStandardAuth(req, config.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, classifyError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, classifyStatusError(resp.StatusCode, resp.Body)
	}

	var payload struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

func (g *GrokExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}
