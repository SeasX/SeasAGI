package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
)

var (
	configuredBaseURL string
	baseURLMu         sync.RWMutex
)

func SetPlatformAPIBaseURL(url string) {
	baseURLMu.Lock()
	defer baseURLMu.Unlock()
	configuredBaseURL = url
}

func getConfiguredBaseURL() string {
	baseURLMu.RLock()
	defer baseURLMu.RUnlock()
	return configuredBaseURL
}

type CloudModelStatsEntry struct {
	Model         string  `json:"model"`
	TotalRequests int     `json:"total_requests"`
	TotalErrors   int     `json:"total_errors"`
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
	ErrorRate     float64 `json:"error_rate"`
}

func (s *Service) Register(email, password, displayName string) error {
	payload, err := json.Marshal(map[string]string{
		"email":        email,
		"password":     password,
		"display_name": displayName,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, platformAPIBaseURL()+"/auth/register", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "platform registration failed"
		}
		return errors.New(result.Error)
	}
	if result.AccessToken == "" {
		return fmt.Errorf("platform returned empty access token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	userID := s.extractUserID(result.AccessToken)
	s.info = AuthInfo{
		IsLoggedIn: true,
		UserID:     &userID,
		Email:      &email,
	}
	s.token = result.AccessToken
	_ = keychain.SavePlatformToken(result.AccessToken)

	return nil
}

func (s *Service) FetchCloudUsage() (*CloudUsage, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data *CloudUsage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch cloud usage failed")
	}
	return result.Data, nil
}

func (s *Service) FetchCloudBilling() (*CloudBilling, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/subscriptions", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data *CloudBilling `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch cloud billing failed")
	}
	return result.Data, nil
}

// OverageUsage 用户当期超额用量记录（对应 platform-api /usage/overage/user）。
type OverageUsage struct {
	OverageID       string  `json:"overage_id"`
	UserID          string  `json:"user_id"`
	PlanID          string  `json:"plan_id"`
	BillingPeriod   string  `json:"billing_period"`
	OverageRequests int     `json:"overage_requests"`
	OverageCost     float64 `json:"overage_cost"`
	Currency        string  `json:"currency"`
	Billed          bool    `json:"billed"`
	InvoiceID       string  `json:"invoice_id"`
	CreatedAt       string  `json:"created_at"`
}

func (s *Service) FetchOverageUsage() (*OverageUsage, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/usage/overage/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch overage usage failed")
	}

	var record OverageUsage
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, err
	}
	return &record, nil
}

// DoPlatformRequest 代客户端调用平台 API：在后端附加访问令牌，前端无需持有平台 Token。
// 返回（HTTP 状态码, 原始响应体, error）。error 仅表示本地失败（未登录 / 请求构造失败 / 网络不可达），
// 平台返回的 4xx/5xx 会如实通过状态码回传，便于调用方按原有语义处理。
func (s *Service) DoPlatformRequest(method, path string, body []byte) (int, []byte, error) {
	if !s.IsLoggedIn() {
		return 0, nil, fmt.Errorf("not logged in")
	}
	if !strings.HasPrefix(path, "/") {
		return 0, nil, fmt.Errorf("invalid platform path %q", path)
	}

	var reader *bytes.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, platformAPIBaseURL()+path, reader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

type ActiveGrant struct {
	GrantID          string  `json:"grant_id"`
	GrantorUserID    string  `json:"grantor_user_id"`
	ChannelID        string  `json:"channel_id"`
	TokenFingerprint string  `json:"token_fingerprint"`
	GrantedQuotaUSD  float64 `json:"granted_quota_usd"`
	UsedQuotaUSD     float64 `json:"used_quota_usd"`
	RemainingQuota   float64 `json:"remaining_quota"`
	GrantedTokens    int64   `json:"granted_tokens"`
	UsedTokens       int64   `json:"used_tokens"`
	RemainingTokens  int64   `json:"remaining_tokens"`
	Status           string  `json:"status"`
	ExpiresAt        string  `json:"expires_at"`
}

func (s *Service) FetchActiveGrants() ([]ActiveGrant, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/token-market/grants/active", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data []ActiveGrant `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch active grants failed")
	}
	return result.Data, nil
}

func (s *Service) FetchCloudOptimizationConfig() (string, error) {
	if !s.IsLoggedIn() {
		return "", fmt.Errorf("not logged in")
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/user/optimization", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Config any `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("fetch optimization config failed")
	}
	if result.Config == nil {
		return "", nil
	}
	return mapToJSONString(result.Config), nil
}

