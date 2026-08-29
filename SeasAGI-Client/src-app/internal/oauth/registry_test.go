package oauth

import (
	"sync"
	"testing"
)

func TestNewProviderRegistry(t *testing.T) {
	r := NewProviderRegistry()

	expectedProviders := []string{"google", "github", "microsoft", "anthropic"}
	for _, name := range expectedProviders {
		info, ok := r.Get(name)
		if !ok {
			t.Errorf("expected provider %s to be registered, but it was not found", name)
		}
		if info.Name != name {
			t.Errorf("expected provider name %q, got %q", name, info.Name)
		}
	}
}

func TestRegister(t *testing.T) {
	r := NewProviderRegistry()

	customProvider := ProviderInfo{
		Name:        "custom",
		DisplayName: "Custom Provider",
		AuthURL:     "https://custom.example.com/auth",
		TokenURL:    "https://custom.example.com/token",
		Scopes:      []string{"scope1", "scope2"},
		IconURL:     "https://custom.example.com/icon.png",
	}
	r.Register(customProvider)

	got, ok := r.Get("custom")
	if !ok {
		t.Fatal("expected provider 'custom' to be registered after Register call")
	}
	if got.DisplayName != "Custom Provider" {
		t.Errorf("expected DisplayName %q, got %q", "Custom Provider", got.DisplayName)
	}
	if got.AuthURL != "https://custom.example.com/auth" {
		t.Errorf("expected AuthURL %q, got %q", "https://custom.example.com/auth", got.AuthURL)
	}
	if got.TokenURL != "https://custom.example.com/token" {
		t.Errorf("expected TokenURL %q, got %q", "https://custom.example.com/token", got.TokenURL)
	}
	if len(got.Scopes) != 2 || got.Scopes[0] != "scope1" || got.Scopes[1] != "scope2" {
		t.Errorf("expected Scopes %v, got %v", []string{"scope1", "scope2"}, got.Scopes)
	}
	if got.IconURL != "https://custom.example.com/icon.png" {
		t.Errorf("expected IconURL %q, got %q", "https://custom.example.com/icon.png", got.IconURL)
	}
}

func TestRegister_Overwrite(t *testing.T) {
	r := NewProviderRegistry()

	original, ok := r.Get("google")
	if !ok {
		t.Fatal("expected provider 'google' to be registered")
	}
	originalScopes := make([]string, len(original.Scopes))
	copy(originalScopes, original.Scopes)

	modified := original
	modified.Scopes = []string{"overwritten"}
	r.Register(modified)

	got, ok := r.Get("google")
	if !ok {
		t.Fatal("expected provider 'google' to still exist after overwrite")
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "overwritten" {
		t.Errorf("expected Scopes to be overwritten to %v, got %v", []string{"overwritten"}, got.Scopes)
	}
}

func TestGet_ExistingProvider(t *testing.T) {
	r := NewProviderRegistry()

	info, ok := r.Get("google")
	if !ok {
		t.Fatal("expected provider 'google' to be found")
	}
	if info.Name != "google" {
		t.Errorf("expected Name %q, got %q", "google", info.Name)
	}
	if info.DisplayName != "Google" {
		t.Errorf("expected DisplayName %q, got %q", "Google", info.DisplayName)
	}
	if info.AuthURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		t.Errorf("expected AuthURL %q, got %q", "https://accounts.google.com/o/oauth2/v2/auth", info.AuthURL)
	}
	if info.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("expected TokenURL %q, got %q", "https://oauth2.googleapis.com/token", info.TokenURL)
	}
	if info.IconURL != "https://www.google.com/favicon.ico" {
		t.Errorf("expected IconURL %q, got %q", "https://www.google.com/favicon.ico", info.IconURL)
	}
	if len(info.Scopes) != 3 {
		t.Errorf("expected 3 scopes, got %d", len(info.Scopes))
	}
}

func TestGet_UnknownProvider(t *testing.T) {
	r := NewProviderRegistry()

	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected Get for unknown provider to return false, got true")
	}
}

func TestGet_EmptyName(t *testing.T) {
	r := NewProviderRegistry()

	_, ok := r.Get("")
	if ok {
		t.Error("expected Get for empty name to return false, got true")
	}
}

