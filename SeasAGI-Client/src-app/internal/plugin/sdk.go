package plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// --- P1-16: Plugin SDK Architecture ---

// PluginManifest describes a plugin's metadata and capabilities.
type PluginManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Homepage    string   `json:"homepage,omitempty"`
	Source      string   `json:"source,omitempty"` // GitHub repo URL

	// Capability declarations
	Hooks          []string `json:"hooks,omitempty"`           // Which hooks this plugin registers
	Translators    []string `json:"translators,omitempty"`     // Which format translations it provides
	CustomSelector bool     `json:"custom_selector,omitempty"`  // Can override routing selector
	Interceptors   []string `json:"interceptors,omitempty"`    // Request interceptor types

	// Entry point
	Entry string `json:"entry,omitempty"` // Entry function symbol
}

// PluginSDKVersion is the current SDK API version.
const PluginSDKVersion = "1.0.0"

// SDKInfo returns the SDK version info.
func SDKInfo() map[string]string {
	return map[string]string{
		"sdk_version": PluginSDKVersion,
		"hooks":       fmt.Sprintf("%d builtin events", len(BuiltinEvents)),
	}
}

// --- P1-17: Plugin Translation Hooks (5 interception points) ---

// TranslationHookEvent defines the 5 translation interception points.
type TranslationHookEvent string

const (
	HookNormalizeRequest         TranslationHookEvent = "normalizeRequest"
	HookTranslateRequest         TranslationHookEvent = "translateRequest"
	HookNormalizeResponseBefore  TranslationHookEvent = "normalizeResponseBefore"
	HookTranslateResponse        TranslationHookEvent = "translateResponse"
	HookNormalizeResponseAfter   TranslationHookEvent = "normalizeResponseAfter"
)

// TranslationHookHandler processes a request/response at a specific translation point.
// Returns the (possibly modified) body and whether the hook fully handled the translation
// (in which case the built-in translator is skipped).
type TranslationHookHandler func(body map[string]any, sourceFormat, targetFormat string) (map[string]any, bool, error)

// TranslationHookRegistration records a translation hook subscription.
type TranslationHookRegistration struct {
	PluginName string
	Handler    TranslationHookHandler
	Priority    int
}

// TranslationHookRegistry manages translation hook subscriptions.
type TranslationHookRegistry struct {
	mu       sync.RWMutex
	hooks    map[TranslationHookEvent][]TranslationHookRegistration
}

// NewTranslationHookRegistry creates a new translation hook registry.
func NewTranslationHookRegistry() *TranslationHookRegistry {
	return &TranslationHookRegistry{
		hooks: make(map[TranslationHookEvent][]TranslationHookRegistration),
	}
}

// RegisterTranslationHook subscribes a handler to a translation interception point.
func (t *TranslationHookRegistry) RegisterTranslationHook(event TranslationHookEvent, pluginName string, handler TranslationHookHandler, priority int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.hooks[event] = append(t.hooks[event], TranslationHookRegistration{
		PluginName: pluginName,
		Handler:    handler,
		Priority:   priority,
	})

	// Sort by priority (lower = earlier)
	for i := len(t.hooks[event]) - 1; i > 0; i-- {
		if t.hooks[event][i].Priority < t.hooks[event][i-1].Priority {
			t.hooks[event][i], t.hooks[event][i-1] = t.hooks[event][i-1], t.hooks[event][i]
		}
	}
}

// EmitTranslationHook runs all handlers for a translation point.
// Returns the (possibly modified) body and a `handled` flag.
// If any handler returns handled=true, subsequent handlers are skipped and
// the built-in translator is bypassed.
func (t *TranslationHookRegistry) EmitTranslationHook(event TranslationHookEvent, body map[string]any, sourceFormat, targetFormat string) (map[string]any, bool, error) {
	t.mu.RLock()
	list := t.hooks[event]
	t.mu.RUnlock()

	currentBody := body
	for _, reg := range list {
		modified, handled, err := reg.Handler(currentBody, sourceFormat, targetFormat)
		if err != nil {
			return currentBody, false, fmt.Errorf("translation hook %s from plugin %s: %w", event, reg.PluginName, err)
		}
		if modified != nil {
			currentBody = modified
		}
		if handled {
			return currentBody, true, nil
		}
	}
	return currentBody, false, nil
}