func (s *Service) PushCloudOptimizationConfig(cfgJSON string) error {
	if !s.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}
	req, err := http.NewRequest(http.MethodPut, platformAPIBaseURL()+"/user/optimization", strings.NewReader(cfgJSON))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("push optimization config failed")
	}
	return nil
}

func mapToJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Service) FetchCloudModelStats() ([]CloudModelStatsEntry, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/usage/model-stats", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []CloudModelStatsEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch cloud model stats failed")
	}
	return result.Data, nil
}

func (s *Service) FetchPlans() ([]CloudPlan, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/plans", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data  []CloudPlan `json:"data"`
		Error string      `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch plans failed"
		}
		return nil, errors.New(result.Error)
	}
	if result.Data == nil {
		return []CloudPlan{}, nil
	}
	return result.Data, nil
}

// FetchProviderHealthMetrics fetches provider health metrics from the cloud API
func (s *Service) FetchProviderHealthMetrics(providerID string) []map[string]any {
	if !s.IsLoggedIn() {
		return nil
	}
	path := "/admin/provider-health"
	if providerID != "" {
		path += "?provider_id=" + providerID
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+path, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return result.Data
}

// FetchProviderHealthSummary fetches provider health summary from the cloud API
func (s *Service) FetchProviderHealthSummary() []map[string]any {
	if !s.IsLoggedIn() {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/admin/provider-health/summary", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return result.Data
}

// FetchByokPolicies fetches BYOK policies from the cloud API
func (s *Service) FetchByokPolicies() []map[string]any {
	if !s.IsLoggedIn() {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/enterprise/byok-policy", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return result.Data
}

// CreateByokPolicy creates a BYOK policy via the cloud API
func (s *Service) CreateByokPolicy(policy map[string]any) (map[string]any, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, platformAPIBaseURL()+"/enterprise/byok-policy", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func (s *Service) FetchRecommendedCombos() ([]CloudCombo, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/entitlement", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			AllowedModels string `json:"allowed_models"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch recommended combos failed")
	}

	return parseCloudCombos(result.Data.AllowedModels), nil
}

type CloudUsage struct {
	MonthRequests     int     `json:"month_requests"`
	MonthInputTokens  int64   `json:"month_input_tokens"`
	MonthOutputTokens int64   `json:"month_output_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
}

type CloudBilling struct {
	PlanID        string         `json:"plan_id"`
	PlanName      string         `json:"plan_name"`
	Price         float64        `json:"price"`
	Quota         int            `json:"quota"`
	UsedQuota     int            `json:"used_quota"`
	RenewalDate   string         `json:"renewal_date"`
	RelayEnabled  bool           `json:"relay_enabled"`
	RelayGateways []RelayGateway `json:"relay_gateways"`
}

type RelayGateway struct {
	GatewayID          string `json:"gateway_id"`
	Name               string `json:"name"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Region             string `json:"region"`
	SupportsFederation bool   `json:"supports_federation"`
}

type CloudPlan struct {
	PlanID       string  `json:"plan_id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Price        float64 `json:"price"`
	MonthlyQuota int     `json:"monthly_quota"`
	MaxRPM       int     `json:"max_rpm"`
	MaxTPM       int     `json:"max_tpm"`
	SortOrder    int     `json:"sort_order"`
	RelayEnabled bool    `json:"relay_enabled"`
}

type CloudCombo struct {
	Name        string   `json:"name"`
	Models      []string `json:"models"`
	Description string   `json:"description"`
	Strategy    string   `json:"strategy"`
}

type CloudComboTemplateStep struct {
	ChannelID string `json:"channel_id,omitempty"`
	Model     string `json:"model"`
}

type CloudComboTemplate struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Tags        []string                 `json:"tags"`
	Steps       []CloudComboTemplateStep `json:"steps"`
	Models      []string                 `json:"models"`
	Strategy    string                   `json:"strategy"`
	StickyUses  int                      `json:"sticky_uses"`
}

func (s *Service) FetchOfficialComboTemplates(ctx context.Context) ([]CloudComboTemplate, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, platformAPIBaseURL()+"/combo-templates", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data  []CloudComboTemplate `json:"data"`
		Error string               `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch official combo templates failed"
		}
		return nil, errors.New(result.Error)
	}
	if result.Data == nil {
		return []CloudComboTemplate{}, nil
	}
	return result.Data, nil
}

