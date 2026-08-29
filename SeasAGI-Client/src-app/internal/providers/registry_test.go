package providers

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
)

// mockProvider implements ProviderAdapter for testing.
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) ChatCompletions(_ context.Context, _ *ProviderConfig, _ *UpstreamRequest) (*UpstreamResponse, error) {
	return &UpstreamResponse{Status: http.StatusOK, Body: io.NopCloser(nil)}, nil
}

func (m *mockProvider) ListModels(_ context.Context, _ *ProviderConfig) ([]ModelInfo, error) {
	return nil, nil
}

func (m *mockProvider) GenericPost(_ context.Context, _ *ProviderConfig, _ string, _ []byte) (*UpstreamResponse, error) {
	return &UpstreamResponse{Status: http.StatusOK, Body: io.NopCloser(nil)}, nil
}

// expectedProviders is the list of providers registered by NewExecutorRegistry.
var expectedProviders = []string{
	"platform",
	"openai",
	"azure-openai",
	"anthropic",
	"gemini",
	"ollama",
	"deepseek",
	"grok",
	"vertex",
	"openrouter",
	"bailian",
	"minimax",
	"moonshot",
	"zhipu",
	"xiaomi",
	"mistral",
	"qwen",
}

func TestNewExecutorRegistry(t *testing.T) {
	r := NewExecutorRegistry()
	providers := r.ListProviders()

	if len(providers) != len(expectedProviders) {
		t.Fatalf("expected %d providers, got %d", len(expectedProviders), len(providers))
	}

	got := make(map[string]bool)
	for _, p := range providers {
		got[p] = true
	}

	for _, exp := range expectedProviders {
		if !got[exp] {
			t.Errorf("expected provider %q not found in registry", exp)
		}
	}
}

func TestRegister(t *testing.T) {
	r := NewExecutorRegistry()

	r.Register("test-provider", func() ProviderAdapter {
		return &mockProvider{name: "test-provider"}
	})

	adapter := r.Resolve("test-provider")
	if adapter == nil {
		t.Fatal("Resolve returned nil after Register")
	}
	if adapter.Name() != "test-provider" {
		t.Errorf("expected name 'test-provider', got %q", adapter.Name())
	}
}

func TestResolve(t *testing.T) {
	r := NewExecutorRegistry()

	for _, name := range expectedProviders {
		adapter := r.Resolve(name)
		if adapter == nil {
			t.Errorf("Resolve(%q) returned nil, expected non-nil", name)
		}
	}
}

func TestResolveUnknown(t *testing.T) {
	r := NewExecutorRegistry()

	adapter := r.Resolve("non-existent-provider")
	if adapter != nil {
		t.Errorf("Resolve for unknown provider returned non-nil: %v", adapter)
	}
}

func TestResolveCaseInsensitive(t *testing.T) {
	r := NewExecutorRegistry()

	variants := []string{"OpenAI", "OPENAI", "openai"}
	for _, v := range variants {
		adapter := r.Resolve(v)
		if adapter == nil {
			t.Errorf("Resolve(%q) returned nil, expected case-insensitive match", v)
		}
	}
}

func TestListProviders(t *testing.T) {
	r := NewExecutorRegistry()

	providers := r.ListProviders()

	if len(providers) != len(expectedProviders) {
		t.Errorf("expected %d providers, got %d", len(expectedProviders), len(providers))
	}

	// Verify all expected providers are present
	got := make(map[string]bool)
	for _, p := range providers {
		got[p] = true
	}
	for _, exp := range expectedProviders {
		if !got[exp] {
			t.Errorf("expected provider %q not found in ListProviders result", exp)
		}
	}

	// Verify no duplicate names
	seen := make(map[string]bool)
	for _, p := range providers {
		if seen[p] {
			t.Errorf("duplicate provider name %q in ListProviders", p)
		}
		seen[p] = true
	}
}

func TestRegisterExecutor(t *testing.T) {
	// Reset the global registry for this test
	original := globalRegistry
	globalRegistry = NewExecutorRegistry()
	defer func() { globalRegistry = original }()

	RegisterExecutor("custom-provider", func() ProviderAdapter {
		return &mockProvider{name: "custom-provider"}
	})

	adapter := ResolveFromRegistry("custom-provider")
	if adapter == nil {
		t.Fatal("ResolveFromRegistry returned nil after RegisterExecutor")
	}
	if adapter.Name() != "custom-provider" {
		t.Errorf("expected name 'custom-provider', got %q", adapter.Name())
	}
}

func TestResolveFromRegistry(t *testing.T) {
	// Reset the global registry for this test
	original := globalRegistry
	globalRegistry = NewExecutorRegistry()
	defer func() { globalRegistry = original }()

	for _, name := range expectedProviders {
		adapter := ResolveFromRegistry(name)
		if adapter == nil {
			t.Errorf("ResolveFromRegistry(%q) returned nil, expected non-nil", name)
		}
	}

	unknown := ResolveFromRegistry("non-existent")
	if unknown != nil {
		t.Errorf("ResolveFromRegistry for unknown provider returned non-nil: %v", unknown)
	}
}

func TestListRegisteredProviders(t *testing.T) {
	// Reset the global registry for this test
	original := globalRegistry
	globalRegistry = NewExecutorRegistry()
	defer func() { globalRegistry = original }()

	providers := ListRegisteredProviders()

	if len(providers) < len(expectedProviders) {
		t.Errorf("expected at least %d providers, got %d", len(expectedProviders), len(providers))
	}
}

func TestDuplicateRegistration(t *testing.T) {
	r := NewExecutorRegistry()

	// Register a custom adapter for "openai"
	r.Register("openai", func() ProviderAdapter {
		return &mockProvider{name: "custom-openai"}
	})

	adapter := r.Resolve("openai")
	if adapter == nil {
		t.Fatal("Resolve returned nil after duplicate registration")
	}
	if adapter.Name() != "custom-openai" {
		t.Errorf("expected duplicate registration to overwrite, got name %q", adapter.Name())
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewExecutorRegistry()

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, name := range expectedProviders {
				adapter := r.Resolve(name)
				if adapter == nil {
					t.Errorf("concurrent Resolve(%q) returned nil", name)
				}
			}
		}()
	}

	// Concurrent ListProviders
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			providers := r.ListProviders()
			if len(providers) < len(expectedProviders) {
				t.Errorf("concurrent ListProviders returned %d providers, expected at least %d", len(providers), len(expectedProviders))
			}
		}()
	}

	// Concurrent writes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "concurrent-provider-" + string(rune('A'+i))
			r.Register(name, func() ProviderAdapter {
				return &mockProvider{name: name}
			})
		}(i)
	}

	wg.Wait()

	// Verify all concurrently registered providers are accessible
	for i := 0; i < 5; i++ {
		name := "concurrent-provider-" + string(rune('A'+i))
		adapter := r.Resolve(name)
		if adapter == nil {
			t.Errorf("concurrently registered provider %q not found", name)
		}
	}
}

func TestResolveAfterRegisterPreservesExisting(t *testing.T) {
	r := NewExecutorRegistry()

	// Verify that after registration, existing providers still work
	r.Register("test-provider", func() ProviderAdapter {
		return &mockProvider{name: "test-provider"}
	})

	for _, name := range expectedProviders {
		adapter := r.Resolve(name)
		if adapter == nil {
			t.Errorf("existing provider %q not resolvable after new registration", name)
		}
	}
}

func TestNewExecutorRegistryReturnsNonNil(t *testing.T) {
	r := NewExecutorRegistry()
	if r == nil {
		t.Fatal("NewExecutorRegistry returned nil")
	}
}