// --- P1-18: Plugin Store ---

// PluginSource describes where a plugin was installed from.
type PluginSource string

const (
	PluginSourceGitHub   PluginSource = "github"
	PluginSourceLocal    PluginSource = "local"
	PluginSourceRegistry PluginSource = "registry"
)

// InstalledPlugin tracks an installed plugin's metadata.
type InstalledPlugin struct {
	Manifest  PluginManifest
	Source    PluginSource
	SourceURL string
	InstallDir string
	Enabled   bool
}

// PluginStore manages plugin installation, updates, and removal.
type PluginStore struct {
	mu       sync.RWMutex
	plugins  map[string]*InstalledPlugin
	baseDir  string
}

// NewPluginStore creates a plugin store with the given base directory.
func NewPluginStore(baseDir string) *PluginStore {
	if baseDir == "" {
		home, _ := os.UserHomeDir()
		baseDir = filepath.Join(home, "Library", "Application Support", "SeasAGI", "plugins")
	}
	_ = os.MkdirAll(baseDir, 0o755)
	return &PluginStore{
		plugins: make(map[string]*InstalledPlugin),
		baseDir: baseDir,
	}
}

// InstallFromGitHub downloads and installs a plugin from a GitHub repo.
func (s *PluginStore) InstallFromGitHub(repoURL, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Download manifest
	manifestURL := fmt.Sprintf("%s/raw/%s/plugin.json", repoURL, defaultBranch(version))
	resp, err := http.Get(manifestURL)
	if err != nil {
		return fmt.Errorf("plugin store: fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("plugin store: manifest not found (HTTP %d)", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("plugin store: read manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("plugin store: parse manifest: %w", err)
	}

	if manifest.Name == "" {
		return fmt.Errorf("plugin store: manifest missing name")
	}

	installDir := filepath.Join(s.baseDir, manifest.Name)
	_ = os.MkdirAll(installDir, 0o755)

	// Save manifest
	manifestPath := filepath.Join(installDir, "plugin.json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("plugin store: save manifest: %w", err)
	}

	s.plugins[manifest.Name] = &InstalledPlugin{
		Manifest:  manifest,
		Source:    PluginSourceGitHub,
		SourceURL: repoURL,
		InstallDir: installDir,
		Enabled:   false,
	}

	return nil
}

// InstallFromLocal installs a plugin from a local directory.
func (s *PluginStore) InstallFromLocal(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	manifestPath := filepath.Join(dir, "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("plugin store: read local manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("plugin store: parse local manifest: %w", err)
	}

	s.plugins[manifest.Name] = &InstalledPlugin{
		Manifest:  manifest,
		Source:    PluginSourceLocal,
		SourceURL: dir,
		InstallDir: dir,
		Enabled:   false,
	}
	return nil
}

// Uninstall removes a plugin.
func (s *PluginStore) Uninstall(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.plugins[name]
	if !ok {
		return fmt.Errorf("plugin store: %q not installed", name)
	}

	if p.Source == PluginSourceGitHub || p.Source == PluginSourceRegistry {
		_ = os.RemoveAll(p.InstallDir)
	}
	delete(s.plugins, name)
	return nil
}

// ListInstalled returns all installed plugins.
func (s *PluginStore) ListInstalled() []*InstalledPlugin {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*InstalledPlugin, 0, len(s.plugins))
	for _, p := range s.plugins {
		result = append(result, p)
	}
	return result
}

// GetInstalled returns a specific installed plugin.
func (s *PluginStore) GetInstalled(name string) *InstalledPlugin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.plugins[name]
}

