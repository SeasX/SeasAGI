package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"
	"time"
)

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() returned error: %v", err)
	}

	if pkce == nil {
		t.Fatal("GeneratePKCE() returned nil")
	}

	// Verifier should be non-empty
	if pkce.Verifier == "" {
		t.Error("Verifier should not be empty")
	}

	// Decode verifier - should be 32 bytes before encoding
	verifierBytes, err := base64.RawURLEncoding.DecodeString(pkce.Verifier)
	if err != nil {
		t.Fatalf("Verifier is not valid base64url: %v", err)
	}
	if len(verifierBytes) != 32 {
		t.Errorf("Verifier decoded length = %d, want 32", len(verifierBytes))
	}

	// Challenge should be valid base64url
	challengeBytes, err := base64.RawURLEncoding.DecodeString(pkce.Challenge)
	if err != nil {
		t.Fatalf("Challenge is not valid base64url: %v", err)
	}
	if len(challengeBytes) != 32 {
		t.Errorf("Challenge decoded length = %d, want 32", len(challengeBytes))
	}

	// Challenge method should be S256
	if pkce.ChallengeMethod != "S256" {
		t.Errorf("ChallengeMethod = %q, want %q", pkce.ChallengeMethod, "S256")
	}
}

func TestGeneratePKCE_Determinism(t *testing.T) {
	pkce1, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() returned error: %v", err)
	}
	pkce2, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() returned error: %v", err)
	}

	if pkce1.Verifier == pkce2.Verifier {
		t.Error("Two calls should produce different verifiers")
	}
	if pkce1.Challenge == pkce2.Challenge {
		t.Error("Two calls should produce different challenges")
	}
}

func TestGeneratePKCE_ChallengeVerification(t *testing.T) {
	pkce, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() returned error: %v", err)
	}

	// Compute SHA-256 of verifier and base64url encode it
	h := sha256.Sum256([]byte(pkce.Verifier))
	expectedChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	if pkce.Challenge != expectedChallenge {
		t.Errorf("Challenge = %q, want %q (SHA-256 of verifier base64url)", pkce.Challenge, expectedChallenge)
	}
}

func TestTokenInfo_IsExpired_NotExpired(t *testing.T) {
	info := &TokenInfo{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	if info.IsExpired() {
		t.Error("IsExpired() should be false when expiry is 1 hour in the future")
	}
}

func TestTokenInfo_IsExpired_Expired(t *testing.T) {
	info := &TokenInfo{
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if !info.IsExpired() {
		t.Error("IsExpired() should be true when expiry is 1 hour in the past")
	}
}

func TestTokenInfo_IsExpired_WithinBuffer(t *testing.T) {
	// 20 seconds before expiry - within the 30s buffer, so should be expired
	info := &TokenInfo{
		ExpiresAt: time.Now().Add(20 * time.Second),
	}
	if !info.IsExpired() {
		t.Error("IsExpired() should be true when within 30s buffer (20s before expiry)")
	}
}

func TestTokenInfo_IsExpired_JustBeforeBuffer(t *testing.T) {
	// 40 seconds before expiry - outside the 30s buffer, so should NOT be expired
	info := &TokenInfo{
		ExpiresAt: time.Now().Add(40 * time.Second),
	}
	if info.IsExpired() {
		t.Error("IsExpired() should be false when 40s before expiry (outside 30s buffer)")
	}
}

func TestBuildAuthURL(t *testing.T) {
	cfg := &OAuthConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.example.com/authorize",
		RedirectURI: "https://myapp.example.com/callback",
		Scopes:      []string{"openid", "profile", "email"},
	}
	state := "test-state-123"
	pkce := &PKCEFlow{
		Verifier:         "test-verifier",
		Challenge:        "test-challenge",
		ChallengeMethod:  "S256",
	}

	authURL := BuildAuthURL(cfg, state, pkce)

	// Parse the URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("BuildAuthURL returned invalid URL: %v", err)
	}

	// Check base URL
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host + parsedURL.Path
	if baseURL != "https://auth.example.com/authorize" {
		t.Errorf("Base URL = %q, want %q", baseURL, "https://auth.example.com/authorize")
	}

	params := parsedURL.Query()

	tests := []struct {
		key      string
		expected string
	}{
		{"response_type", "code"},
		{"client_id", "test-client-id"},
		{"redirect_uri", "https://myapp.example.com/callback"},
		{"state", "test-state-123"},
		{"scope", "openid profile email"},
		{"code_challenge", "test-challenge"},
		{"code_challenge_method", "S256"},
	}

	for _, tt := range tests {
		if got := params.Get(tt.key); got != tt.expected {
			t.Errorf("Query param %q = %q, want %q", tt.key, got, tt.expected)
		}
	}
}

