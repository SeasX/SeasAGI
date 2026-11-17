package providers

import (
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
	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
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
var sharedHTTPClient = &http.Client{Transport: sharedTransport, CheckRedirect: secureRedirectPolicy}

// metadataHTTPClient 用于模型列表等短请求，保留整体超时。
var metadataHTTPClient = &http.Client{Transport: sharedTransport, Timeout: 30 * time.Second, CheckRedirect: secureRedirectPolicy}

// secureRedirectPolicy 统一出站重定向策略：跨主机剥离凭证、禁止 https 降级（CL-S10）。
func secureRedirectPolicy(req *http.Request, via []*http.Request) error {
	return urlguard.SecureRedirect(req, via)
}

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

// translateAnthropicResponse 把 Anthropic 非流式响应翻译为 OpenAI 格式（实现位于 protocol 包）。
func translateAnthropicResponse(body io.ReadCloser, model string) (io.ReadCloser, http.Header, error) {
	data, err := protocol.AnthropicResponseToOpenAI(body, model)
	if err != nil {
		return nil, nil, err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return io.NopCloser(bytes.NewReader(data)), headers, nil
}

// chatUsage 构造 OpenAI 风格 usage。estimated 表示 token 数由本地估算得出。
func chatUsage(inputTokens, outputTokens int, estimated bool) map[string]any {
	return protocol.ChatUsage(inputTokens, outputTokens, estimated)
}

// translateAnthropicStream 把 Anthropic SSE 流翻译为 OpenAI SSE 流（实现位于 protocol 包）。
func translateAnthropicStream(body io.ReadCloser, model string) (io.ReadCloser, http.Header) {
	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")
	return protocol.AnthropicStreamToOpenAI(body, model), headers
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
