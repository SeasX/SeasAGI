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
	mu       sync.RWMutex
	tokens   map[string]*OAuthTokenInfo // channelID → token info
	margin   time.Duration              // refresh safety margin
	httpClient *http.Client
}

// NewOAuthTokenRefresher creates a new refresher with the given safety margin.
func NewOAuthTokenRefresher(margin time.Duration) *OAuthTokenRefresher {
	if margin <= 0 {
		margin = 60 * time.Second
	}
	return &OAuthTokenRefresher{
		tokens:   make(map[string]*OAuthTokenInfo),
		margin:   margin,
		httpClient: &http.Client{Timeout: 15 * time.Second},
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

	// Refresh
	newToken, err := r.refreshToken(channelID, refreshToken, clientID, clientSecret, tokenURL)
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
	return info, nil
}

// RemoveToken deletes the cached token for a channel (e.g. on channel deletion).
func (r *OAuthTokenRefresher) RemoveToken(channelID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tokens, channelID)
}
