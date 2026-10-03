package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
)

type AuthInfo struct {
	IsLoggedIn bool    `json:"is_logged_in"`
	UserID     *string `json:"user_id"`
	Email      *string `json:"email"`
}

type Service struct {
	mu           sync.RWMutex
	info         AuthInfo
	token        string
	oauthRunning bool
}

func NewService() *Service {
	token, _ := keychain.GetPlatformToken()
	svc := &Service{
		info:  AuthInfo{IsLoggedIn: false},
		token: token,
	}
	if token != "" {
		userID := svc.extractUserID(token)
		svc.info = AuthInfo{
			IsLoggedIn: true,
			UserID:     &userID,
		}
	}
	return svc
}

func (s *Service) Login(email, password string) error {
	token, err := s.callPlatformLogin(email, password)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	userID := s.extractUserID(token)
	s.info = AuthInfo{
		IsLoggedIn: true,
		UserID:     &userID,
		Email:      &email,
	}
	s.token = token
	_ = keychain.SavePlatformToken(token)

	return nil
}

func (s *Service) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.info = AuthInfo{IsLoggedIn: false}
	s.token = ""
	_ = keychain.ClearPlatformToken()
	return nil
}

func (s *Service) GetAuthState() AuthInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info
}

func (s *Service) IsLoggedIn() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.info.IsLoggedIn
}

func (s *Service) GetPlatformToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

func (s *Service) callPlatformLogin(email, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, platformAPIBaseURL()+"/auth/login", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "platform login failed"
		}
		return "", errors.New(result.Error)
	}
	if result.AccessToken == "" {
		return "", errors.New("platform returned empty access token")
	}
	return result.AccessToken, nil
}

func (s *Service) extractUserID(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}

func (s *Service) FetchPlatformChannels(ctx context.Context) ([]map[string]interface{}, error) {
	if !s.IsLoggedIn() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, platformAPIBaseURL()+"/channels", nil)
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
		Data  []map[string]interface{} `json:"data"`
		Error string                   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch platform channels failed"
		}
		return nil, errors.New(result.Error)
	}
	return result.Data, nil
}

// FetchFreeChannels 从企业服务端（优先）或平台 API 拉取免费通道种子列表。
// 企业服务端配置了 ENTERPRISE_API_BASE_URL 时优先使用，降级使用开源平台 API。
func (s *Service) FetchFreeChannels(ctx context.Context) ([]map[string]interface{}, error) {
	if !s.IsLoggedIn() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	// 优先从企业服务端拉取
	enterpriseURL := enterpriseAPIBaseURL()
	if enterpriseURL != "" {
		data, err := s.fetchFreeChannelsFrom(reqCtx, enterpriseURL)
		if err == nil && len(data) > 0 {
			return data, nil
		}
		// 企业服务端失败或无数据，降级到平台 API
	}

	// 降级：从开源平台 API 拉取
	return s.fetchFreeChannelsFrom(reqCtx, platformAPIBaseURL())
}

// FetchEnterpriseChannels 从企业服务端拉取通道列表（含 provider_type, base_url, models）。
func (s *Service) FetchEnterpriseChannels(ctx context.Context) ([]map[string]interface{}, error) {
	if !s.IsLoggedIn() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	enterpriseURL := enterpriseAPIBaseURL()
	if enterpriseURL == "" {
		return nil, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, enterpriseURL+"/channels", nil)
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
		Data  []map[string]interface{} `json:"data"`
		Error string                   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch enterprise channels failed"
		}
		return nil, errors.New(result.Error)
	}
	return result.Data, nil
}

// fetchFreeChannelsFrom 从指定 base URL 拉取免费通道种子列表。
func (s *Service) fetchFreeChannelsFrom(ctx context.Context, baseURL string) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/free-channels", nil)
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
		Data  []map[string]interface{} `json:"data"`
		Error string                   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch free channels failed"
		}
		return nil, errors.New(result.Error)
	}
	return result.Data, nil
}

// FetchModelCatalog 从企业服务端拉取模型目录列表。
// 返回每个模型的元数据（provider、display_name、capabilities、价格等），
// 供通道配置页面按 provider 自动补全可用模型。
func (s *Service) FetchModelCatalog(ctx context.Context) ([]map[string]interface{}, error) {
	if !s.IsLoggedIn() {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	enterpriseURL := enterpriseAPIBaseURL()
	if enterpriseURL == "" {
		return nil, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, enterpriseURL+"/models", nil)
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
		Data  []map[string]interface{} `json:"data"`
		Error string                   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch model catalog failed"
		}
		return nil, errors.New(result.Error)
	}
	return result.Data, nil
}

// FetchModelIndex 拉取 AI 模型指数公开榜单（企业服务端优先，降级开源平台 API）。
// 该端点为公开资讯内容，无需登录；category 为空时服务端返回综合榜。
func (s *Service) FetchModelIndex(ctx context.Context, category string) (map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	reqCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	base := enterpriseAPIBaseURL()
	if base == "" {
		base = platformAPIBaseURL()
	}
	endpoint := base + "/model-index"
	if category != "" {
		endpoint += "?category=" + url.QueryEscape(category)
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if s.GetPlatformToken() != "" {
		req.Header.Set("Authorization", "Bearer "+s.GetPlatformToken())
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data  map[string]interface{} `json:"data"`
		Error string                 `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch model index failed"
		}
		return nil, errors.New(result.Error)
	}
	return result.Data, nil
}

func (s *Service) PlatformAPIBaseURL() string {
	return platformAPIBaseURL()
}

func platformAPIBaseURL() string {
	if value := os.Getenv("PLATFORM_API_BASE_URL"); value != "" {
		return value
	}
	if value := getConfiguredBaseURL(); value != "" {
		return value
	}
	return "https://seasagi.seasx.ai/api/v1"
}

// enterpriseAPIBaseURL 返回企业服务端 API 基础 URL。
// 通过 ENTERPRISE_API_BASE_URL 环境变量配置，未配置时返回空字符串（表示使用开源平台 API）。
func enterpriseAPIBaseURL() string {
	return os.Getenv("ENTERPRISE_API_BASE_URL")
}
