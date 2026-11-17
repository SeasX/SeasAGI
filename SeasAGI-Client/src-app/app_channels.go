package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/discovery"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
)

func (a *App) SyncPlatformChannels() error {
	if !a.authSvc.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}
	platformChannels, err := a.authSvc.FetchPlatformChannels(a.ctx)
	if err != nil {
		return err
	}
	return a.configSvc.UpsertPlatformChannels(convertPlatformChannels(platformChannels))
}

// FetchFreeChannels 从企业服务端（优先）或平台 API 拉取免费通道种子列表，供 Token 市场展示。
func (a *App) FetchFreeChannels() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	seeds, err := a.authSvc.FetchFreeChannels(a.ctx)
	if err != nil {
		// 返回空列表而不是包含 error 字段的对象：前端 Token 市场直接访问 ch.models.length，
		// 缺少 models 字段的数据会导致页面崩溃白屏。
		return []map[string]any{}
	}
	if seeds == nil {
		return []map[string]any{}
	}
	return seeds
}

// FetchEnterpriseChannels 从企业服务端拉取通道列表，含 provider_type、base_url、models。
// 客户端据此将企业服务端配置的通道同步到本地。
func (a *App) FetchEnterpriseChannels() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	channels, err := a.authSvc.FetchEnterpriseChannels(a.ctx)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	if channels == nil {
		return []map[string]any{}
	}
	return channels
}

// FetchModelCatalog 从企业服务端拉取模型目录，供通道配置页面按 provider 自动补全模型列表。
func (a *App) FetchModelCatalog() []map[string]any {
	if !a.authSvc.IsLoggedIn() {
		return []map[string]any{}
	}
	models, err := a.authSvc.FetchModelCatalog(a.ctx)
	if err != nil {
		return []map[string]any{{"error": err.Error()}}
	}
	if models == nil {
		return []map[string]any{}
	}
	return models
}

// FetchModelIndex 拉取 AI 模型指数榜单（公开资讯，无需登录；企业服务端优先，降级平台 API）。
// 返回 {category, updated_at, entries:[...]}，异常时返回 {"error": ...}。
func (a *App) FetchModelIndex(category string) map[string]any {
	data, err := a.authSvc.FetchModelIndex(a.ctx, category)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	if data == nil {
		return map[string]any{}
	}
	return data
}

func (a *App) ListChannels() ([]config.Channel, error) {
	return a.configSvc.ListChannels()
}

func (a *App) SaveCustomChannel(channel config.Channel) (string, error) {
	if channel.ChannelID == "" {
		channel.ChannelID = fmt.Sprintf("ch_%d", time.Now().UnixNano())
	}
	// 单 key 存入 keychain
	if channel.APIKey != "" {
		if err := keychain.SaveChannelKey(channel.ChannelID, channel.APIKey); err != nil {
			return "", err
		}
	}
	// 多 key 存入 keychain（不落盘 config.json）。首个 key 同时写入单 key 槽位，
	// 兼容只读取单 key 的调用路径。
	if len(channel.APIKeys) > 0 {
		if err := keychain.SaveChannelKeys(channel.ChannelID, channel.APIKeys); err != nil {
			return "", err
		}
		if channel.APIKey == "" && channel.APIKeys[0] != "" {
			if err := keychain.SaveChannelKey(channel.ChannelID, channel.APIKeys[0]); err != nil {
				return "", err
			}
		}
	}
	return a.configSvc.SaveCustomChannel(channel)
}

func (a *App) DeleteCustomChannel(channelID string) error {
	if err := keychain.DeleteChannelKey(channelID); err != nil {
		return err
	}
	if err := keychain.DeleteChannelKeys(channelID); err != nil {
		return err
	}
	return a.configSvc.DeleteCustomChannel(channelID)
}

func (a *App) ReorderChannels(orderedIDs []string) error {
	return a.configSvc.ReorderChannels(orderedIDs)
}

func (a *App) ExportConfig() (string, error) {
	data, err := a.configioSvc.Export()
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *App) ImportConfig(jsonStr string) error {
	return a.configioSvc.Import([]byte(jsonStr))
}

func (a *App) TestCustomChannel(channelID string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	return a.testConnectivity(ch, true)
}

func (a *App) TestChannelDirect(channelID string) (map[string]any, error) {
	ch, ok := a.configSvc.GetChannel(channelID)
	if !ok {
		return nil, fmt.Errorf("channel not found")
	}
	return a.testConnectivity(ch, false)
}

