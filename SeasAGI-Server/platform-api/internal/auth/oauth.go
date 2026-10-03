package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/database"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/i18n"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// oauthProviderConfig 描述一个 OAuth 提供方的端点与凭证（凭证只保存在服务端）。
type oauthProviderConfig struct {
	Name         string
	ClientID     string
	ClientSecret string
	AuthorizeURL string // 第三方授权页（浏览器 302 跳转目标）
	TokenURL     string
	UserInfoURL  string
	EmailsURL    string // 仅 GitHub 需要（用户主邮箱兜底）
}

// OAuthProviderInfo 下发给客户端的 provider 概要（不含密钥）。
type OAuthProviderInfo struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
}

type oauthUserInfo struct {
	ProviderUserID string
	Email          string
}

const oauthHTTPTimeout = 15 * time.Second

// oauthCallbackPath / oauthStartPath 是服务端对外暴露的 OAuth 回调与跳转页路径。
const (
	oauthCallbackPath = "/api/v1/auth/oauth/callback"
	oauthStartPath    = "/api/v1/auth/oauth/start"
)

// GetOAuthProviders 返回服务端已配置的第三方登录方式（供客户端动态渲染按钮）。
func GetOAuthProviders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"providers": listConfiguredProviders()})
}

// oauthSession 一次第三方登录会话：客户端先创建会话，浏览器完成授权后由客户端轮询取回结果。
type oauthSession struct {
	SessionID    string
	Provider     string
	State        string
	CodeVerifier string // Google 在服务端做 PKCE；GitHub OAuth App 不支持
	Status       string // pending | completed | failed
	AccessToken  string
	RefreshToken string
	Error        string
	CreatedAt    time.Time
}

const oauthSessionTTL = 10 * time.Minute

var (
	oauthSessionsMu sync.Mutex
	oauthSessions   = map[string]*oauthSession{}
)

func saveOAuthSession(s *oauthSession) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	oauthSessions[s.SessionID] = s
}

func getOAuthSession(id string) (*oauthSession, bool) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	s, ok := oauthSessions[id]
	return s, ok
}

func findOAuthSessionByState(state string) *oauthSession {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	for _, s := range oauthSessions {
		if s.State == state {
			return s
		}
	}
	return nil
}

func completeOAuthSession(id, accessToken, refreshToken string) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	if s, ok := oauthSessions[id]; ok {
		s.Status = "completed"
		s.AccessToken = accessToken
		s.RefreshToken = refreshToken
	}
}

func failOAuthSession(id, msg string) {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	if s, ok := oauthSessions[id]; ok {
		s.Status = "failed"
		s.Error = msg
	}
}

func cleanupExpiredOAuthSessions() {
	oauthSessionsMu.Lock()
	defer oauthSessionsMu.Unlock()
	now := time.Now()
	for id, s := range oauthSessions {
		if now.Sub(s.CreatedAt) > oauthSessionTTL {
			delete(oauthSessions, id)
		}
	}
}

