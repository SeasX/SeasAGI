package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/protocol"
)

// sharedTransport 是进程级共享的 HTTP 连接池。它只约束「等待响应头」的时间，
// 不设置覆盖整个响应体的 Client.Timeout——后者会把长时间流式输出硬性截断。
var sharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          128,
	MaxIdleConnsPerHost:   32,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   15 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 120 * time.Second,
}

// sharedHTTPClient 用于聊天/补全请求（含流式），超时交由 ctx 与 Transport 控制。
var sharedHTTPClient = &http.Client{Transport: sharedTransport}

// metadataHTTPClient 用于模型列表等短请求，保留整体超时。
var metadataHTTPClient = &http.Client{Transport: sharedTransport, Timeout: 30 * time.Second}

type ProviderConfig struct {
	ChannelType            string
	ProviderType           string
	BaseURL                string
	APIKey                 string
	PlatformToken          string
	ProviderSpecificConfig map[string]string
}

type UpstreamRequest struct {
	Model    string
	Messages []map[string]any
	Stream   bool
	Extra    map[string]any
}

type UpstreamResponse struct {
	Status  int
	Headers http.Header
	Body    io.ReadCloser
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ProviderError struct {
	Type       string
	Message    string
	StatusCode int
	Retryable  bool
}

func (e *ProviderError) Error() string {
	return e.Message
}

type ProviderAdapter interface {
	Name() string
	ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error)
	ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error)
	GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error)
}

func GetAdapter(providerType string) ProviderAdapter {
	return ResolveExecutor(&ProviderConfig{ProviderType: providerType})
}

func ResolveExecutor(config *ProviderConfig) ProviderAdapter {
	if config == nil {
		return &OpenAIExecutor{}
	}
	if config.ChannelType == "platform" {
		return &PlatformExecutor{}
	}

	if adapter := ResolveFromRegistry(config.ProviderType); adapter != nil {
		return adapter
	}

	if looksLikeAzure(config) {
		return &AzureOpenAIExecutor{}
	}
	if looksLikeAnthropic(config) {
		return &AnthropicExecutor{}
	}
	return &OpenAIExecutor{}
}

type OpenAIExecutor struct{}

type PlatformExecutor struct{}

type AzureOpenAIExecutor struct{}

type AnthropicExecutor struct{}

func (a *OpenAIExecutor) Name() string {
	return "openai"
}

