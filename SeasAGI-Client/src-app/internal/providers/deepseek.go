package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type DeepSeekExecutor struct{}

func (d *DeepSeekExecutor) Name() string {
	return "deepseek"
}

func (d *DeepSeekExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := &http.Client{Timeout: 180 * time.Second}

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
		baseURL = "https://api.deepseek.com"
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

func (d *DeepSeekExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
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

func (d *DeepSeekExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}

func ExtractReasoningContent(body io.ReadCloser) (reasoning string, contentBody io.ReadCloser, err error) {
	var payload map[string]any
	data, readErr := io.ReadAll(body)
	body.Close()
	if readErr != nil {
		return "", nil, readErr
	}

	if jsonErr := json.Unmarshal(data, &payload); jsonErr != nil {
		return "", io.NopCloser(newReader(data)), nil
	}

	choices, _ := payload["choices"].([]any)
	if len(choices) == 0 {
		return "", io.NopCloser(newReader(data)), nil
	}

	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)

	if rc, ok := message["reasoning_content"].(string); ok && rc != "" {
		delete(message, "reasoning_content")
		cleaned, _ := json.Marshal(payload)
		return rc, io.NopCloser(newReader(cleaned)), nil
	}

	return "", io.NopCloser(newReader(data)), nil
}

type byteReader struct {
	data []byte
	pos  int
}

func newReader(data []byte) *byteReader {
	return &byteReader{data: data}
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