// CheckUpdate checks if an update is available for a GitHub-sourced plugin.
func (s *PluginStore) CheckUpdate(name string) (string, bool, error) {
	s.mu.RLock()
	p, ok := s.plugins[name]
	s.mu.RUnlock()

	if !ok {
		return "", false, fmt.Errorf("plugin %q not installed", name)
	}
	if p.Source != PluginSourceGitHub {
		return "", false, nil
	}

	// Fetch latest manifest
	manifestURL := fmt.Sprintf("%s/raw/main/plugin.json", p.SourceURL)
	resp, err := http.Get(manifestURL)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", false, nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, err
	}

	var latest PluginManifest
	if err := json.Unmarshal(data, &latest); err != nil {
		return "", false, err
	}

	if latest.Version != p.Manifest.Version {
		return latest.Version, true, nil
	}
	return p.Manifest.Version, false, nil
}

func defaultBranch(version string) string {
	if version == "" {
		return "main"
	}
	return version
}

// --- P1-19: Plugin Scheduler Override ---

// PluginScheduler is an interface that plugins can implement to override
// the built-in channel selector.
type PluginScheduler interface {
	// SelectChannel chooses a channel for the given request.
	// candidates is the list of channels that can serve the model.
	// Returns the selected channel ID and the index in candidates.
	SelectChannel(model string, candidates []map[string]any) (string, int)
}

// SchedulerRegistry manages plugin-provided custom schedulers.
type SchedulerRegistry struct {
	mu         sync.RWMutex
	schedulers map[string]PluginScheduler // plugin name → scheduler
}

// NewSchedulerRegistry creates a new scheduler registry.
func NewSchedulerRegistry() *SchedulerRegistry {
	return &SchedulerRegistry{
		schedulers: make(map[string]PluginScheduler),
	}
}

// RegisterScheduler registers a plugin's custom scheduler.
func (s *SchedulerRegistry) RegisterScheduler(pluginName string, scheduler PluginScheduler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedulers[pluginName] = scheduler
}

// UnregisterScheduler removes a plugin's scheduler.
func (s *SchedulerRegistry) UnregisterScheduler(pluginName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.schedulers, pluginName)
}

// GetScheduler returns the first registered plugin scheduler, if any.
// This allows a plugin to fully override the built-in selector.
func (s *SchedulerRegistry) GetScheduler() PluginScheduler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sched := range s.schedulers {
		return sched
	}
	return nil
}

// --- P1-20: Request Interceptor ---

// RequestInterceptor intercepts requests after auth resolution but before
// forwarding to the upstream provider. This allows plugins to modify the
// request body, add headers, or block the request.
type RequestInterceptor interface {
	// Intercept is called after the channel/auth is resolved but before
	// the request is sent upstream.
	// Returns: modifiedBody, headers, blocked, error
	Intercept(ctx *PluginContext, body map[string]any, channelID, model string) (map[string]any, map[string]string, bool, error)
}

// InterceptorRegistry manages request interceptors.
type InterceptorRegistry struct {
	mu            sync.RWMutex
	interceptors  []RequestInterceptor
}

// NewInterceptorRegistry creates a new interceptor registry.
func NewInterceptorRegistry() *InterceptorRegistry {
	return &InterceptorRegistry{}
}

// RegisterInterceptor adds a request interceptor.
func (i *InterceptorRegistry) RegisterInterceptor(interceptor RequestInterceptor) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.interceptors = append(i.interceptors, interceptor)
}

// RunInterceptors executes all interceptors in order.
// Returns: finalBody, extraHeaders, blocked, error
func (i *InterceptorRegistry) RunInterceptors(ctx *PluginContext, body map[string]any, channelID, model string) (map[string]any, map[string]string, bool, error) {
	i.mu.RLock()
	list := i.interceptors
	i.mu.RUnlock()

	currentBody := body
	extraHeaders := make(map[string]string)

	for _, interceptor := range list {
		modified, headers, blocked, err := interceptor.Intercept(ctx, currentBody, channelID, model)
		if err != nil {
			return currentBody, extraHeaders, false, err
		}
		if modified != nil {
			currentBody = modified
		}
		if headers != nil {
			for k, v := range headers {
				extraHeaders[k] = v
			}
		}
		if blocked {
			return currentBody, extraHeaders, true, nil
		}
	}

	return currentBody, extraHeaders, false, nil
}