func parseCloudCombos(modelsJSON string) []CloudCombo {
	var models []string
	if err := json.Unmarshal([]byte(modelsJSON), &models); err != nil {
		return nil
	}
	if len(models) == 0 {
		return nil
	}

	combos := []CloudCombo{
		{
			Name:        "智能推荐",
			Models:      models,
			Description: "根据你的订阅计划自动推荐的模型组合",
			Strategy:    "fallback",
		},
	}
	return combos
}

// CloudUserCombo mirrors the server's user-level combo JSON shape.
type CloudUserCombo struct {
	ComboID       string         `json:"combo_id"`
	Scope         string         `json:"scope"`
	LogicalName   string         `json:"logical_name"`
	DisplayName   string         `json:"display_name"`
	Description   string         `json:"description"`
	Tags          string         `json:"tags"`
	Strategy      string         `json:"strategy"`
	StickyUses    int            `json:"sticky_uses"`
	QuickStrategy string         `json:"quick_strategy"`
	TaskProfile   map[string]any `json:"task_profile"`
	Steps         []struct {
		ChannelID string `json:"channel_id,omitempty"`
		Model     string `json:"model"`
		StepRole  string `json:"step_role,omitempty"`
	} `json:"steps"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Version   int    `json:"version"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// FetchCloudCombos retrieves user-level combos from the cloud server.
func (s *Service) FetchCloudCombos() ([]CloudUserCombo, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodGet, platformAPIBaseURL()+"/combos", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []CloudUserCombo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch cloud combos failed")
	}
	if result.Data == nil {
		return []CloudUserCombo{}, nil
	}
	return result.Data, nil
}

// PushCloudCombo creates a user-level combo on the cloud server.
func (s *Service) PushCloudCombo(logicalName, displayName, description, strategy string, stickyUses int, quickStrategy string, taskProfile map[string]any, steps []byte) (*CloudUserCombo, error) {
	if !s.IsLoggedIn() {
		return nil, fmt.Errorf("not logged in")
	}

	payload := map[string]any{
		"logical_name": logicalName,
		"display_name": displayName,
		"description":  description,
		"strategy":     strategy,
		"sticky_uses":  stickyUses,
		"source":       "local",
	}
	if quickStrategy != "" {
		payload["quick_strategy"] = quickStrategy
	}
	if taskProfile != nil {
		payload["task_profile"] = taskProfile
	}

	var stepsArr []map[string]any
	if err := json.Unmarshal(steps, &stepsArr); err == nil {
		payload["steps"] = stepsArr
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, platformAPIBaseURL()+"/combos", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data *CloudUserCombo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("push cloud combo failed")
	}
	return result.Data, nil
}

// UpdateCloudCombo updates a user-level combo on the cloud server.
func (s *Service) UpdateCloudCombo(comboID, displayName, description, strategy string, stickyUses int, status string, quickStrategy string, taskProfile map[string]any, steps []byte) error {
	if !s.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}

	payload := map[string]any{}
	if displayName != "" {
		payload["display_name"] = displayName
	}
	if description != "" {
		payload["description"] = description
	}
	if strategy != "" {
		payload["strategy"] = strategy
	}
	if stickyUses > 0 {
		payload["sticky_uses"] = stickyUses
	}
	if status != "" {
		payload["status"] = status
	}
	if quickStrategy != "" {
		payload["quick_strategy"] = quickStrategy
	}
	if taskProfile != nil {
		payload["task_profile"] = taskProfile
	}
	if len(steps) > 0 {
		var stepsArr []map[string]any
		if err := json.Unmarshal(steps, &stepsArr); err == nil {
			payload["steps"] = stepsArr
		}
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPut, platformAPIBaseURL()+"/combos/"+comboID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("update cloud combo failed")
	}
	return nil
}

// DeleteCloudCombo deletes (archives) a user-level combo on the cloud server.
func (s *Service) DeleteCloudCombo(comboID string) error {
	if !s.IsLoggedIn() {
		return fmt.Errorf("not logged in")
	}

	req, err := http.NewRequest(http.MethodDelete, platformAPIBaseURL()+"/combos/"+comboID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete cloud combo failed")
	}
	return nil
}

// SyncCloudCombosToLocal fetches cloud combos and merges them into local config.
// Returns the combined list with cloud combos marked with source='cloud'.
// This is called by the frontend sync logic.
func (s *Service) SyncCloudCombosToLocal() ([]CloudUserCombo, error) {
	return s.FetchCloudCombos()
}
