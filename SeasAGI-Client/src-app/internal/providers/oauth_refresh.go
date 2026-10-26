package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// OAuthTokenInfo holds a cached OAuth token with its expiry.
type OAuthTokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
}

// ShouldRefresh returns true if the token is expired or will expire within the
// safety margin (default 60 s).
func (t *OAuthTokenInfo) ShouldRefresh(margin time.Duration) bool {
	if t == nil || t.AccessToken == "" {
		return true
	}
	return time.Until(t.ExpiresAt) <= margin
}

// OAuthTokenRefresher manages per-channel OAuth token refresh.
// It caches access tokens and refreshes them before expiry using the
// stored refresh token.
type OAuthTokenRefresher struct {
	mu         sync.RWMutex
	tokens     map[string]*OAuthTokenInfo // channelID → token info
	margin     time.Duration              // refresh safety margin
	httpClient *http.Client

	// 同一渠道的并发刷新合并为一次，避免提供方因重复使用 refresh token 而吊销令牌（CL-11）。
	refreshMu  sync.Mutex
	refreshing map[string]*refreshCall

	// persistFn 在提供方轮换 refresh token 时回写持久化存储（keychain）。
	persistFn func(channelID, refreshToken string) error
}

// refreshCall 表示一次进行中的刷新，等待方共享其结果。
type refreshCall struct {
	done  chan struct{}
	token *OAuthTokenInfo
	err   error
}

// SetRefreshTokenPersist 安装 refresh token 轮换后的回写钩子。
func (r *OAuthTokenRefresher) SetRefreshTokenPersist(fn func(channelID, refreshToken string) error) {
	r.mu.Lock()
	r.persistFn = fn
	r.mu.Unlock()
}

// NewOAuthTokenRefresher creates a new refresher with the given safety margin.
func NewOAuthTokenRefresher(margin time.Duration) *OAuthTokenRefresher {
	if margin <= 0 {
		margin = 60 * time.Second
	}
	return &OAuthTokenRefresher{
		tokens:     make(map[string]*OAuthTokenInfo),
		margin:     margin,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		refreshing: make(map[string]*refreshCall),
	}
}

// GetToken returns the cached token for the channel, or nil if none.
func (r *OAuthTokenRefresher) GetToken(channelID string) *OAuthTokenInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.tokens[channelID]; ok {
		cp := *t
		return &cp
	}
	return nil
}

// SetToken caches a token for the channel (e.g. after initial OAuth flow).
func (r *OAuthTokenRefresher) SetToken(channelID string, token *OAuthTokenInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if token == nil {
		delete(r.tokens, channelID)
		return
	}
	cp := *token
	r.tokens[channelID] = &cp
}

// EnsureValidToken returns a valid access token for the channel, refreshing
// if necessary. If no refresh token is available it returns the cached token
// as-is (the caller should handle a potentially expired token).
func (r *OAuthTokenRefresher) EnsureValidToken(channelID, refreshToken, clientID, clientSecret, tokenURL string) (string, error) {
	cached := r.GetToken(channelID)
	if cached != nil && !cached.ShouldRefresh(r.margin) {
		return cached.AccessToken, nil
	}

	if refreshToken == "" || tokenURL == "" {
		if cached != nil {
			return cached.AccessToken, nil
		}
		return "", fmt.Errorf("no cached token and no refresh token for channel %s", channelID)
	}

	// 单飞：同一渠道同一时刻只允许一个刷新请求，其余等待并共享结果。
	r.refreshMu.Lock()
	if call, ok := r.refreshing[channelID]; ok {
		r.refreshMu.Unlock()
		<-call.done
		if call.err != nil {
			if cached != nil {
				return cached.AccessToken, nil
			}
			return "", call.err
		}
		return call.token.AccessToken, nil
	}
	call := &refreshCall{done: make(chan struct{})}
	r.refreshing[channelID] = call
	r.refreshMu.Unlock()

	newToken, err := r.refreshToken(channelID, refreshToken, clientID, clientSecret, tokenURL)

	r.refreshMu.Lock()
	delete(r.refreshing, channelID)
	r.refreshMu.Unlock()
	call.token, call.err = newToken, err
	close(call.done)

	if err != nil {
		// Return cached token if available, even if expired
		if cached != nil {
			return cached.AccessToken, nil
		}
		return "", fmt.Errorf("token refresh failed for channel %s: %w", channelID, err)
	}
	return newToken.AccessToken, nil
}

// refreshToken performs an RFC 6749 §6 refresh_token exchange.
func (r *OAuthTokenRefresher) refreshToken(channelID, refreshToken, clientID, clientSecret, tokenURL string) (*OAuthTokenInfo, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	resp, err := r.httpClient.PostForm(tokenURL, form)
	if err != nil {
		return nil, fmt.Errorf("token endpoint request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if result.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	// Some providers issue a new refresh token; keep the old one if not.
	rt := result.RefreshToken
	if rt == "" {
		rt = refreshToken
	}

	info := &OAuthTokenInfo{
		AccessToken:  result.AccessToken,
		RefreshToken: rt,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
		TokenType:    result.TokenType,
	}
	if info.TokenType == "" {
		info.TokenType = "Bearer"
	}

	r.SetToken(channelID, info)
	// 提供方轮换了 refresh token：回写 keychain，避免重启后继续使用旧令牌（CL-11）。
	if rt != refreshToken {
		r.mu.RLock()
		fn := r.persistFn
		r.mu.RUnlock()
		if fn != nil {
			_ = fn(channelID, rt)
		}
	}
	return info, nil
}

// RemoveToken deletes the cached token for a channel (e.g. on channel deletion).
func (r *OAuthTokenRefresher) RemoveToken(channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, channelID)
}
