package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/auth"
	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/gateway"
	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/mitm"
)

func (a *App) CreateCheckoutSession(planID string, quantity ...int) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	qty := 1
	if len(quantity) > 0 && quantity[0] > 1 {
		qty = quantity[0]
	}

	body := map[string]any{"plan_id": planID, "quantity": qty}
	bodyBytes, _ := json.Marshal(body)

	apiBase := a.authSvc.PlatformAPIBaseURL()
	reqCtx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, apiBase+"/checkout/create", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.authSvc.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if resp.StatusCode >= 400 {
		errMsg := "create checkout session failed"
		if result != nil {
			if e, ok := result["error"].(string); ok {
				errMsg = e
			}
		}
		return nil, errors.New(errMsg)
	}
	return result, nil
}

func (a *App) Login(email, password string) error {
	if err := a.authSvc.Login(email, password); err != nil {
		return err
	}
	return a.afterPlatformLogin()
}

// afterPlatformLogin 登录成功后的公共处理：拉平台通道、云同步、启动网关。
func (a *App) afterPlatformLogin() error {
	platformChannels, err := a.authSvc.FetchPlatformChannels(a.ctx)
	if err != nil {
		return err
	}

	if err := a.configSvc.UpsertPlatformChannels(convertPlatformChannels(platformChannels)); err != nil {
		return err
	}

	// Sync local data to cloud (best-effort)
	_ = a.syncLocalDataToCloud()

	// P4-24：登录成功后立即同步企业下发的 MITM 路由规则（best-effort）。
	go func() {
		token := ""
		if a.authSvc.IsLoggedIn() {
			token = a.authSvc.GetPlatformToken()
		}
		if err := mitm.FetchRoutingRulesFromEnterprise(a.ctx, token); err != nil {
			logging.Debugf("fetch enterprise mitm routing rules after login: %v", err)
		}
	}()

	if !a.gatewaySvc.IsRunning() {
		go a.gatewaySvc.Start(a.ctx)
	}
	return nil
}

// GetOAuthProviders 获取服务端已配置的第三方登录方式。
func (a *App) GetOAuthProviders() ([]auth.OAuthProvider, error) {
	return a.authSvc.FetchOAuthProviders()
}

// StartOAuthLogin 发起第三方 OAuth 登录：
// 请求服务端创建授权会话 -> 打开浏览器访问服务端授权跳转页（服务端 302 到 Google/GitHub）
// -> 轮询服务端取回登录态。
func (a *App) StartOAuthLogin(provider string) error {
	if err := a.authSvc.StartOAuthLogin(provider); err != nil {
		return err
	}
	return a.afterPlatformLogin()
}

func (a *App) Logout() error {
	a.gatewaySvc.Stop()

	// Sync local data before clearing auth, unless user is not logged in
	if a.authSvc.IsLoggedIn() {
		if err := a.syncLocalDataToCloud(); err != nil {
			// Best-effort: don't block logout
		}
	}

	if err := a.authSvc.Logout(); err != nil {
		return err
	}
	return a.configSvc.ClearPlatformChannels()
}

func (a *App) SyncLocalDataToCloud() error {
	return a.syncLocalDataToCloud()
}

func (a *App) syncLocalDataToCloud() error {
	if !a.authSvc.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}

	// Sync local custom channels
	channels, _ := a.configSvc.ListChannels()
	var customChannels []map[string]any
	for _, ch := range channels {
		if ch.ChannelType == "custom" {
			customChannels = append(customChannels, map[string]any{
				"channel_id":    ch.ChannelID,
				"display_name":  ch.DisplayName,
				"provider_type": ch.ProviderType,
				"base_url":      ch.BaseURL,
				"channel_type":  ch.ChannelType,
				"models":        ch.Models,
				"enabled":       ch.Enabled,
			})
		}
	}
	if len(customChannels) > 0 {
		_, err := a.SyncCustomChannelsToCloud(customChannels)
		if err != nil {
			return fmt.Errorf("sync channels: %w", err)
		}
	}

	// Sync local combos
	combos := a.configSvc.ListModelCombos()
	for _, combo := range combos {
		stepsJSON, _ := json.Marshal(combo.Steps)
		_, _ = a.PushCloudCombo(combo.LogicalName, combo.DisplayName, combo.Description,
			combo.Strategy, combo.StickyUses, combo.QuickStrategy,
			combo.TaskProfile, string(stepsJSON))
	}

	return nil
}

func (a *App) GetAuthState() auth.AuthInfo {
	return a.authSvc.GetAuthState()
}

