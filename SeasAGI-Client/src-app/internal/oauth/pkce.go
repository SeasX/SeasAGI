package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PKCEFlow struct {
	Verifier         string
	Challenge        string
	ChallengeMethod string
}

func GeneratePKCE() (*PKCEFlow, error) {
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	h := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(h[:])

	return &PKCEFlow{
		Verifier:         verifier,
		Challenge:        challenge,
		ChallengeMethod:  "S256",
	}, nil
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	RedirectURI  string
	Scopes       []string
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
}

type TokenInfo struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope"`
}

func (t *TokenInfo) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-30 * time.Second))
}

func BuildAuthURL(cfg *OAuthConfig, state string, pkce *PKCEFlow) string {
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.ClientID)
	params.Set("redirect_uri", cfg.RedirectURI)
	params.Set("state", state)
	params.Set("scope", strings.Join(cfg.Scopes, " "))

	if pkce != nil {
		params.Set("code_challenge", pkce.Challenge)
		params.Set("code_challenge_method", pkce.ChallengeMethod)
	}

	return cfg.AuthURL + "?" + params.Encode()
}

func ExchangeCode(cfg *OAuthConfig, code string, pkce *PKCEFlow) (*TokenResponse, error) {
	params := url.Values{}
	params.Set("grant_type", "authorization_code")
	params.Set("code", code)
	params.Set("redirect_uri", cfg.RedirectURI)
	params.Set("client_id", cfg.ClientID)
	if cfg.ClientSecret != "" {
		params.Set("client_secret", cfg.ClientSecret)
	}
	if pkce != nil {
		params.Set("code_verifier", pkce.Verifier)
	}

	resp, err := http.PostForm(cfg.TokenURL, params)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

func RefreshToken(cfg *OAuthConfig, refreshToken string) (*TokenResponse, error) {
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", refreshToken)
	params.Set("client_id", cfg.ClientID)
	if cfg.ClientSecret != "" {
		params.Set("client_secret", cfg.ClientSecret)
	}

	resp, err := http.PostForm(cfg.TokenURL, params)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	return &tokenResp, nil
}

func TokenResponseToInfo(resp *TokenResponse) *TokenInfo {
	expiresAt := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	if resp.ExpiresIn == 0 {
		expiresAt = time.Now().Add(3600 * time.Second)
	}
	return &TokenInfo{
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		TokenType:    resp.TokenType,
		ExpiresAt:    expiresAt,
		Scope:        resp.Scope,
	}
}
