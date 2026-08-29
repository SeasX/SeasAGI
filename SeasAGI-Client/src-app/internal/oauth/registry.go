package oauth

import (
	"sync"
)

type ProviderInfo struct {
	Name         string
	DisplayName  string
	AuthURL      string
	TokenURL     string
	Scopes       []string
	IconURL      string
}

type ProviderRegistry struct {
	mu       sync.RWMutex
	providers map[string]ProviderInfo
}

var globalProviderRegistry = NewProviderRegistry()

func NewProviderRegistry() *ProviderRegistry {
	r := &ProviderRegistry{
		providers: make(map[string]ProviderInfo),
	}

	r.Register(ProviderInfo{
		Name:        "google",
		DisplayName: "Google",
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    "https://oauth2.googleapis.com/token",
		Scopes:      []string{"openid", "email", "profile"},
		IconURL:     "https://www.google.com/favicon.ico",
	})

	r.Register(ProviderInfo{
		Name:        "github",
		DisplayName: "GitHub",
		AuthURL:     "https://github.com/login/oauth/authorize",
		TokenURL:    "https://github.com/login/oauth/access_token",
		Scopes:      []string{"user:email", "read:org"},
		IconURL:     "https://github.com/favicon.ico",
	})

	r.Register(ProviderInfo{
		Name:        "microsoft",
		DisplayName: "Microsoft",
		AuthURL:     "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		TokenURL:    "https://login.microsoftonline.com/common/oauth2/v2.0/token",
		Scopes:      []string{"openid", "email", "profile", "User.Read"},
		IconURL:     "https://www.microsoft.com/favicon.ico",
	})

	r.Register(ProviderInfo{
		Name:        "anthropic",
		DisplayName: "Anthropic",
		AuthURL:     "https://console.anthropic.com/oauth/authorize",
		TokenURL:    "https://console.anthropic.com/oauth/token",
		Scopes:      []string{"openid", "email"},
		IconURL:     "https://www.anthropic.com/favicon.ico",
	})

	return r
}

func (r *ProviderRegistry) Register(info ProviderInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[info.Name] = info
}

func (r *ProviderRegistry) Get(name string) (ProviderInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.providers[name]
	return info, ok
}

func (r *ProviderRegistry) List() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]ProviderInfo, 0, len(r.providers))
	for _, info := range r.providers {
		result = append(result, info)
	}
	return result
}

func RegisterOAuthProvider(info ProviderInfo) {
	globalProviderRegistry.Register(info)
}

func GetOAuthProvider(name string) (ProviderInfo, bool) {
	return globalProviderRegistry.Get(name)
}

func ListOAuthProviders() []ProviderInfo {
	return globalProviderRegistry.List()
}