// testConnectivity checks whether the channel's API is reachable via a lightweight request.
// When useGateway is true, it sends a minimal request through the local gateway.
// When false, it sends a GET to the channel's models endpoint directly.
func (a *App) testConnectivity(ch config.Channel, useGateway bool) (map[string]any, error) {
	apiKey := ""
	if len(ch.APIKeys) > 0 {
		apiKey = ch.APIKeys[0]
	} else if ch.APIKey != "" {
		apiKey = ch.APIKey
	}

	channelID := ch.ChannelID

	if !useGateway {
		// Direct connectivity check: GET {base_url}/v1/models (or /models as fallback)
		if apiKey == "" {
			return map[string]any{
				"success": false,
				"error":   "no API key configured for this channel",
			}, nil
		}

		baseURL := strings.TrimRight(ch.BaseURL, "/")
		endpoints := []string{baseURL + "/v1/models", baseURL + "/models", baseURL}
		var lastErr string
		for _, url := range endpoints {
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			client := &http.Client{Timeout: 8 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				lastErr = fmt.Sprintf("connectivity test failed: %v", err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode < 500 {
				_ = a.configSvc.UpdateChannelHealth(channelID, "healthy")
				return map[string]any{
					"success":     true,
					"message":     fmt.Sprintf("channel API is reachable (HTTP %d)", resp.StatusCode),
					"status_code": resp.StatusCode,
				}, nil
			}
			lastErr = fmt.Sprintf("server returned status %d", resp.StatusCode)
		}
		_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return map[string]any{
			"success": false,
			"error":   lastErr,
		}, nil
	}

	// Via local gateway: send a minimal chat completion to verify the pipeline
	if !a.gatewaySvc.IsRunning() {
		return map[string]any{
			"success": false,
			"error":   "local gateway is not running, start it first",
		}, nil
	}

	if apiKey == "" {
		return map[string]any{
			"success": false,
			"error":   "no API key configured for this channel",
		}, nil
	}

	token, err := a.localTokenStore.GetOrCreate()
	if err != nil {
		return nil, err
	}

	port := a.gatewaySvc.GetListenPort()
	body := map[string]any{
		"model":      "gpt-4o-mini",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 1,
		"stream":     false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
		return map[string]any{
			"success": false,
			"error":   fmt.Sprintf("gateway connectivity test failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		_ = a.configSvc.UpdateChannelHealth(channelID, "healthy")
		return map[string]any{
			"success":     true,
			"message":     "channel is reachable via local gateway",
			"status_code": resp.StatusCode,
		}, nil
	}

	_ = a.configSvc.UpdateChannelHealth(channelID, "unhealthy")
	return map[string]any{
		"success":     false,
		"error":       fmt.Sprintf("gateway returned HTTP %d", resp.StatusCode),
		"status_code": resp.StatusCode,
	}, nil
}

func (a *App) DiscoverModels(channelID string) ([]discovery.DiscoveredModel, error) {
	return a.discoverySvc.DiscoverModels(channelID)
}

func (a *App) FetchModelsFromURL(modelsURL, apiKey string) (map[string]any, error) {
	// 用户填写的地址会附带渠道 Key：限制协议与目标，并禁止跨主权重定向（CL-S19）。
	if err := urlguard.ValidateSecureURL(modelsURL, true); err != nil {
		return nil, err
	}
	if u, err := neturl.Parse(modelsURL); err != nil {
		return nil, fmt.Errorf("invalid models URL: %w", err)
	} else if err := urlguard.RejectLinkLocalHost(u.Host); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: urlguard.SecureRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract model IDs from OpenAI-compatible response
	var modelNames []string
	if data, ok := result["data"].([]any); ok {
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				if id, ok := m["id"].(string); ok {
					modelNames = append(modelNames, id)
				}
			}
		}
	}

	return map[string]any{
		"models": modelNames,
		"total":  len(modelNames),
	}, nil
}

func (a *App) SyncCustomChannelsToCloud(channels []map[string]any) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	baseURL := a.configSvc.GetPlatformAPIBaseURL()
	token := a.authSvc.GetPlatformToken()

	body := map[string]any{"channels": channels}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/tenant-admin/channels/sync-custom", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.StatusCode >= 400 {
		errMsg, _ := result["error"].(string)
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, errMsg)
	}

	return result, nil
}

// wireProtocolForChannel 依据 provider 与 base_url 推断线协议。
func wireProtocolForChannel(ch config.Channel) string {
	provider := strings.ToLower(ch.ProviderType)
	base := strings.ToLower(ch.BaseURL)
	switch {
	case strings.Contains(provider, "anthropic") || strings.Contains(base, "anthropic.com"):
		return "anthropic"
	case strings.Contains(provider, "gemini") || strings.Contains(provider, "vertex") ||
		strings.Contains(provider, "google") || strings.Contains(base, "generativelanguage"):
		return "gemini"
	case strings.Contains(provider, "responses"):
		return "openai-responses"
	default:
		return "openai-chat"
	}
}

func (a *App) UpdateDefaultModel(modelName, channelID string) error {
	a.configSvc.SetDefaultModel(modelName, channelID)
	return nil
}

func convertPlatformChannels(items []map[string]interface{}) []config.Channel {
	result := make([]config.Channel, 0, len(items))
	for _, item := range items {
		channel := config.Channel{
			ChannelType:            "platform",
			Enabled:                true,
			HealthStatus:           "healthy",
			ProviderSpecificConfig: map[string]string{},
		}
		if value, ok := item["channel_id"].(string); ok {
			channel.ChannelID = value
		}
		if value, ok := item["provider_type"].(string); ok {
			channel.ProviderType = value
		}
		if value, ok := item["display_name"].(string); ok {
			channel.DisplayName = value
		}
		if value, ok := item["base_url"].(string); ok {
			channel.BaseURL = value
		}
		if value, ok := item["enabled"].(bool); ok {
			channel.Enabled = value
		}
		if values, ok := item["models"].([]interface{}); ok {
			models := make([]string, 0, len(values))
			for _, raw := range values {
				model, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if modelName, ok := model["model_name"].(string); ok {
					models = append(models, modelName)
				} else if modelID, ok := model["model_id"].(string); ok {
					models = append(models, modelID)
				}
			}
			channel.Models = models
		}
		result = append(result, channel)
	}
	return result
}

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