// SyncAssetsToCloud 把本地可上架资产（含凭证）同步到企业端资产注册表。
// 企业端立即用 KMS 信封加密落库，客户端不保留额外副本。
func (a *App) SyncAssetsToCloud(assets []map[string]any) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	baseURL := a.configSvc.GetPlatformAPIBaseURL()
	token := a.authSvc.GetPlatformToken()

	bodyBytes, err := json.Marshal(map[string]any{"assets": assets})
	if err != nil {
		return nil, fmt.Errorf("failed to encode assets: %w", err)
	}

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		baseURL+"/user-assets/sync", bytes.NewReader(bodyBytes))
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

// ListMyAssets 拉取企业端资产注册表（不返回凭证）。
func (a *App) ListMyAssets() ([]map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	baseURL := a.configSvc.GetPlatformAPIBaseURL()
	token := a.authSvc.GetPlatformToken()

	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet, baseURL+"/user-assets", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return payload.Data, nil
}

// ExportLocalAssets 从本地通道生成可上架资产清单（含加密凭证与指纹）。
//
// 指纹 = SHA-256(api_key) 前 16 字节十六进制，不可逆，仅用于企业端识别同一凭证；
// 这样企业端无需知道明文即可完成「同一资产的唯一注册与所有权校验」。
// 凭证只走平台 RSA-OAEP 公钥信封（api_key_enc）：拿不到公钥或加密失败时整体失败，
// 绝不回退明文（CL-S15）；OAuth 会话类通道不含凭证，仅登记资产元数据。
func (a *App) ExportLocalAssets() ([]map[string]any, error) {
	// 走受控凭证读取（keychain），脱敏渠道列表不含密钥、会导出空清单。
	channels, err := a.configSvc.ListChannelsWithCredentials()
	if err != nil {
		return nil, err
	}

	assets := make([]map[string]any, 0, len(channels))
	// 平台公钥按需获取：只有需要上送 API Key 时才要求登录态/公钥可用。
	var pubKey *rsa.PublicKey
	for _, ch := range channels {
		key := strings.TrimSpace(ch.APIKey)
		if key == "" && len(ch.APIKeys) > 0 {
			key = strings.TrimSpace(ch.APIKeys[0])
		}
		if strings.TrimSpace(ch.BaseURL) == "" || !ch.Enabled {
			continue
		}
		// OAuth 会话类通道没有 API Key：仅能通过 A 端节点模式交付，凭证不出本机。
		if key == "" {
			if strings.TrimSpace(ch.OAuthRefreshToken) == "" {
				continue
			}
			sum := sha256.Sum256([]byte("oauth:" + ch.ChannelID))
			assets = append(assets, map[string]any{
				"token_fingerprint": hex.EncodeToString(sum[:16]),
				"label":             ch.DisplayName,
				"provider_type":     ch.ProviderType,
				"wire_protocol":     wireProtocolForChannel(ch),
				"auth_type":         "oauth",
				"base_url":          strings.TrimRight(ch.BaseURL, "/"),
				"models":            ch.Models,
			})
			continue
		}
		sum := sha256.Sum256([]byte(key))
		if pubKey == nil {
			pubKey, err = a.fetchAssetPublicKey()
			if err != nil {
				return nil, fmt.Errorf("refusing to export assets without platform encryption key: %w", err)
			}
		}
		enc, err := encryptWithPublicKey(pubKey, key)
		if err != nil {
			return nil, fmt.Errorf("encrypt credential for channel %s: %w", ch.ChannelID, err)
		}
		if enc == "" {
			return nil, fmt.Errorf("encrypt credential for channel %s: empty ciphertext", ch.ChannelID)
		}
		assets = append(assets, map[string]any{
			"token_fingerprint": hex.EncodeToString(sum[:16]),
			"label":             ch.DisplayName,
			"provider_type":     ch.ProviderType,
			"wire_protocol":     wireProtocolForChannel(ch),
			"auth_type":         "api_key",
			"base_url":          strings.TrimRight(ch.BaseURL, "/"),
			"models":            ch.Models,
			"api_key_enc":       enc,
		})
	}
	return assets, nil
}

// fetchAssetPublicKey 获取企业端资产公钥（RSA-OAEP-SHA256）。
func (a *App) fetchAssetPublicKey() (*rsa.PublicKey, error) {
	status, body, err := a.authSvc.DoPlatformRequest(http.MethodGet, "/user-assets/pubkey", nil)
	if err != nil || status >= 400 {
		return nil, fmt.Errorf("asset public key unavailable")
	}
	var payload struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(payload.Data.PublicKey))
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("unexpected public key type")
	}
	return pub, nil
}

