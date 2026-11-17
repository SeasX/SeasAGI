package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/SeasAGI/SeasAGI-Client/internal/network"
)

func (a *App) StartLocalGateway() error {
	go a.gatewaySvc.Start(a.ctx)
	return nil
}

func (a *App) StopLocalGateway() error {
	a.gatewaySvc.Stop()
	return nil
}

func (a *App) GetRuntimeStatus() map[string]any {
	cfg := a.configSvc.GetConfig()
	return map[string]any{
		"gateway_running":    a.gatewaySvc.IsRunning(),
		"listen_port":        a.gatewaySvc.GetListenPort(),
		"default_model":      emptyStringToNil(cfg.DefaultModel),
		"default_channel_id": emptyStringToNil(cfg.DefaultChannelID),
	}
}

// GetComboRouteMetrics returns combo-level route metrics from the gateway
func (a *App) GetComboRouteMetrics() []map[string]any {
	metrics := a.gatewaySvc.GetComboRouteMetrics()
	result := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		result = append(result, map[string]any{
			"combo_name":         m.ComboName,
			"total_requests":     m.TotalRequests,
			"total_fallbacks":    m.TotalFallbacks,
			"step1_success":      m.Step1Success,
			"last_step_fallback": m.LastStepFallback,
			"step1_hit_rate":     m.Step1HitRate,
			"last_step_hit_rate": m.LastStepHitRate,
			"avg_attempts":       m.AvgAttempts,
			"task_type":          m.TaskType,
		})
	}
	return result
}

// GetLocalAccessTokenStatus 返回本地令牌是否存在与掩码值。
// 渲染层不再持有完整令牌，复制动作由后端直接写入系统剪贴板（CL-S5）。
func (a *App) GetLocalAccessTokenStatus() (map[string]any, error) {
	token, err := a.localTokenStore.Get()
	if err != nil || token == "" {
		token, err = a.localTokenStore.GetOrCreate()
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"present": token != "", "masked": maskToken(token)}, nil
}

// CopyLocalAccessToken 把完整本地令牌写入系统剪贴板；明文不经过渲染层。
func (a *App) CopyLocalAccessToken() error {
	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return err
	}
	return runtime.ClipboardSetText(a.ctx, token)
}

// CopyGatewaySnippet 复制接入片段（Base URL + API Key），明文由后端拼接。
func (a *App) CopyGatewaySnippet() error {
	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return err
	}
	text := fmt.Sprintf("Base URL: http://127.0.0.1:%d/v1\nAPI Key: %s", a.gatewaySvc.GetListenPort(), token)
	return runtime.ClipboardSetText(a.ctx, text)
}

// ResetLocalAccessToken 轮换本地令牌并返回新的掩码状态（不返回明文）。
func (a *App) ResetLocalAccessToken() (map[string]any, error) {
	token, err := a.localTokenStore.Reset()
	if err != nil {
		return nil, err
	}
	a.gatewaySvc.SetAccessToken(token)
	// MITM 拦截转发注入的令牌需同步更新，避免被接管请求因旧令牌 401。
	if a.mitmMgr != nil {
		a.mitmMgr.SetAccessToken(token)
	}
	return map[string]any{"present": true, "masked": maskToken(token)}, nil
}

// maskToken 仅保留首尾各 4 位，避免渲染层拿到可用凭证。
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("•", len(token))
	}
	return token[:4] + "••••" + token[len(token)-4:]
}

func (a *App) SetAutoLaunch(enabled bool) error {
	if err := network.SetAutoLaunch(enabled); err != nil {
		return err
	}
	return a.configSvc.SetAutoLaunch(enabled)
}