func TestList(t *testing.T) {
	r := NewProviderRegistry()

	providers := r.List()
	if len(providers) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(providers))
	}

	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name] = true
	}

	expectedNames := []string{"google", "github", "microsoft", "anthropic"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected provider %q to be in the list, but it was not found", name)
		}
	}
}

func TestList_EmptyRegistry(t *testing.T) {
	r := &ProviderRegistry{
		providers: make(map[string]ProviderInfo),
	}

	providers := r.List()
	if len(providers) != 0 {
		t.Errorf("expected empty list from empty registry, got %d providers", len(providers))
	}
}

func TestList_ReturnsCopy(t *testing.T) {
	r := NewProviderRegistry()

	originalList := r.List()
	originalLen := len(originalList)

	// Register a new provider after getting the list
	r.Register(ProviderInfo{
		Name: "test-provider",
	})

	// The original list should not be affected
	if len(originalList) != originalLen {
		t.Errorf("List should return a copy; original list length changed from %d to %d", originalLen, len(originalList))
	}
}

func TestRegisterOAuthProvider(t *testing.T) {
	saved := globalProviderRegistry
	globalProviderRegistry = NewProviderRegistry()
	defer func() {
		globalProviderRegistry = saved
	}()

	customProvider := ProviderInfo{
		Name:        "test-global",
		DisplayName: "Test Global",
		AuthURL:     "https://test.example.com/auth",
		TokenURL:    "https://test.example.com/token",
		Scopes:      []string{"test"},
		IconURL:     "https://test.example.com/icon.png",
	}
	RegisterOAuthProvider(customProvider)

	info, ok := globalProviderRegistry.Get("test-global")
	if !ok {
		t.Fatal("expected provider 'test-global' to be registered via RegisterOAuthProvider")
	}
	if info.DisplayName != "Test Global" {
		t.Errorf("expected DisplayName %q, got %q", "Test Global", info.DisplayName)
	}
}

func TestGetOAuthProvider(t *testing.T) {
	saved := globalProviderRegistry
	globalProviderRegistry = NewProviderRegistry()
	defer func() {
		globalProviderRegistry = saved
	}()

	// Test getting an existing provider
	info, ok := GetOAuthProvider("google")
	if !ok {
		t.Fatal("expected provider 'google' to be found via GetOAuthProvider")
	}
	if info.Name != "google" {
		t.Errorf("expected Name %q, got %q", "google", info.Name)
	}

	// Test getting a non-existent provider
	_, ok = GetOAuthProvider("nonexistent")
	if ok {
		t.Error("expected GetOAuthProvider for unknown provider to return false, got true")
	}
}

func TestListOAuthProviders(t *testing.T) {
	saved := globalProviderRegistry
	globalProviderRegistry = NewProviderRegistry()
	defer func() {
		globalProviderRegistry = saved
	}()

	providers := ListOAuthProviders()
	if len(providers) != 4 {
		t.Fatalf("expected 4 providers from ListOAuthProviders, got %d", len(providers))
	}

	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name] = true
	}

	for _, name := range []string{"google", "github", "microsoft", "anthropic"} {
		if !names[name] {
			t.Errorf("expected provider %q to be in the global list, but it was not found", name)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewProviderRegistry()

	var wg sync.WaitGroup
	const goroutines = 20

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				r.Get("google")
				r.List()
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "provider-" + string(rune('a'+idx))
			for j := 0; j < 20; j++ {
				r.Register(ProviderInfo{
					Name: name,
				})
			}
		}(i)
	}

	wg.Wait()

	// Verify the registry is still in a consistent state
	info, ok := r.Get("google")
	if !ok {
		t.Error("expected 'google' provider to still exist after concurrent access")
	}
	if info.DisplayName != "Google" {
		t.Errorf("expected DisplayName %q, got %q", "Google", info.DisplayName)
	}
}

func TestConcurrentReadOnly(t *testing.T) {
	r := NewProviderRegistry()

	var wg sync.WaitGroup
	const goroutines = 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.Get("google")
				r.Get("github")
				r.Get("microsoft")
				r.Get("anthropic")
				r.List()
			}
		}()
	}

	wg.Wait()
}