func encryptWithPublicKey(pub *rsa.PublicKey, plain string) (string, error) {
	if pub == nil || plain == "" {
		return "", fmt.Errorf("public key unavailable")
	}
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, []byte(plain), nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// marketNodeSecretName 是 A 端节点共享密钥在系统钥匙串中的键名（不落配置文件）。
const marketNodeSecretName = "market_node_secret"

// nodeHeartbeatRunning 保证同一进程只有一个节点心跳协程。
var nodeHeartbeatRunning int32

// RegisterMarketNode 注册（或更新）A 端节点并开启心跳。
//
// 节点地址需由用户填写可被企业 relay 访问的地址（Tunnel 公网地址或内网反代）；
// 共享密钥仅写入系统钥匙串，企业端只保存 node_id 与哈希后的转发密钥。
func (a *App) RegisterMarketNode(nodeURL string, models, protocols []string) (map[string]any, error) {
	nodeURL = strings.TrimRight(strings.TrimSpace(nodeURL), "/")
	if nodeURL == "" {
		return nil, fmt.Errorf("node URL is required")
	}
	if !strings.HasPrefix(nodeURL, "http://") && !strings.HasPrefix(nodeURL, "https://") {
		return nil, fmt.Errorf("node URL must start with http:// or https://")
	}

	data, err := a.registerNodeOnPlatform(nodeURL, models, protocols)
	if err != nil {
		return nil, err
	}
	if nodeID, _ := data["node_id"].(string); nodeID != "" {
		cfg := config.MarketNodeConfig{
			NodeID:        nodeID,
			NodeURL:       nodeURL,
			ListenPort:    a.gatewaySvc.GetListenPort(),
			Models:        models,
			WireProtocols: protocols,
			Enabled:       true,
		}
		if err := a.configSvc.SetMarketNodeConfig(cfg); err != nil {
			return nil, err
		}
	}
	a.startNodeHeartbeat()
	return a.GetMarketNodeStatus(), nil
}

// GetMarketNodeStatus 返回本地节点配置与平台侧注册状态。
func (a *App) GetMarketNodeStatus() map[string]any {
	cfg := a.configSvc.GetMarketNodeConfig()
	out := map[string]any{
		"enabled":         cfg.Enabled && gateway.NodeSecretConfigured(),
		"node_id":         cfg.NodeID,
		"node_url":        cfg.NodeURL,
		"models":          cfg.Models,
		"wire_protocols":  cfg.WireProtocols,
		"listen_port":     a.gatewaySvc.GetListenPort(),
		"gateway_running": a.gatewaySvc.IsRunning(),
	}
	if a.authSvc.IsLoggedIn() {
		if status, body, err := a.authSvc.DoPlatformRequest(http.MethodGet, "/nodes/me", nil); err == nil && status < 400 {
			var payload struct {
				Data map[string]any `json:"data"`
			}
			if json.Unmarshal(body, &payload) == nil && payload.Data != nil {
				out["registered"] = true
				out["remote"] = payload.Data
			}
		}
	}
	return out
}

// DisableMarketNode 注销节点：停止心跳、清空本地密钥与配置。
func (a *App) DisableMarketNode() error {
	if a.authSvc.IsLoggedIn() {
		// 平台侧注销失败不阻断本地关闭，避免残留密钥继续可用。
		_, _, _ = a.authSvc.DoPlatformRequest(http.MethodDelete, "/nodes/me", nil)
	}
	_ = keychain.DeleteSecret(marketNodeSecretName)
	gateway.SetNodeSecret("")
	cfg := a.configSvc.GetMarketNodeConfig()
	cfg.NodeID = ""
	cfg.Enabled = false
	return a.configSvc.SetMarketNodeConfig(cfg)
}

// restoreMarketNode 在启动时恢复节点密钥并继续心跳；密钥缺失时尝试重新注册。
func (a *App) restoreMarketNode() {
	cfg := a.configSvc.GetMarketNodeConfig()
	if !cfg.Enabled || strings.TrimSpace(cfg.NodeURL) == "" {
		return
	}
	if secret, err := keychain.GetSecret(marketNodeSecretName); err == nil && secret != "" {
		gateway.SetNodeSecret(secret)
		a.startNodeHeartbeat()
		return
	}
	go func() {
		if !a.authSvc.IsLoggedIn() {
			return
		}
		if _, err := a.registerNodeOnPlatform(cfg.NodeURL, cfg.Models, cfg.WireProtocols); err == nil {
			a.startNodeHeartbeat()
		}
	}()
}

func (a *App) startNodeHeartbeat() {
	if !atomic.CompareAndSwapInt32(&nodeHeartbeatRunning, 0, 1) {
		return
	}
	go func() {
		defer atomic.StoreInt32(&nodeHeartbeatRunning, 0)
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				cfg := a.configSvc.GetMarketNodeConfig()
				if !cfg.Enabled || strings.TrimSpace(cfg.NodeURL) == "" {
					return
				}
				_, _ = a.registerNodeOnPlatform(cfg.NodeURL, cfg.Models, cfg.WireProtocols)
			}
		}
	}()
}