func (a *OpenAIExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := sharedHTTPClient

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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(config.BaseURL, "/v1/chat/completions"), bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	applyStandardAuth(httpReq, config.APIKey)

	if org, ok := config.ProviderSpecificConfig["organization"]; ok {
		httpReq.Header.Set("Openai-Organization", org)
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

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}

func (a *OpenAIExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := metadataHTTPClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(config.BaseURL, "/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	applyStandardAuth(req, config.APIKey)
	forwardProviderHeaders(req, config.ProviderSpecificConfig, "header.")

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

func (a *PlatformExecutor) Name() string {
	return "platform"
}

func (a *PlatformExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := sharedHTTPClient
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(config.BaseURL, "/chat/completions"), bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	applyStandardAuth(httpReq, config.PlatformToken)
	if req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

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

func (a *PlatformExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := metadataHTTPClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(config.BaseURL, "/models"), nil)
	if err != nil {
		return nil, err
	}
	applyStandardAuth(req, config.PlatformToken)

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

func (a *AzureOpenAIExecutor) Name() string {
	return "azure-openai"
}

func (a *AzureOpenAIExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	deployment := strings.TrimSpace(config.ProviderSpecificConfig["deployment_name"])
	if deployment == "" {
		return nil, &ProviderError{Type: "configuration_error", Message: "azure deployment_name is required"}
	}

	client := sharedHTTPClient
	body := map[string]any{
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, buildAzureChatURL(config.BaseURL, deployment, config.ProviderSpecificConfig["api_version"]), bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	if config.APIKey != "" {
		httpReq.Header.Set("api-key", config.APIKey)
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

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}

func (a *AzureOpenAIExecutor) ListModels(_ context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	deployment := strings.TrimSpace(config.ProviderSpecificConfig["deployment_name"])
	if deployment == "" {
		return nil, &ProviderError{Type: "configuration_error", Message: "azure deployment_name is required"}
	}
	return []ModelInfo{
		{
			ID:      deployment,
			Object:  "model",
			Created: time.Now().Unix(),
			OwnedBy: "azure-openai",
		},
	}, nil
}

func (a *AnthropicExecutor) Name() string {
	return "anthropic"
}

func (a *AnthropicExecutor) ChatCompletions(ctx context.Context, config *ProviderConfig, req *UpstreamRequest) (*UpstreamResponse, error) {
	client := sharedHTTPClient

	body := map[string]any{
		"model":      req.Model,
		"messages":   protocol.ToAnthropicMessages(req.Messages),
		"stream":     req.Stream,
		"max_tokens": anthropicMaxTokens(req.Extra),
	}
	if systemPrompt := extractSystemPrompt(req.Messages); systemPrompt != "" {
		body["system"] = systemPrompt
	}

	for k, v := range req.Extra {
		if _, blocked := anthropicBlockedFields[k]; blocked {
			continue
		}
		switch k {
		case "tools":
			// OpenAI function 工具定义 → Anthropic tools（input_schema 外壳）
			body[k] = toAnthropicTools(v)
			continue
		case "tool_choice":
			// OpenAI tool_choice → Anthropic tool_choice
			body[k] = toAnthropicToolChoice(v)
			continue
		}
		body[k] = v
	}

	bodyReader, contentType, err := encodeJSONBody(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, joinURL(config.BaseURL, "/v1/messages"), bodyReader)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("anthropic-version", anthropicVersion(config))
	if config.APIKey != "" {
		httpReq.Header.Set("x-api-key", config.APIKey)
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
		body, headers := translateAnthropicStream(resp.Body, req.Model)
		return &UpstreamResponse{
			Status:  http.StatusOK,
			Headers: headers,
			Body:    body,
		}, nil
	}

	translatedBody, headers, err := translateAnthropicResponse(resp.Body, req.Model)
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

func (a *AnthropicExecutor) ListModels(ctx context.Context, config *ProviderConfig) ([]ModelInfo, error) {
	client := metadataHTTPClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(config.BaseURL, "/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("anthropic-version", anthropicVersion(config))
	if config.APIKey != "" {
		req.Header.Set("x-api-key", config.APIKey)
		req.Header.Set("Authorization", "Bearer "+config.APIKey)
	}
	forwardProviderHeaders(req, config.ProviderSpecificConfig, "header.")

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
	if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil && len(payload.Data) > 0 {
		return payload.Data, nil
	}

	return []ModelInfo{}, nil
}

func encodeJSONBody(body any) (io.Reader, string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(data), "application/json", nil
}

func classifyError(err error) *ProviderError {
	return &ProviderError{Type: "network_error", Message: err.Error(), Retryable: true}
}

func classifyStatusError(status int, body io.ReadCloser) *ProviderError {
	data, _ := io.ReadAll(body)
	message := string(bytes.TrimSpace(data))
	if message == "" {
		message = fmt.Sprintf("upstream returned status %d", status)
	}
	return &ProviderError{
		Type:       "upstream_error",
		Message:    message,
		StatusCode: status,
		Retryable:  isRetryableStatus(status),
	}
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return status >= 500
	}
}

func applyStandardAuth(req *http.Request, token string) {
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func forwardProviderHeaders(req *http.Request, config map[string]string, prefix string) {
	for key, value := range config {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		headerName := strings.TrimPrefix(key, prefix)
		if headerName == "" || value == "" {
			continue
		}
		req.Header.Set(headerName, value)
	}
}

func joinURL(baseURL, suffix string) string {
	return strings.TrimRight(baseURL, "/") + suffix
}

func looksLikeAzure(config *ProviderConfig) bool {
	if config == nil {
		return false
	}
	if strings.EqualFold(config.ProviderType, "azure-openai") {
		return true
	}
	return strings.TrimSpace(config.ProviderSpecificConfig["deployment_name"]) != ""
}

func looksLikeAnthropic(config *ProviderConfig) bool {
	if config == nil {
		return false
	}
	if strings.EqualFold(config.ProviderType, "anthropic") {
		return true
	}
	return strings.EqualFold(config.ProviderSpecificConfig["protocol"], "anthropic")
}

func buildAzureChatURL(baseURL, deployment, apiVersion string) string {
	normalizedBase := strings.TrimRight(baseURL, "/")
	if apiVersion == "" {
		apiVersion = "2024-02-01"
	}
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		normalizedBase,
		url.PathEscape(deployment),
		url.QueryEscape(apiVersion),
	)
}

var anthropicBlockedFields = map[string]struct{}{
	"messages": {},
	"model":    {},
	"stream":   {},
}

func anthropicVersion(config *ProviderConfig) string {
	if config == nil {
		return "2023-06-01"
	}
	if version := strings.TrimSpace(config.ProviderSpecificConfig["anthropic_version"]); version != "" {
		return version
	}
	return "2023-06-01"
}

func anthropicMaxTokens(extra map[string]any) int {
	if extra == nil {
		return 4096
	}
	switch value := extra["max_tokens"].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	}
	return 4096
}

func extractSystemPrompt(messages []map[string]any) string {
	var systems []string
	for _, message := range messages {
		role, _ := message["role"].(string)
		if role != "system" {
			continue
		}
		if text, ok := message["content"].(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				systems = append(systems, text)
			}
		}
	}
	return strings.Join(systems, "\n\n")
}

func translateAnthropicResponse(body io.ReadCloser, model string) (io.ReadCloser, http.Header, error) {
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
		return nil, nil, err
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
		"usage": chatUsage(payload.Usage.InputTokens, payload.Usage.OutputTokens, false),
	}

	data, err := json.Marshal(openAIResponse)
	if err != nil {
		return nil, nil, err
	}

	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return io.NopCloser(bytes.NewReader(data)), headers, nil
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

// chatUsage 构造 OpenAI 风格 usage。estimated 表示 token 数由本地估算得出。
func chatUsage(inputTokens, outputTokens int, estimated bool) map[string]any {
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

func translateAnthropicStream(body io.ReadCloser, model string) (io.ReadCloser, http.Header) {
	reader, writer := io.Pipe()
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

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
			return writeSSEChunk(writer, map[string]any{
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
				out = protocol.EstimateTokens(contentBuf.String())
				estimated = true
			}
			if !usageSeen && (out > 0 || inputTokens > 0) {
				estimated = true
			}
			return chunk(map[string]any{
				"usage": chatUsage(inputTokens, out, estimated),
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

	return reader, headers
}

// toAnthropicTools 把 OpenAI tools 定义转换为 Anthropic tools 格式。
// 已是 Anthropic 格式（含 input_schema）的定义原样保留。
func toAnthropicTools(raw any) any {
	list, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]map[string]any); ok {
			list = make([]any, 0, len(typed))
			for _, item := range typed {
				list = append(list, item)
			}
		} else {
			return raw
		}
	}

	result := make([]map[string]any, 0, len(list))
	for _, item := range list {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if _, hasInputSchema := tool["input_schema"]; hasInputSchema {
			result = append(result, tool)
			continue
		}
		fn, ok := tool["function"].(map[string]any)
		if !ok {
			result = append(result, tool)
			continue
		}
		converted := map[string]any{}
		if name, ok := fn["name"].(string); ok {
			converted["name"] = name
		}
		if desc, ok := fn["description"].(string); ok && desc != "" {
			converted["description"] = desc
		}
		if params, ok := fn["parameters"].(map[string]any); ok {
			converted["input_schema"] = params
		} else {
			converted["input_schema"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		result = append(result, converted)
	}
	return result
}

// toAnthropicToolChoice 把 OpenAI tool_choice 语义映射为 Anthropic tool_choice。
func toAnthropicToolChoice(raw any) any {
	switch choice := raw.(type) {
	case string:
		switch choice {
		case "required":
			return map[string]any{"type": "any"}
		case "none":
			return map[string]any{"type": "none"}
		default:
			return map[string]any{"type": "auto"}
		}
	case map[string]any:
		if fn, ok := choice["function"].(map[string]any); ok {
			if name, ok := fn["name"].(string); ok && name != "" {
				return map[string]any{"type": "tool", "name": name}
			}
		}
		if t, ok := choice["type"].(string); ok {
			switch t {
			case "any", "auto", "none", "tool":
				return choice
			}
		}
	}
	return raw
}

func writeSSEChunk(w io.Writer, payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

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

func (a *OpenAIExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}

func (a *PlatformExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.PlatformToken)
}

func (a *AzureOpenAIExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}

func (a *AnthropicExecutor) GenericPost(ctx context.Context, config *ProviderConfig, path string, body []byte) (*UpstreamResponse, error) {
	return genericPostImpl(ctx, config, path, body, config.APIKey)
}

func genericPostImpl(ctx context.Context, config *ProviderConfig, path string, body []byte, authToken string) (*UpstreamResponse, error) {
	client := sharedHTTPClient
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	url := joinURL(baseURL, path)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if authToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+authToken)
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

	return &UpstreamResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    resp.Body,
	}, nil
}
