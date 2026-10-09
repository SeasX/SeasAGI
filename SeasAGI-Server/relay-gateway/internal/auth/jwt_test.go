package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-key-for-unit-testing"

func TestMain(m *testing.M) {
	os.Setenv("JWT_SECRET", testSecret)
	code := m.Run()
	os.Unsetenv("JWT_SECRET")
	os.Exit(code)
}

// TestGenerateToken_CreatesValidToken verifies GenerateToken produces a non-empty
// token string with exactly three dot-separated parts (header.payload.signature).
func TestGenerateToken_CreatesValidToken(t *testing.T) {
	token, err := GenerateToken("user-1", "channel-1")
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken returned empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT, got %d", len(parts))
	}
}

// TestValidateToken_AcceptsGeneratedToken verifies that a token produced by
// GenerateToken is accepted by ValidateToken without error.
func TestValidateToken_AcceptsGeneratedToken(t *testing.T) {
	token, err := GenerateToken("user-1", "channel-1")
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken rejected a valid token: %v", err)
	}
	if claims == nil {
		t.Fatal("ValidateToken returned nil claims on valid token")
	}
}

// TestValidateToken_ReturnsCorrectClaims verifies that the claims returned by
// ValidateToken contain the exact UserID and ChannelID used during generation.
func TestValidateToken_ReturnsCorrectClaims(t *testing.T) {
	wantUserID := "user-42"
	wantChannelID := "channel-99"

	token, err := GenerateToken(wantUserID, wantChannelID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.UserID != wantUserID {
		t.Errorf("expected UserID %q, got %q", wantUserID, claims.UserID)
	}
	if claims.ChannelID != wantChannelID {
		t.Errorf("expected ChannelID %q, got %q", wantChannelID, claims.ChannelID)
	}
}

// TestValidateToken_RejectsExpiredToken manually crafts a token whose expiry is
// in the past and verifies that ValidateToken returns an error.
func TestValidateToken_RejectsExpiredToken(t *testing.T) {
	claims := Claims{
		UserID:    "user-expired",
		ChannelID: "channel-expired",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	_, err = ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

// TestValidateToken_RejectsMalformedToken passes a clearly invalid string
// and expects ValidateToken to return an error.
func TestValidateToken_RejectsMalformedToken(t *testing.T) {
	_, err := ValidateToken("this.is.not.a.valid.jwt")
	if err == nil {
		t.Fatal("expected error for malformed token, got nil")
	}
}

// TestValidateToken_RejectsWrongSigningMethod creates a token signed with the
// RSA-based RS256 algorithm (not HS256) and verifies that ValidateToken
// rejects it because the signing method is not HMAC.
func TestValidateToken_RejectsWrongSigningMethod(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	claims := Claims{
		UserID:    "user-wrong-alg",
		ChannelID: "channel-wrong-alg",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenStr, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("failed to sign token with RS256: %v", err)
	}

	_, err = ValidateToken(tokenStr)
	if err == nil {
		t.Fatal("expected error for token with wrong signing method, got nil")
	}
}

// TestValidateToken_ChecksTokenExpiry verifies that a token with a future
// expiry passes validation, confirming that the expiry check is working
// (not rejecting all tokens or accepting all tokens).
func TestValidateToken_ChecksTokenExpiry(t *testing.T) {
	// Valid token with future expiry should pass.
	claims := Claims{
		UserID:    "user-future",
		ChannelID: "channel-future",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign valid token: %v", err)
	}

	_, err = ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("expected valid token to pass, got error: %v", err)
	}
}

// TestClaims_HasAllRequiredFields verifies that the Claims struct can hold
// all defined fields and that they round-trip correctly through the JWT.
func TestClaims_HasAllRequiredFields(t *testing.T) {
	now := time.Now()
	claims := Claims{
		UserID:    "user-field-test",
		ChannelID: "channel-field-test",
		DeviceID:  "device-001",
		TenantID:  "tenant-xyz",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   "test-subject",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	parsed, err := ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if parsed.UserID != claims.UserID {
		t.Errorf("UserID: expected %q, got %q", claims.UserID, parsed.UserID)
	}
	if parsed.ChannelID != claims.ChannelID {
		t.Errorf("ChannelID: expected %q, got %q", claims.ChannelID, parsed.ChannelID)
	}
	if parsed.DeviceID != claims.DeviceID {
		t.Errorf("DeviceID: expected %q, got %q", claims.DeviceID, parsed.DeviceID)
	}
	if parsed.TenantID != claims.TenantID {
		t.Errorf("TenantID: expected %q, got %q", claims.TenantID, parsed.TenantID)
	}
}

// TestGenerateToken_WithEmptyUserID verifies that GenerateToken succeeds
// when the userID is empty (it should still produce a valid token).
func TestGenerateToken_WithEmptyUserID(t *testing.T) {
	token, err := GenerateToken("", "channel-1")
	if err != nil {
		t.Fatalf("GenerateToken with empty userID returned error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken with empty userID returned empty token")
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken rejected token with empty userID: %v", err)
	}
	if claims.UserID != "" {
		t.Errorf("expected empty UserID, got %q", claims.UserID)
	}
	if claims.ChannelID != "channel-1" {
		t.Errorf("expected ChannelID %q, got %q", "channel-1", claims.ChannelID)
	}
}

// TestGenerateToken_WithEmptyChannelID verifies that GenerateToken succeeds
// when the channelID is empty (it should still produce a valid token).
func TestGenerateToken_WithEmptyChannelID(t *testing.T) {
	token, err := GenerateToken("user-1", "")
	if err != nil {
		t.Fatalf("GenerateToken with empty channelID returned error: %v", err)
	}
	if token == "" {
		t.Fatal("GenerateToken with empty channelID returned empty token")
	}

	claims, err := ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken rejected token with empty channelID: %v", err)
	}
	if claims.ChannelID != "" {
		t.Errorf("expected empty ChannelID, got %q", claims.ChannelID)
	}
	if claims.UserID != "user-1" {
		t.Errorf("expected UserID %q, got %q", "user-1", claims.UserID)
	}
}