func (a *App) ChatCompletion(messages []map[string]any, model string) (map[string]any, error) {
	body := map[string]any{
		"model":    model,
		"messages": messages,
		// 本地网关识别该标记后，在非流式响应中附加 _combo_steps/_intent 诊断字段
		"_seasagi_debug": true,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// P8: Grant-based routing — if a grant is selected, send directly to relay gateway
	grantID := a.configSvc.GetSelectedGrantID()
	grantRelayURL := a.configSvc.GetSelectedGrantRelayURL()
	if grantID != "" && grantRelayURL != "" {
		targetURL := strings.TrimRight(grantRelayURL, "/") + "/v1/chat/completions"
		req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Grant-Id", grantID)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("relay gateway request failed: %w", err)
		}
		defer resp.Body.Close()

		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var result map[string]any
		if err := json.Unmarshal(respBytes, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		result["_gateway_status"] = resp.StatusCode
		result["_grant_id"] = grantID
		result["_curl_command"] = buildCurlCommand(targetURL, "X-Grant-Id: "+grantID, string(bodyBytes))
		return result, nil
	}

	// Default: local gateway routing
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort

	// Build curl command for debugging。
	// 安全：真实令牌绝不回传给前端（会被截图/复制外泄），统一使用环境变量占位符，
	// 用户在自己的终端执行时再导出同名变量即可复现。
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", gatewayPort),
		curlChannelKeyAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}

// curlChannelKeyAuthHeader 是 Playground 回传 curl 命令时使用的上游渠道 Key 占位符。
// 真实的渠道 Key / 本机令牌只保留在进程内，绝不出现在任何回传数据里。
const curlChannelKeyAuthHeader = "Bearer $SEASAGI_CHANNEL_KEY"

// curlLocalTokenAuthHeader 是访问本机网关时使用的令牌占位符（CL-S2）。
const curlLocalTokenAuthHeader = "Bearer $SEASAGI_LOCAL_TOKEN"

func buildCurlCommand(url, authHeader, bodyJSON string) string {
	escapedBody := strings.ReplaceAll(bodyJSON, "'", "'\\''")
	return fmt.Sprintf("curl -X POST '%s' \\\n  -H 'Content-Type: application/json' \\\n  -H 'Authorization: %s' \\\n  -d '%s'", url, authHeader, escapedBody)
}

func (a *App) ChatCompletionForChannel(channelID string, messages []map[string]any, model string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode

	// Build curl command for debugging。
	// 安全：上游渠道 Key 绝不能出现在回传内容中（Playground 截图/日志会外泄），
	// 只返回环境变量占位符。
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/chat/completions",
		curlChannelKeyAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) ImageGeneration(prompt string, model string, params map[string]any) (map[string]any, error) {
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/images/generations", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort
	// 安全：本机网关令牌同样不得回传前端，只给环境变量占位符（CL-S2）。
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/images/generations", gatewayPort),
		curlLocalTokenAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) ImageGenerationForChannel(channelID string, prompt string, model string, params map[string]any) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/images/generations", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode
	// 安全：上游渠道 Key 不得回传前端（截图/日志外泄），只给环境变量占位符（CL-S2）。
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/images/generations",
		curlChannelKeyAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) VideoGeneration(prompt string, model string, params map[string]any) (map[string]any, error) {
	gatewayPort := a.gatewaySvc.GetListenPort()
	if gatewayPort == 0 {
		return nil, fmt.Errorf("local gateway is not running")
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/videos/generations", gatewayPort),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_gateway_status"] = resp.StatusCode
	result["_gateway_port"] = gatewayPort
	// 安全：本机网关令牌同样不得回传前端，只给环境变量占位符（CL-S2）。
	result["_curl_command"] = buildCurlCommand(
		fmt.Sprintf("http://127.0.0.1:%d/v1/videos/generations", gatewayPort),
		curlLocalTokenAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}

func (a *App) VideoGenerationForChannel(channelID string, prompt string, model string, params map[string]any) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key configured for channel %s", channelID)
	}

	body := map[string]any{
		"model":  model,
		"prompt": prompt,
	}
	for k, v := range params {
		body[k] = v
	}
	bodyBytes, _ := json.Marshal(body)

	baseURL := strings.TrimRight(ch.BaseURL, "/")
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/videos/generations", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("channel request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result["_channel_id"] = channelID
	result["_channel_name"] = ch.DisplayName
	result["_status_code"] = resp.StatusCode
	// 安全：上游渠道 Key 不得回传前端（截图/日志外泄），只给环境变量占位符（CL-S2）。
	result["_curl_command"] = buildCurlCommand(
		baseURL+"/videos/generations",
		curlChannelKeyAuthHeader,
		string(bodyBytes),
	)

	return result, nil
}