func TestBuildAuthURL_NoPKCE(t *testing.T) {
	cfg := &OAuthConfig{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.example.com/authorize",
		RedirectURI: "https://myapp.example.com/callback",
		Scopes:      []string{"openid"},
	}
	state := "test-state"

	authURL := BuildAuthURL(cfg, state, nil)

	parsedURL, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("BuildAuthURL returned invalid URL: %v", err)
	}

	params := parsedURL.Query()

	// PKCE params should NOT be present when pkce is nil
	if params.Get("code_challenge") != "" {
		t.Error("code_challenge should not be present when pkce is nil")
	}
	if params.Get("code_challenge_method") != "" {
		t.Error("code_challenge_method should not be present when pkce is nil")
	}

	// Non-PKCE params should still be present
	if params.Get("client_id") != "test-client-id" {
		t.Errorf("client_id = %q, want %q", params.Get("client_id"), "test-client-id")
	}
	if params.Get("state") != "test-state" {
		t.Errorf("state = %q, want %q", params.Get("state"), "test-state")
	}
}

func TestTokenResponseToInfo(t *testing.T) {
	before := time.Now()
	resp := &TokenResponse{
		AccessToken:  "access-token-123",
		RefreshToken: "refresh-token-456",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		Scope:        "openid profile",
		IDToken:      "id-token-789",
	}

	info := TokenResponseToInfo(resp)
	after := time.Now()

	if info.AccessToken != "access-token-123" {
		t.Errorf("AccessToken = %q, want %q", info.AccessToken, "access-token-123")
	}
	if info.RefreshToken != "refresh-token-456" {
		t.Errorf("RefreshToken = %q, want %q", info.RefreshToken, "refresh-token-456")
	}
	if info.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", info.TokenType, "Bearer")
	}
	if info.Scope != "openid profile" {
		t.Errorf("Scope = %q, want %q", info.Scope, "openid profile")
	}

	// ExpiresAt should be approximately now + 3600s
	expectedMin := before.Add(3600 * time.Second)
	expectedMax := after.Add(3600 * time.Second)
	if info.ExpiresAt.Before(expectedMin) || info.ExpiresAt.After(expectedMax) {
		t.Errorf("ExpiresAt = %v, want between %v and %v", info.ExpiresAt, expectedMin, expectedMax)
	}
}

func TestTokenResponseToInfo_DefaultExpiry(t *testing.T) {
	before := time.Now()
	resp := &TokenResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    0, // Should default to 3600
		Scope:        "openid",
	}

	info := TokenResponseToInfo(resp)
	after := time.Now()

	// When ExpiresIn is 0, should default to 3600 seconds
	expectedMin := before.Add(3600 * time.Second)
	expectedMax := after.Add(3600 * time.Second)
	if info.ExpiresAt.Before(expectedMin) || info.ExpiresAt.After(expectedMax) {
		t.Errorf("With ExpiresIn=0, ExpiresAt = %v, want between %v and %v", info.ExpiresAt, expectedMin, expectedMax)
	}
}