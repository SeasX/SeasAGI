package auth

import (
	"sync"
	"testing"
)

func TestNewService_NonNil(t *testing.T) {
	svc := NewService()
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
}

func TestNewService_NotLoggedInInitially(t *testing.T) {
	// NewService tries to load a token from the keychain/environment.
	// This test documents that the service is not logged in when no token is available.
	// For isolated state tests, construct the service directly.
	svc := &Service{}
	if svc.IsLoggedIn() {
		t.Fatal("expected service to not be logged in initially")
	}
}

func TestGetAuthState_ReturnsCorrectState(t *testing.T) {
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      strPtr("test@example.com"),
		},
		token: "some-token",
	}

	state := svc.GetAuthState()
	if !state.IsLoggedIn {
		t.Error("expected IsLoggedIn to be true")
	}
	if state.UserID == nil || *state.UserID != "user123" {
		t.Errorf("expected UserID 'user123', got %v", state.UserID)
	}
	if state.Email == nil || *state.Email != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got %v", state.Email)
	}
}

func TestIsLoggedIn_ReturnsFalseWhenNotLoggedIn(t *testing.T) {
	svc := &Service{
		info:  AuthInfo{IsLoggedIn: false},
		token: "",
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to return false")
	}
}

func TestGetPlatformToken_ReturnsEmptyWhenNotLoggedIn(t *testing.T) {
	svc := &Service{}
	token := svc.GetPlatformToken()
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
}

func TestLogout_ClearsState(t *testing.T) {
	email := "test@example.com"
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      &email,
		},
		token: "some-token",
	}

	err := svc.Logout()
	if err != nil {
		t.Fatalf("Logout() returned error: %v", err)
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to be false after logout")
	}
	if svc.GetPlatformToken() != "" {
		t.Error("expected token to be empty after logout")
	}
	state := svc.GetAuthState()
	if state.IsLoggedIn {
		t.Error("expected AuthState.IsLoggedIn to be false after logout")
	}
}

func TestLogout_WhenNotLoggedIn(t *testing.T) {
	svc := &Service{
		info:  AuthInfo{IsLoggedIn: false},
		token: "",
	}

	err := svc.Logout()
	if err != nil {
		t.Fatalf("Logout() returned error: %v", err)
	}

	if svc.IsLoggedIn() {
		t.Error("expected IsLoggedIn to remain false")
	}
}

func TestExtractUserID_ReturnsFirst8Chars(t *testing.T) {
	svc := &Service{}
	token := "abcdefghijklmnop"
	result := svc.extractUserID(token)
	if result != "abcdefgh" {
		t.Errorf("expected 'abcdefgh', got %q", result)
	}
}

func TestExtractUserID_EmptyString(t *testing.T) {
	svc := &Service{}
	result := svc.extractUserID("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestExtractUserID_ShortString(t *testing.T) {
	svc := &Service{}
	token := "abc"
	result := svc.extractUserID(token)
	if result != "abc" {
		t.Errorf("expected 'abc', got %q", result)
	}
}

func TestExtractUserID_Exactly8Chars(t *testing.T) {
	svc := &Service{}
	token := "12345678"
	result := svc.extractUserID(token)
	if result != "12345678" {
		t.Errorf("expected '12345678', got %q", result)
	}
}

func TestConcurrentAccess(t *testing.T) {
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("concurrent"),
			Email:      strPtr("concurrent@test.com"),
		},
		token: "concurrent-token",
	}

	var wg sync.WaitGroup
	workers := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.GetAuthState()
			_ = svc.IsLoggedIn()
			_ = svc.GetPlatformToken()
		}()
	}

	wg.Wait()
}

func TestConcurrentAccess_WithLogout(t *testing.T) {
	email := "user@test.com"
	svc := &Service{
		info: AuthInfo{
			IsLoggedIn: true,
			UserID:     strPtr("user123"),
			Email:      &email,
		},
		token: "some-token",
	}

	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%3 == 0 {
				_ = svc.Logout()
			} else {
				_ = svc.GetAuthState()
				_ = svc.IsLoggedIn()
				_ = svc.GetPlatformToken()
			}
		}(i)
	}

	wg.Wait()
}

func strPtr(s string) *string {
	return &s
}
