package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
)

type VertexExecutor struct{}

func (v *VertexExecutor) Name() string {
	return "vertex"
}

func (v *VertexExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := &http.Client{Timeout: 120 * time.Second}

	projectID := config.ProviderSpecificConfig["project_id"]
	location := config.ProviderSpecificConfig["location"]
	if location == "" {
		location = "us-central1"
	}

	vertexBody := protocol.CanonicalToVertexAI(&protocol.CanonicalRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Extra:    req.Extra,
	}, projectID, location)

	bodyReader, contentType, err := encodeJSONBody(vertexBody)
	if err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://aiplatform.googleapis.com"
	}

	var endpoint string
	if req.Stream {
		endpoint = protocol.BuildVertexAIStreamURL(baseURL, projectID, location, req.Model)
	} else {
		endpoint = protocol.BuildVertexAIURL(baseURL, projectID, location, req.Model)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	if config.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
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
		return &UpstreamResponse{
			Status:  resp.StatusCode,
			Headers: resp.Header,
			Body:    resp.Body,
		}, nil
	}

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}

func (v *VertexExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	projectID := config.ProviderSpecificConfig["project_id"]
	location := config.ProviderSpecificConfig["location"]
	if location == "" {
		location = "us-central1"
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://aiplatform.googleapis.com"
	}

	endpoint := fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models",
		strings.TrimRight(baseURL, "/"), projectID, location)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
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
			OwnedBy: "google",
		})
	}
	return result, nil
}

func (v *VertexExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}
