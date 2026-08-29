package auth

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestLoginRequest_StructFields(t *testing.T) {
	req := LoginRequest{
		Email:    "test@example.com",
		Password: "securepassword123",
	}
	if req.Email != "test@example.com" {
		t.Errorf("expected Email field to be 'test@example.com', got %q", req.Email)
	}
	if req.Password != "securepassword123" {
		t.Errorf("expected Password field to be 'securepassword123', got %q", req.Password)
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal LoginRequest: %v", err)
	}
	var unmarshalled LoginRequest
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal LoginRequest: %v", err)
	}
	if unmarshalled.Email != req.Email {
		t.Errorf("JSON round-trip failed for Email: expected %q, got %q", req.Email, unmarshalled.Email)
	}
	if unmarshalled.Password != req.Password {
		t.Errorf("JSON round-trip failed for Password: expected %q, got %q", req.Password, unmarshalled.Password)
	}
}

func TestTokenResponse_StructFields(t *testing.T) {
	resp := TokenResponse{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		ExpiresIn:    86400,
	}
	if resp.AccessToken != "access-token-value" {
		t.Errorf("expected AccessToken field to be 'access-token-value', got %q", resp.AccessToken)
	}
	if resp.RefreshToken != "refresh-token-value" {
		t.Errorf("expected RefreshToken field to be 'refresh-token-value', got %q", resp.RefreshToken)
	}
	if resp.ExpiresIn != 86400 {
		t.Errorf("expected ExpiresIn field to be 86400, got %d", resp.ExpiresIn)
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal TokenResponse: %v", err)
	}
	var unmarshalled TokenResponse
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal TokenResponse: %v", err)
	}
	if unmarshalled.AccessToken != resp.AccessToken {
		t.Errorf("JSON round-trip failed for AccessToken: expected %q, got %q", resp.AccessToken, unmarshalled.AccessToken)
	}
	if unmarshalled.RefreshToken != resp.RefreshToken {
		t.Errorf("JSON round-trip failed for RefreshToken: expected %q, got %q", resp.RefreshToken, unmarshalled.RefreshToken)
	}
	if unmarshalled.ExpiresIn != resp.ExpiresIn {
		t.Errorf("JSON round-trip failed for ExpiresIn: expected %d, got %d", resp.ExpiresIn, unmarshalled.ExpiresIn)
	}
}

func TestClaims_StructFields(t *testing.T) {
	now := time.Now()
	claims := Claims{
		UserID: "user_123",
		Email:  "test@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	if claims.UserID != "user_123" {
		t.Errorf("expected UserID field to be 'user_123', got %q", claims.UserID)
	}
	if claims.Email != "test@example.com" {
		t.Errorf("expected Email field to be 'test@example.com', got %q", claims.Email)
	}
	if claims.ExpiresAt == nil {
		t.Error("expected ExpiresAt field (from embedded RegisteredClaims) to be non-nil")
	}
	if claims.IssuedAt == nil {
		t.Error("expected IssuedAt field (from embedded RegisteredClaims) to be non-nil")
	}

	// Verify JSON tags via marshal/unmarshal
	data, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("failed to marshal Claims: %v", err)
	}
	var unmarshalled Claims
	if err := json.Unmarshal(data, &unmarshalled); err != nil {
		t.Fatalf("failed to unmarshal Claims: %v", err)
	}
	if unmarshalled.UserID != claims.UserID {
		t.Errorf("JSON round-trip failed for UserID: expected %q, got %q", claims.UserID, unmarshalled.UserID)
	}
	if unmarshalled.Email != claims.Email {
		t.Errorf("JSON round-trip failed for Email: expected %q, got %q", claims.Email, unmarshalled.Email)
	}
}

func TestGenerateAccessToken_CreatesValidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	tokenStr, err := generateAccessToken("user_456", "user@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken returned an error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected a non-empty token string")
	}

	// Parse and verify the token
	claims, err := validateToken(tokenStr)
	if err != nil {
		t.Fatalf("validateToken failed on the generated token: %v", err)
	}
	if claims.UserID != "user_456" {
		t.Errorf("expected UserID 'user_456', got %q", claims.UserID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("expected Email 'user@example.com', got %q", claims.Email)
	}
	if claims.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
	if claims.IssuedAt == nil {
		t.Error("expected IssuedAt to be set")
	}
}