// AuthorizeOAuth 创建一次第三方登录会话，返回浏览器要打开的授权跳转地址。
// 客户端流程：POST /auth/oauth/authorize -> 打开 authorize_url -> 授权完成后轮询 /auth/oauth/poll。
func AuthorizeOAuth(c *gin.Context) {
	var req struct {
		Provider string `json:"provider" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	providerName := strings.ToLower(strings.TrimSpace(req.Provider))
	cfg, ok := resolveProvider(providerName)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": i18n.TFromContext(c, "auth.oauthUnsupportedProvider")})
		return
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": i18n.TFromContext(c, "auth.oauthUnavailable")})
		return
	}

	sessionID, err := randomHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}
	state, err := randomHex(16)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
		return
	}
	// Google 使用服务端 PKCE（S256）保护授权码；GitHub OAuth App 不支持 PKCE。
	codeVerifier := ""
	if cfg.Name == "google" {
		if codeVerifier, err = randomHex(32); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": i18n.TFromContext(c, "auth.tokenGenerationFailed")})
			return
		}
	}

	cleanupExpiredOAuthSessions()
	saveOAuthSession(&oauthSession{
		SessionID:    sessionID,
		Provider:     cfg.Name,
		State:        state,
		CodeVerifier: codeVerifier,
		Status:       "pending",
		CreatedAt:    time.Now(),
	})

	authorizeURL := publicBaseURL() + oauthStartPath + "?session_id=" + url.QueryEscape(sessionID)
	c.JSON(http.StatusOK, gin.H{"session_id": sessionID, "authorize_url": authorizeURL})
}

// StartOAuth 是浏览器打开的授权跳转页：校验会话后 302 到第三方授权页。
func StartOAuth(c *gin.Context) {
	sessionID := c.Query("session_id")
	sess, ok := getOAuthSession(sessionID)
	if !ok || time.Since(sess.CreatedAt) > oauthSessionTTL {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8",
			[]byte(oauthPageHTML(i18n.TFromContext(c, "auth.oauthPageInvalidSession"))))
		return
	}
	cfg, ok := resolveProvider(sess.Provider)
	if !ok || cfg.ClientID == "" {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8",
			[]byte(oauthPageHTML(i18n.TFromContext(c, "auth.oauthUnavailable"))))
		return
	}

	redirectURI := publicBaseURL() + oauthCallbackPath
	params := url.Values{
		"client_id":     {cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {sess.State},
	}
	switch cfg.Name {
	case "google":
		params.Set("scope", "openid email profile")
		params.Set("code_challenge", oauthS256CodeChallenge(sess.CodeVerifier))
		params.Set("code_challenge_method", "S256")
	case "github":
		params.Set("scope", "read:user user:email")
	}
	c.Redirect(http.StatusFound, cfg.AuthorizeURL+"?"+params.Encode())
}

// OAuthCallback 是第三方授权完成后的回调：服务端换取用户信息、落库并签发平台 JWT。
func OAuthCallback(c *gin.Context) {
	sess := findOAuthSessionByState(c.Query("state"))
	if sess == nil {
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8",
			[]byte(oauthPageHTML(i18n.TFromContext(c, "auth.oauthPageInvalidSession"))))
		return
	}

	fail := func(msg string) {
		failOAuthSession(sess.SessionID, msg)
		page := i18n.TFromContext(c, "auth.oauthPageFailed")
		c.Data(http.StatusOK, "text/html; charset=utf-8",
			[]byte(oauthPageHTML(strings.ReplaceAll(page, "%s", msg))))
	}

	if errParam := c.Query("error"); errParam != "" {
		fail(i18n.TFromContext(c, "auth.oauthDenied"))
		return
	}

	code := c.Query("code")
	if code == "" {
		fail(i18n.TFromContext(c, "auth.oauthExchangeFailed"))
		return
	}

	cfg, ok := resolveProvider(sess.Provider)
	if !ok || cfg.ClientID == "" || cfg.ClientSecret == "" {
		fail(i18n.TFromContext(c, "auth.oauthUnavailable"))
		return
	}

	providerToken, err := exchangeAuthorizationCode(cfg, code, publicBaseURL()+oauthCallbackPath, sess.CodeVerifier)
	if err != nil {
		fail(i18n.TFromContext(c, "auth.oauthExchangeFailed"))
		return
	}

	info, err := fetchUserInfo(cfg, providerToken)
	if err != nil {
		fail(i18n.TFromContext(c, "auth.oauthUserInfoFailed"))
		return
	}

	userID, err := findOrCreateUserByIdentity(cfg.Name, info.ProviderUserID, info.Email)
	if err != nil {
		fail(i18n.TFromContext(c, "auth.tokenGenerationFailed"))
		return
	}

	accessToken, err := generateAccessToken(userID, info.Email)
	if err != nil {
		fail(i18n.TFromContext(c, "auth.tokenGenerationFailed"))
		return
	}
	refreshToken, err := generateRefreshToken(userID)
	if err != nil {
		fail(i18n.TFromContext(c, "auth.tokenGenerationFailed"))
		return
	}

	completeOAuthSession(sess.SessionID, accessToken, refreshToken)
	c.Data(http.StatusOK, "text/html; charset=utf-8",
		[]byte(oauthPageHTML(i18n.TFromContext(c, "auth.oauthPageSuccess"))))
}

// PollOAuth 供客户端轮询登录结果：pending / completed（带平台 JWT）/ failed（带原因）。
func PollOAuth(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cleanupExpiredOAuthSessions()

	sess, ok := getOAuthSession(req.SessionID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": i18n.TFromContext(c, "auth.oauthSessionNotFound")})
		return
	}
	switch sess.Status {
	case "completed":
		c.JSON(http.StatusOK, gin.H{
			"status":        "completed",
			"access_token":  sess.AccessToken,
			"refresh_token": sess.RefreshToken,
			"expires_in":    86400,
		})
	case "failed":
		c.JSON(http.StatusOK, gin.H{"status": "failed", "error": sess.Error})
	default:
		c.JSON(http.StatusOK, gin.H{"status": "pending"})
	}
}

// resolveProvider 根据名称返回 provider 配置与凭证（来自环境变量）。
func resolveProvider(name string) (*oauthProviderConfig, bool) {
	switch strings.ToLower(name) {
	case "google":
		return &oauthProviderConfig{
			Name:         "google",
			ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
			ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
			AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:     "https://oauth2.googleapis.com/token",
			UserInfoURL:  "https://www.googleapis.com/oauth2/v3/userinfo",
		}, true
	case "github":
		return &oauthProviderConfig{
			Name:         "github",
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			AuthorizeURL: "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserInfoURL:  "https://api.github.com/user",
			EmailsURL:    "https://api.github.com/user/emails",
		}, true
	}
	return nil, false
}

// listConfiguredProviders 返回 client_id 与 client_secret 均已配置的 provider。
func listConfiguredProviders() []OAuthProviderInfo {
	providers := []OAuthProviderInfo{}
	for _, name := range []string{"google", "github"} {
		if cfg, ok := resolveProvider(name); ok && cfg.ClientID != "" && cfg.ClientSecret != "" {
			providers = append(providers, OAuthProviderInfo{Name: cfg.Name, ClientID: cfg.ClientID})
		}
	}
	return providers
}

// publicBaseURL 返回服务端对外可达的基础地址（用于拼接回调与授权跳转页）。
func publicBaseURL() string {
	base := strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("API_BASE_URL"))
	}
	if base == "" {
		base = "http://localhost:9318"
	}
	return strings.TrimRight(base, "/")
}

// oauthS256CodeChallenge 计算 PKCE S256 code_challenge。
func oauthS256CodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// exchangeAuthorizationCode 用授权码向 provider 换取 access_token。
func exchangeAuthorizationCode(cfg *oauthProviderConfig, code, redirectURI, codeVerifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
	}
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	httpReq, err := http.NewRequest(http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: oauthHTTPTimeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("empty access token")
	}
	return tokenResp.AccessToken, nil
}

// fetchUserInfo 拉取 provider 用户信息，返回稳定的 provider_user_id 与邮箱。
func fetchUserInfo(cfg *oauthProviderConfig, accessToken string) (*oauthUserInfo, error) {
	body, err := oauthGetJSON(cfg.UserInfoURL, accessToken)
	if err != nil {
		return nil, err
	}

	info := &oauthUserInfo{}
	switch cfg.Name {
	case "google":
		var payload struct {
			Sub   string `json:"sub"`
			Email string `json:"email"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		if payload.Sub == "" {
			return nil, fmt.Errorf("missing sub")
		}
		info.ProviderUserID = payload.Sub
		info.Email = payload.Email
	case "github":
		var payload struct {
			ID    int64  `json:"id"`
			Email string `json:"email"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		if payload.ID == 0 {
			return nil, fmt.Errorf("missing id")
		}
		info.ProviderUserID = strconv.FormatInt(payload.ID, 10)
		info.Email = payload.Email
		if info.Email == "" {
			email, err := fetchGitHubPrimaryEmail(cfg, accessToken)
			if err != nil {
				return nil, err
			}
			info.Email = email
		}
	default:
		return nil, fmt.Errorf("unsupported provider")
	}

	if info.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	return info, nil
}

// fetchGitHubPrimaryEmail 从 /user/emails 兜底获取主邮箱。
func fetchGitHubPrimaryEmail(cfg *oauthProviderConfig, accessToken string) (string, error) {
	body, err := oauthGetJSON(cfg.EmailsURL, accessToken)
	if err != nil {
		return "", err
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	return "", fmt.Errorf("no verified email")
}

func oauthGetJSON(endpoint, accessToken string) ([]byte, error) {
	httpReq, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	httpReq.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: oauthHTTPTimeout}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}
	return body, nil
}

// findOrCreateUserByIdentity 按 identity 复用 -> email 绑定 -> 新建用户的顺序解析平台账号。
func findOrCreateUserByIdentity(provider, providerUserID, email string) (string, error) {
	if database.DB == nil {
		return "", fmt.Errorf("database not initialized")
	}
	if providerUserID == "" || email == "" {
		return "", fmt.Errorf("provider user id and email are required")
	}
	email = strings.ToLower(email)

	var userID string
	err := database.DB.QueryRow(
		`SELECT user_id FROM oauth_identities WHERE provider = ? AND provider_user_id = ?`,
		provider, providerUserID,
	).Scan(&userID)
	if err == nil {
		return userID, nil
	}

	// 已有同邮箱的平台账号则绑定 identity，不新建。
	err = database.DB.QueryRow(`SELECT user_id FROM users WHERE email = ?`, email).Scan(&userID)
	if err == nil {
		if _, err := database.DB.Exec(
			`INSERT INTO oauth_identities (user_id, provider, provider_user_id, email) VALUES (?, ?, ?, ?)`,
			userID, provider, providerUserID, email,
		); err != nil {
			return "", fmt.Errorf("bind identity failed: %w", err)
		}
		return userID, nil
	}

	// 新用户：随机占位密码（无法用密码登录，只能走 OAuth）。
	newUserID := fmt.Sprintf("user_%d", time.Now().UnixNano())
	placeholder, err := randomHex(32)
	if err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(placeholder), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	if _, err := database.DB.Exec(
		`INSERT INTO users (user_id, email, hashed_password) VALUES (?, ?, ?)`,
		newUserID, email, string(hashed),
	); err != nil {
		return "", fmt.Errorf("create user failed: %w", err)
	}
	if _, err := database.DB.Exec(
		`INSERT INTO oauth_identities (user_id, provider, provider_user_id, email) VALUES (?, ?, ?, ?)`,
		newUserID, provider, providerUserID, email,
	); err != nil {
		return "", fmt.Errorf("create identity failed: %w", err)
	}
	return newUserID, nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// oauthPageHTML 生成授权结果提示页（浏览器关闭前给用户的反馈）。
func oauthPageHTML(message string) string {
	return `<!doctype html><html><head><meta charset="utf-8">` +
		`<title>SeasAGI</title>` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<style>body{font-family:-apple-system,'Segoe UI',Roboto,'PingFang SC','Microsoft YaHei',sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#0f1115;color:#e8eaed}` +
		`.card{text-align:center;padding:40px 48px;border-radius:12px;background:#1b1f27;box-shadow:0 8px 30px rgba(0,0,0,.35)}` +
		`h1{font-size:20px;margin:0 0 12px}p{margin:0;color:#9aa0a6;font-size:14px;max-width:420px;line-height:1.6}</style></head>` +
		`<body><div class="card"><h1>SeasAGI</h1><p>` + html.EscapeString(message) + `</p></div></body></html>`
}
