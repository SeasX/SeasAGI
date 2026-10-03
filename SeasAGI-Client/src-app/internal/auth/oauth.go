package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
)

// OAuthProvider 服务端已配置的第三方登录方式。
type OAuthProvider struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
}

const oauthHTTPTimeout = 8 * time.Second

// oauthLoginTimeout / oauthPollInterval 抽为包级变量，便于单元测试替换。
var (
	oauthLoginTimeout = 5 * time.Minute
	oauthPollInterval = time.Second
)

// FetchOAuthProviders 获取企业服务端已配置的第三方登录方式（用于前端动态渲染按钮）。
// 未配置 ENTERPRISE_API_BASE_URL 时返回空列表，注册/登录页不展示第三方登录按钮。
func (s *Service) FetchOAuthProviders() ([]OAuthProvider, error) {
	base := enterpriseAPIBaseURL()
	if base == "" {
		return nil, nil
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), oauthHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"/auth/oauth/providers", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Providers []OAuthProvider `json:"providers"`
		Error     string          `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "fetch oauth providers failed"
		}
		return nil, fmt.Errorf("%s", result.Error)
	}
	return result.Providers, nil
}

// StartOAuthLogin 执行企业服务端中介的第三方 OAuth 登录流程：
// 1. 请求企业服务端创建授权会话，得到浏览器要打开的授权跳转地址；
// 2. 打开浏览器访问企业服务端授权跳转页（企业服务端 302 到 Google/GitHub，授权后回调换码并签发平台 JWT）；
// 3. 轮询企业服务端取回登录态并持久化。
func (s *Service) StartOAuthLogin(providerName string) error {
	if providerName != "google" && providerName != "github" {
		return fmt.Errorf("unsupported oauth provider: %s", providerName)
	}

	s.mu.Lock()
	if s.oauthRunning {
		s.mu.Unlock()
		return fmt.Errorf("oauth login already in progress")
	}
	s.oauthRunning = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.oauthRunning = false
		s.mu.Unlock()
	}()

	sessionID, authorizeURL, err := s.authorizeOAuth(providerName)
	if err != nil {
		return err
	}

	if err := openBrowser(authorizeURL); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}

	token, err := s.pollOAuthToken(sessionID)
	if err != nil {
		return err
	}

	userID := s.extractUserID(token)
	s.mu.Lock()
	s.info = AuthInfo{
		IsLoggedIn: true,
		UserID:     &userID,
	}
	s.token = token
	s.mu.Unlock()
	_ = savePlatformToken(token)

	return nil
}

// authorizeOAuth 请求企业服务端创建 OAuth 会话，返回会话 ID 与浏览器要打开的授权跳转地址。
func (s *Service) authorizeOAuth(provider string) (sessionID, authorizeURL string, err error) {
	base := enterpriseAPIBaseURL()
	if base == "" {
		return "", "", fmt.Errorf("enterprise api base url is not configured")
	}

	payload, err := json.Marshal(map[string]string{"provider": provider})
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequest(http.MethodPost, base+"/auth/oauth/authorize", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var result struct {
		SessionID    string `json:"session_id"`
		AuthorizeURL string `json:"authorize_url"`
		Error        string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "oauth authorize failed"
		}
		return "", "", fmt.Errorf("%s", result.Error)
	}
	if result.SessionID == "" || result.AuthorizeURL == "" {
		return "", "", fmt.Errorf("oauth authorize returned empty session")
	}
	return result.SessionID, result.AuthorizeURL, nil
}

// pollOAuthToken 轮询服务端直到授权完成、失败或超时，返回平台 JWT。
func (s *Service) pollOAuthToken(sessionID string) (string, error) {
	deadline := time.Now().Add(oauthLoginTimeout)
	for {
		token, done, err := s.pollOnce(sessionID)
		if err != nil {
			return "", err
		}
		if done {
			return token, nil
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("oauth login timed out")
		}
		time.Sleep(oauthPollInterval)
	}
}

// pollOnce 发起一次轮询。done 为 true 时 token 携带平台 JWT（completed）或 err 说明失败原因。
func (s *Service) pollOnce(sessionID string) (token string, done bool, err error) {
	base := enterpriseAPIBaseURL()
	if base == "" {
		return "", false, fmt.Errorf("enterprise api base url is not configured")
	}

	payload, err := json.Marshal(map[string]string{"session_id": sessionID})
	if err != nil {
		return "", false, err
	}

	req, err := http.NewRequest(http.MethodPost, base+"/auth/oauth/poll", bytes.NewReader(payload))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()

	var result struct {
		Status      string `json:"status"`
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", false, err
	}
	if resp.StatusCode >= 400 {
		if result.Error == "" {
			result.Error = "oauth poll failed"
		}
		return "", false, fmt.Errorf("%s", result.Error)
	}
	switch result.Status {
	case "completed":
		if result.AccessToken == "" {
			return "", false, fmt.Errorf("platform returned empty access token")
		}
		return result.AccessToken, true, nil
	case "failed":
		if result.Error == "" {
			result.Error = "oauth authorization failed"
		}
		return "", false, fmt.Errorf("%s", result.Error)
	default:
		return "", false, nil
	}
}

// savePlatformToken / openBrowser 抽为包级变量，便于单元测试替换。
var savePlatformToken = keychain.SavePlatformToken

var openBrowser = func(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", rawURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return exec.Command("xdg-open", rawURL).Start()
	}
}