// registerNodeOnPlatform 调用平台注册接口，并把返回的共享密钥写入钥匙串与网关。
func (a *App) registerNodeOnPlatform(nodeURL string, models, protocols []string) (map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	payload, err := json.Marshal(map[string]any{
		"node_url":       nodeURL,
		"display_name":   "SeasAGI Client Node",
		"models":         models,
		"wire_protocols": protocols,
	})
	if err != nil {
		return nil, err
	}
	status, body, err := a.authSvc.DoPlatformRequest(http.MethodPost, "/nodes/register", payload)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data  map[string]any `json:"data"`
		Error string         `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)
	if status >= 400 {
		return nil, fmt.Errorf("register node failed (%d): %s", status, parsed.Error)
	}
	if parsed.Data == nil {
		return nil, fmt.Errorf("empty node registration response")
	}
	if secret, ok := parsed.Data["node_secret"].(string); ok && secret != "" {
		_ = keychain.SaveSecret(marketNodeSecretName, secret)
		gateway.SetNodeSecret(secret)
	}
	return parsed.Data, nil
}

func (a *App) HandleDeepLink(rawURL string) error {
	return a.deeplinkMgr.Handle(rawURL)
}

func (a *App) Register(email, password, displayName string) error {
	return a.authSvc.Register(email, password, displayName)
}

func (a *App) GetCloudUsage() (map[string]any, error) {
	usageData, err := a.authSvc.FetchCloudUsage()
	if err != nil {
		return nil, err
	}
	if usageData == nil {
		return nil, nil
	}
	return map[string]any{
		"month_requests":      usageData.MonthRequests,
		"month_input_tokens":  usageData.MonthInputTokens,
		"month_output_tokens": usageData.MonthOutputTokens,
		"total_cost_usd":      usageData.TotalCostUSD,
	}, nil
}

func (a *App) GetCloudBilling() (map[string]any, error) {
	billingData, err := a.authSvc.FetchCloudBilling()
	if err != nil {
		return nil, err
	}
	if billingData == nil {
		return nil, nil
	}
	return map[string]any{
		"plan_id":        billingData.PlanID,
		"plan_name":      billingData.PlanName,
		"price":          billingData.Price,
		"quota":          billingData.Quota,
		"used_quota":     billingData.UsedQuota,
		"renewal_date":   billingData.RenewalDate,
		"relay_enabled":  billingData.RelayEnabled,
		"relay_gateways": billingData.RelayGateways,
	}, nil
}

// GetOverageUsage 获取当前用户超额用量（由 Go 后端代理平台请求，避免前端直接持有并外发平台 Token）。
func (a *App) GetOverageUsage() (map[string]any, error) {
	rec, err := a.authSvc.FetchOverageUsage()
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	return map[string]any{
		"overage_id":       rec.OverageID,
		"user_id":          rec.UserID,
		"plan_id":          rec.PlanID,
		"billing_period":   rec.BillingPeriod,
		"overage_requests": rec.OverageRequests,
		"overage_cost":     rec.OverageCost,
		"currency":         rec.Currency,
		"billed":           rec.Billed,
		"invoice_id":       rec.InvoiceID,
		"created_at":       rec.CreatedAt,
	}, nil
}

// PlatformRequest 是平台 API 的统一代理入口：由 Go 后端附加访问令牌后转发，
// 前端无需持有平台 Token，也不再直连平台地址（避免绕过 Wails 层）。
// 返回 { status, body }，status 为平台 HTTP 状态码，body 为解析后的响应体（非 JSON 时退回字符串）。
func (a *App) PlatformRequest(method, path, body string) (map[string]any, error) {
	var payload []byte
	if strings.TrimSpace(body) != "" {
		payload = []byte(body)
	}
	status, respBody, err := a.authSvc.DoPlatformRequest(method, path, payload)
	if err != nil {
		return nil, err
	}
	var parsed any
	if len(respBody) > 0 {
		if json.Unmarshal(respBody, &parsed) != nil {
			parsed = string(respBody)
		}
	}
	return map[string]any{
		"status": status,
		"body":   parsed,
	}, nil
}

func (a *App) FetchActiveGrants() ([]map[string]any, error) {
	grants, err := a.authSvc.FetchActiveGrants()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(grants))
	for _, g := range grants {
		result = append(result, map[string]any{
			"grant_id":          g.GrantID,
			"grantor_user_id":   g.GrantorUserID,
			"channel_id":        g.ChannelID,
			"token_fingerprint": g.TokenFingerprint,
			"granted_quota_usd": g.GrantedQuotaUSD,
			"used_quota_usd":    g.UsedQuotaUSD,
			"remaining_quota":   g.RemainingQuota,
			"granted_tokens":    g.GrantedTokens,
			"used_tokens":       g.UsedTokens,
			"remaining_tokens":  g.RemainingTokens,
			"status":            g.Status,
			"expires_at":        g.ExpiresAt,
		})
	}
	return result, nil
}

func (a *App) SetSelectedGrant(grantID, relayURL string) error {
	return a.configSvc.SetSelectedGrant(grantID, relayURL)
}

func (a *App) GetSelectedGrant() map[string]any {
	return map[string]any{
		"grant_id":  a.configSvc.GetSelectedGrantID(),
		"relay_url": a.configSvc.GetSelectedGrantRelayURL(),
	}
}

func (a *App) ClearSelectedGrant() error {
	return a.configSvc.SetSelectedGrant("", "")
}

func (a *App) GetPlans() ([]map[string]any, error) {
	if !a.authSvc.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	plans, err := a.authSvc.FetchPlans()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		entry := map[string]any{
			"plan_id":       p.PlanID,
			"name":          p.Name,
			"description":   p.Description,
			"price":         p.Price,
			"monthly_quota": p.MonthlyQuota,
			"max_rpm":       p.MaxRPM,
			"max_tpm":       p.MaxTPM,
			"sort_order":    p.SortOrder,
			"relay_enabled": p.RelayEnabled,
		}
		result = append(result, entry)
	}
	return result, nil
}

// FetchCloudCombos retrieves user combos from the cloud server.
func (a *App) FetchCloudCombos() ([]map[string]any, error) {
	combos, err := a.authSvc.FetchCloudCombos()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(combos))
	for _, c := range combos {
		steps := make([]map[string]any, 0, len(c.Steps))
		for _, s := range c.Steps {
			steps = append(steps, map[string]any{
				"channel_id": s.ChannelID,
				"model":      s.Model,
				"step_role":  s.StepRole,
			})
		}
		result = append(result, map[string]any{
			"combo_id":       c.ComboID,
			"scope":          c.Scope,
			"logical_name":   c.LogicalName,
			"display_name":   c.DisplayName,
			"description":    c.Description,
			"tags":           c.Tags,
			"strategy":       c.Strategy,
			"sticky_uses":    c.StickyUses,
			"quick_strategy": c.QuickStrategy,
			"task_profile":   c.TaskProfile,
			"steps":          steps,
			"status":         c.Status,
			"source":         c.Source,
			"version":        c.Version,
			"created_at":     c.CreatedAt,
			"updated_at":     c.UpdatedAt,
		})
	}
	return result, nil
}

// PushCloudCombo creates a user combo on the cloud server.
func (a *App) PushCloudCombo(logicalName, displayName, description, strategy string, stickyUses int, quickStrategy string, taskProfile map[string]any, stepsJSON string) (map[string]any, error) {
	combo, err := a.authSvc.PushCloudCombo(logicalName, displayName, description, strategy, stickyUses, quickStrategy, taskProfile, []byte(stepsJSON))
	if err != nil {
		return nil, err
	}
	if combo == nil {
		return nil, nil
	}
	return map[string]any{
		"combo_id":       combo.ComboID,
		"logical_name":   combo.LogicalName,
		"display_name":   combo.DisplayName,
		"description":    combo.Description,
		"strategy":       combo.Strategy,
		"sticky_uses":    combo.StickyUses,
		"quick_strategy": combo.QuickStrategy,
		"task_profile":   combo.TaskProfile,
		"status":         combo.Status,
		"version":        combo.Version,
	}, nil
}

// UpdateCloudCombo updates a user combo on the cloud server.
func (a *App) UpdateCloudCombo(comboID, displayName, description, strategy string, stickyUses int, status, quickStrategy string, taskProfile map[string]any, stepsJSON string) error {
	return a.authSvc.UpdateCloudCombo(comboID, displayName, description, strategy, stickyUses, status, quickStrategy, taskProfile, []byte(stepsJSON))
}

// DeleteCloudCombo deletes a user combo on the cloud server.
func (a *App) DeleteCloudCombo(comboID string) error {
	return a.authSvc.DeleteCloudCombo(comboID)
}
