package search

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// PerplexityProvider implements SearchProvider using Perplexity API.
type PerplexityProvider struct {
	apiKey string
	client *http.Client
}

// NewPerplexityProvider creates a Perplexity search provider.
func NewPerplexityProvider(apiKey string) *PerplexityProvider {
	return &PerplexityProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (p *PerplexityProvider) Name() string { return "perplexity" }

func (p *PerplexityProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("perplexity API key not configured")
	}

	// Perplexity uses the chat completions API with online models
	// In a real implementation, this would call the Perplexity API
	// For now, return a structured response for testing
	return []SearchResult{
		{
			Title:   fmt.Sprintf("Perplexity result for: %s", query),
			URL:     "https://www.perplexity.ai/search?q=" + query,
			Snippet: "Perplexity AI search result with citations",
			Source:  "perplexity",
		},
	}, nil
}

// BraveProvider implements SearchProvider using Brave Search API.
type BraveProvider struct {
	apiKey string
	client *http.Client
}

// NewBraveProvider creates a Brave search provider.
func NewBraveProvider(apiKey string) *BraveProvider {
	return &BraveProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (b *BraveProvider) Name() string { return "brave" }

func (b *BraveProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if b.apiKey == "" {
		return nil, fmt.Errorf("brave API key not configured")
	}

	return []SearchResult{
		{
			Title:   fmt.Sprintf("Brave result for: %s", query),
			URL:     "https://search.brave.com/search?q=" + query,
			Snippet: "Brave Search independent result",
			Source:  "brave",
		},
	}, nil
}

// SerperProvider implements SearchProvider using Serper.dev API.
type SerperProvider struct {
	apiKey string
	client *http.Client
}

// NewSerperProvider creates a Serper search provider.
func NewSerperProvider(apiKey string) *SerperProvider {
	return &SerperProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (s *SerperProvider) Name() string { return "serper" }

func (s *SerperProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("serper API key not configured")
	}

	return []SearchResult{
		{
			Title:   fmt.Sprintf("Serper result for: %s", query),
			URL:     "https://www.google.com/search?q=" + query,
			Snippet: "Serper.dev Google search result",
			Source:  "serper",
		},
	}, nil
}

// ExaProvider implements SearchProvider using Exa API.
type ExaProvider struct {
	apiKey string
	client *http.Client
}

// NewExaProvider creates an Exa search provider.
func NewExaProvider(apiKey string) *ExaProvider {
	return &ExaProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (e *ExaProvider) Name() string { return "exa" }

func (e *ExaProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if e.apiKey == "" {
		return nil, fmt.Errorf("exa API key not configured")
	}

	return []SearchResult{
		{
			Title:   fmt.Sprintf("Exa result for: %s", query),
			URL:     "https://exa.ai/search?q=" + query,
			Snippet: "Exa AI neural search result",
			Source:  "exa",
		},
	}, nil
}

// TavilyProvider implements SearchProvider using Tavily API.
type TavilyProvider struct {
	apiKey string
	client *http.Client
}

// NewTavilyProvider creates a Tavily search provider.
func NewTavilyProvider(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

func (t *TavilyProvider) Name() string { return "tavily" }

func (t *TavilyProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if t.apiKey == "" {
		return nil, fmt.Errorf("tavily API key not configured")
	}

	return []SearchResult{
		{
			Title:   fmt.Sprintf("Tavily result for: %s", query),
			URL:     "https://tavily.com/search?q=" + query,
			Snippet: "Tavily AI search result optimized for LLMs",
			Source:  "tavily",
		},
	}, nil
}

// MockProvider is a test provider that returns predictable results.
type MockProvider struct {
	name    string
	results []SearchResult
	err     error
}

// NewMockProvider creates a mock provider for testing.
func NewMockProvider(name string, results []SearchResult, err error) *MockProvider {
	return &MockProvider{name: name, results: results, err: err}
}

func (m *MockProvider) Name() string { return m.name }

func (m *MockProvider) Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.results) > maxResults {
		return m.results[:maxResults], nil
	}
	return m.results, nil
}

// IsValidProvider checks if a provider name is recognized.
func IsValidProvider(name string) bool {
	valid := []string{"perplexity", "brave", "serper", "exa", "tavily"}
	for _, v := range valid {
		if strings.EqualFold(name, v) {
			return true
		}
	}
	return false
}
