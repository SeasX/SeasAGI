package oauth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*TokenInfo
	dir    string
}

func NewTokenStore(dir string) *TokenStore {
	return &TokenStore{
		tokens: make(map[string]*TokenInfo),
		dir:    dir,
	}
}

func (s *TokenStore) Save(provider string, info *TokenInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[provider] = info
	return s.persist()
}

func (s *TokenStore) Get(provider string) (*TokenInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.tokens[provider]
	if !ok {
		return nil, fmt.Errorf("no token for provider %s", provider)
	}
	return info, nil
}

func (s *TokenStore) Delete(provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, provider)
	return s.persist()
}

func (s *TokenStore) ListProviders() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.tokens))
	for k := range s.tokens {
		result = append(result, k)
	}
	return result
}

func (s *TokenStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.dir, "oauth_tokens.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.tokens)
}

func (s *TokenStore) persist() error {
	if s.dir == "" {
		return nil
	}

	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(s.dir, "oauth_tokens.json")
	tmpPath := path + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

type AutoRefresher struct {
	store   *TokenStore
	configs map[string]*OAuthConfig
	mu      sync.RWMutex
	stopCh  chan struct{}
}

func NewAutoRefresher(store *TokenStore) *AutoRefresher {
	return &AutoRefresher{
		store:   store,
		configs: make(map[string]*OAuthConfig),
		stopCh:  make(chan struct{}),
	}
}

func (ar *AutoRefresher) RegisterProvider(provider string, cfg *OAuthConfig) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	ar.configs[provider] = cfg
}

func (ar *AutoRefresher) Start() {
	go ar.run()
}

func (ar *AutoRefresher) Stop() {
	close(ar.stopCh)
}

func (ar *AutoRefresher) run() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ar.stopCh:
			return
		case <-ticker.C:
			ar.refreshAll()
		}
	}
}

func (ar *AutoRefresher) refreshAll() {
	ar.mu.RLock()
	configs := make(map[string]*OAuthConfig)
	for k, v := range ar.configs {
		configs[k] = v
	}
	ar.mu.RUnlock()

	for provider, cfg := range configs {
		info, err := ar.store.Get(provider)
		if err != nil {
			continue
		}

		if info.RefreshToken == "" {
			continue
		}

		if !info.IsExpired() {
			continue
		}

		resp, err := RefreshToken(cfg, info.RefreshToken)
		if err != nil {
			continue
		}

		newInfo := TokenResponseToInfo(resp)
		_ = ar.store.Save(provider, newInfo)
	}
}

func (ar *AutoRefresher) GetValidToken(provider string) (string, error) {
	info, err := ar.store.Get(provider)
	if err != nil {
		return "", err
	}

	if !info.IsExpired() {
		return info.AccessToken, nil
	}

	ar.mu.RLock()
	cfg, ok := ar.configs[provider]
	ar.mu.RUnlock()

	if !ok || info.RefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh available for %s", provider)
	}

	resp, err := RefreshToken(cfg, info.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh failed for %s: %w", provider, err)
	}

	newInfo := TokenResponseToInfo(resp)
	if err := ar.store.Save(provider, newInfo); err != nil {
		return "", err
	}

	return newInfo.AccessToken, nil
}