func TestGenerateRefreshToken_CreatesValidToken(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	tokenStr, err := generateRefreshToken("user_789")
	if err != nil {
		t.Fatalf("generateRefreshToken returned an error: %v", err)
	}
	if tokenStr == "" {
		t.Fatal("expected a non-empty token string")
	}

	// Parse and verify the token using the refresh secret
	claims, err := validateTokenWithSecret(tokenStr, "test-refresh-secret-key")
	if err != nil {
		t.Fatalf("validateTokenWithSecret failed on the generated refresh token: %v", err)
	}
	if claims.UserID != "user_789" {
		t.Errorf("expected UserID 'user_789', got %q", claims.UserID)
	}
	if claims.ExpiresAt == nil {
		t.Error("expected ExpiresAt to be set")
	}
	if claims.IssuedAt == nil {
		t.Error("expected IssuedAt to be set")
	}
}

func TestValidateToken_Valid(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	tokenStr, err := generateAccessToken("user_111", "valid@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	claims, err := validateToken(tokenStr)
	if err != nil {
		t.Fatalf("validateToken failed on a valid token: %v", err)
	}
	if claims.UserID != "user_111" {
		t.Errorf("expected UserID 'user_111', got %q", claims.UserID)
	}
	if claims.Email != "valid@example.com" {
		t.Errorf("expected Email 'valid@example.com', got %q", claims.Email)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	_, err := validateToken("invalid-token-string")
	if err == nil {
		t.Error("expected validateToken to return an error for an invalid token")
	}
}

func TestValidateTokenWithSecret_CorrectSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	tokenStr, err := generateAccessToken("user_222", "correct@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	claims, err := validateTokenWithSecret(tokenStr, "test-access-secret-key")
	if err != nil {
		t.Fatalf("validateTokenWithSecret with correct secret failed: %v", err)
	}
	if claims.UserID != "user_222" {
		t.Errorf("expected UserID 'user_222', got %q", claims.UserID)
	}
}

func TestValidateTokenWithSecret_IncorrectSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-access-secret-key")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret-key")

	tokenStr, err := generateAccessToken("user_333", "wrong@example.com")
	if err != nil {
		t.Fatalf("generateAccessToken failed: %v", err)
	}

	_, err = validateTokenWithSecret(tokenStr, "wrong-secret-key")
	if err == nil {
		t.Error("expected validateTokenWithSecret to return an error when using an incorrect secret")
	}
}

func TestGetJWTSecret_EnvSet(t *testing.T) {
	t.Setenv("JWT_SECRET", "my-custom-secret")
	t.Setenv("JWT_REFRESH_SECRET", "some-refresh-secret")

	secret := getJWTSecret()
	if secret != "my-custom-secret" {
		t.Errorf("expected getJWTSecret to return 'my-custom-secret', got %q", secret)
	}
}

func TestGetJWTRefreshSecret_EnvSet(t *testing.T) {
	t.Setenv("JWT_SECRET", "some-access-secret")
	t.Setenv("JWT_REFRESH_SECRET", "my-custom-refresh-secret")

	secret := getJWTRefreshSecret()
	if secret != "my-custom-refresh-secret" {
		t.Errorf("expected getJWTRefreshSecret to return 'my-custom-refresh-secret', got %q", secret)
	}
}

func TestClaims_Expired(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-expired-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")

	// Create claims with an expiration time in the past
	claims := Claims{
		UserID: "user_expired",
		Email:  "expired@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-expired-secret"))
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	_, err = validateTokenWithSecret(tokenStr, "test-expired-secret")
	if err == nil {
		t.Error("expected validateTokenWithSecret to return an error for an expired token")
	}
}

func TestClaims_NotExpired(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-valid-secret")
	t.Setenv("JWT_REFRESH_SECRET", "test-refresh-secret")

	// Create claims with an expiration time in the future
	claims := Claims{
		UserID: "user_valid",
		Email:  "valid@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte("test-valid-secret"))
	if err != nil {
		t.Fatalf("failed to sign valid token: %v", err)
	}

	parsedClaims, err := validateTokenWithSecret(tokenStr, "test-valid-secret")
	if err != nil {
		t.Fatalf("validateTokenWithSecret failed for a non-expired token: %v", err)
	}
	if parsedClaims.UserID != "user_valid" {
		t.Errorf("expected UserID 'user_valid', got %q", parsedClaims.UserID)
	}
	if parsedClaims.Email != "valid@example.com" {
		t.Errorf("expected Email 'valid@example.com', got %q", parsedClaims.Email)
	}
}