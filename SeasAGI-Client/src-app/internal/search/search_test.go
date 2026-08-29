package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchPerplexity(t *testing.T) {
	provider := NewPerplexityProvider("test-key")
	results, err := provider.Search(context.Background(), "Go programming", 5)
	if err != nil {
		t.Fatalf("Perplexity search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 result")
	}
	if results[0].Source != "perplexity" {
		t.Errorf("Expected source 'perplexity', got '%s'", results[0].Source)
	}
}

func TestSearchBrave(t *testing.T) {
	provider := NewBraveProvider("test-key")
	results, err := provider.Search(context.Background(), "Go programming", 5)
	if err != nil {
		t.Fatalf("Brave search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 result")
	}
	if results[0].Source != "brave" {
		t.Errorf("Expected source 'brave', got '%s'", results[0].Source)
	}
}

func TestSearchSerper(t *testing.T) {
	provider := NewSerperProvider("test-key")
	results, err := provider.Search(context.Background(), "Go programming", 5)
	if err != nil {
		t.Fatalf("Serper search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 result")
	}
}

func TestSearchExa(t *testing.T) {
	provider := NewExaProvider("test-key")
	results, err := provider.Search(context.Background(), "Go programming", 5)
	if err != nil {
		t.Fatalf("Exa search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 result")
	}
}

func TestSearchTavily(t *testing.T) {
	provider := NewTavilyProvider("test-key")
	results, err := provider.Search(context.Background(), "Go programming", 5)
	if err != nil {
		t.Fatalf("Tavily search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("Expected at least 1 result")
	}
}

func TestSearchHandlerRegisterAndSearch(t *testing.T) {
	handler := NewHandler()
	mockResults := []SearchResult{
		{Title: "Test Result 1", URL: "https://example.com/1", Snippet: "Snippet 1", Source: "mock"},
		{Title: "Test Result 2", URL: "https://example.com/2", Snippet: "Snippet 2", Source: "mock"},
	}
	handler.RegisterProvider(NewMockProvider("mock", mockResults, nil))

	resp, err := handler.Search(context.Background(), "test query", "mock", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Expected 2 results, got %d", resp.Count)
	}
	if resp.Provider != "mock" {
		t.Errorf("Expected provider 'mock', got '%s'", resp.Provider)
	}
}

func TestSearchHandlerDefaultProvider(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("first", []SearchResult{{Title: "1"}}, nil))
	handler.RegisterProvider(NewMockProvider("second", []SearchResult{{Title: "2"}}, nil))

	// Default should be the first registered
	resp, err := handler.Search(context.Background(), "query", "", 10)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if resp.Provider != "first" {
		t.Errorf("Expected default provider 'first', got '%s'", resp.Provider)
	}
}

func TestSearchHandlerEmptyQuery(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("mock", []SearchResult{}, nil))

	_, err := handler.Search(context.Background(), "", "mock", 10)
	if err == nil {
		t.Error("Expected error for empty query")
	}
}

func TestSearchHandlerUnknownProvider(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("mock", []SearchResult{}, nil))

	_, err := handler.Search(context.Background(), "query", "nonexistent", 10)
	if err == nil {
		t.Error("Expected error for unknown provider")
	}
}

func TestSearchHandlerAvailableProviders(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("p1", nil, nil))
	handler.RegisterProvider(NewMockProvider("p2", nil, nil))
	handler.RegisterProvider(NewMockProvider("p3", nil, nil))

	providers := handler.AvailableProviders()
	if len(providers) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(providers))
	}
}

func TestSearchHandlerServeHTTP(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("mock", []SearchResult{
		{Title: "HTTP Test", URL: "https://example.com", Snippet: "Test snippet", Source: "mock"},
	}, nil))

	server := httptest.NewServer(handler)
	defer server.Close()

	resp, err := http.Get(server.URL + "?q=test&provider=mock&max_results=5")
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var searchResp SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if searchResp.Count != 1 {
		t.Errorf("Expected 1 result, got %d", searchResp.Count)
	}
}

func TestSearchAllProviders(t *testing.T) {
	handler := NewHandler()
	handler.RegisterProvider(NewMockProvider("p1", []SearchResult{{Title: "1", Source: "p1"}}, nil))
	handler.RegisterProvider(NewMockProvider("p2", []SearchResult{{Title: "2", Source: "p2"}}, nil))

	resp, err := handler.SearchAll(context.Background(), "test", 10)
	if err != nil {
		t.Fatalf("SearchAll failed: %v", err)
	}
	if resp.Count != 2 {
		t.Errorf("Expected 2 results from all providers, got %d", resp.Count)
	}
	if resp.Provider != "all" {
		t.Errorf("Expected provider 'all', got '%s'", resp.Provider)
	}
}

func TestSearchProviderNoAPIKey(t *testing.T) {
	provider := NewPerplexityProvider("")
	_, err := provider.Search(context.Background(), "test", 5)
	if err == nil {
		t.Error("Expected error when API key is empty")
	}
}

func TestIsValidProvider(t *testing.T) {
	valid := []string{"perplexity", "brave", "serper", "exa", "tavily"}
	for _, p := range valid {
		if !IsValidProvider(p) {
			t.Errorf("Expected %s to be valid", p)
		}
	}
	if IsValidProvider("nonexistent") {
		t.Error("Expected 'nonexistent' to be invalid")
	}
}
