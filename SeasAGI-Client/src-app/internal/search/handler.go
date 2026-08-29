// Package search provides a unified search API that aggregates results
// from multiple search providers (Perplexity, Brave, Serper, Exa, Tavily).
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SearchResult represents a single search result item.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}

// SearchResponse is the unified response from all providers.
type SearchResponse struct {
	Query    string         `json:"query"`
	Results  []SearchResult `json:"results"`
	Provider string         `json:"provider"`
	Count    int            `json:"count"`
	Took     string         `json:"took"`
}

// SearchProvider defines the interface for a search provider.
type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}

// Handler handles search requests and routes them to the configured provider.
type Handler struct {
	providers map[string]SearchProvider
	default_  string
}

// NewHandler creates a new search handler with the given providers.
func NewHandler() *Handler {
	return &Handler{
		providers: make(map[string]SearchProvider),
	}
}

// RegisterProvider registers a search provider.
func (h *Handler) RegisterProvider(p SearchProvider) {
	h.providers[p.Name()] = p
	if h.default_ == "" {
		h.default_ = p.Name()
	}
}

// SetDefault sets the default provider.
func (h *Handler) SetDefault(name string) error {
	if _, ok := h.providers[name]; !ok {
		return fmt.Errorf("provider %s not registered", name)
	}
	h.default_ = name
	return nil
}

// GetProvider returns a provider by name.
func (h *Handler) GetProvider(name string) (SearchProvider, error) {
	p, ok := h.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %s not found", name)
	}
	return p, nil
}

// Search performs a search using the specified provider (or default if empty).
func (h *Handler) Search(ctx context.Context, query, providerName string, maxResults int) (*SearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	if maxResults <= 0 {
		maxResults = 10
	}

	provider := h.default_
	if providerName != "" {
		provider = providerName
	}

	p, err := h.GetProvider(provider)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	results, err := p.Search(ctx, query, maxResults)
	took := time.Since(start)
	if err != nil {
		return nil, err
	}

	return &SearchResponse{
		Query:    query,
		Results:  results,
		Provider: provider,
		Count:    len(results),
		Took:     took.String(),
	}, nil
}

// ServeHTTP implements http.Handler for the search API.
// GET /v1/search?q=...&provider=...&max_results=...
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if r.Method == http.MethodPost {
		var body struct {
			Query      string `json:"query"`
			Provider   string `json:"provider"`
			MaxResults int    `json:"max_results"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.Query != "" {
			query = body.Query
		}
	}

	provider := r.URL.Query().Get("provider")
	maxResults := 10
	fmt.Sscanf(r.URL.Query().Get("max_results"), "%d", &maxResults)

	resp, err := h.Search(r.Context(), query, provider, maxResults)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// AvailableProviders returns a list of registered provider names.
func (h *Handler) AvailableProviders() []string {
	names := make([]string, 0, len(h.providers))
	for name := range h.providers {
		names = append(names, name)
	}
	return names
}

// SearchAll queries all registered providers concurrently and merges results.
func (h *Handler) SearchAll(ctx context.Context, query string, maxResults int) (*SearchResponse, error) {
	var wg sync.WaitGroup
	mu := sync.Mutex{}
	allResults := []SearchResult{}

	for name, p := range h.providers {
		wg.Add(1)
		go func(name string, p SearchProvider) {
			defer wg.Done()
			results, err := p.Search(ctx, query, maxResults)
			if err != nil {
				return
			}
			mu.Lock()
			for _, r := range results {
				r.Source = name
				allResults = append(allResults, r)
			}
			mu.Unlock()
		}(name, p)
	}

	wg.Wait()

	return &SearchResponse{
		Query:    query,
		Results:  allResults,
		Provider: "all",
		Count:    len(allResults),
	}, nil
}
