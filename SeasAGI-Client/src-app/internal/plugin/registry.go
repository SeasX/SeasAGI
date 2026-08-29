package plugin

import (
	"sort"
	"sync"
)

// Registry 插件注册表。
type Registry struct {
	mu          sync.RWMutex
	plugins     map[string]*Plugin
	hookMap     map[HookEvent][]HookRegistration
	rateLimiter *RateLimiter
}

// NewRegistry 创建注册表。
func NewRegistry() *Registry {
	return &Registry{
		plugins:     make(map[string]*Plugin),
		hookMap:     make(map[HookEvent][]HookRegistration),
		rateLimiter: NewRateLimiter(100, 1000), // 100 次/秒
	}
}

// Register 注册插件。
func (r *Registry) Register(p *Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.Name == "" {
		return
	}

	r.plugins[p.Name] = p

	// 注册所有非 nil 的 hook
	if p.Enabled {
		r.registerHooksUnlocked(p)
	}
}

// registerHooksUnlocked 注册插件的所有 hook（不加锁）。
func (r *Registry) registerHooksUnlocked(p *Plugin) {
	hooks := r.getPluginHooks(p)
	for event, handler := range hooks {
		if handler == nil {
			continue
		}
		r.addHookUnlocked(event, p.Name, handler, p.Priority)
	}
}

// getPluginHooks 获取插件的所有 hook。
func (r *Registry) getPluginHooks(p *Plugin) map[HookEvent]HookHandler {
	return map[HookEvent]HookHandler{
		HookOnRequest:       p.OnRequest,
		HookOnResponse:      p.OnResponse,
		HookOnError:         p.OnError,
		HookOnModelSelect:   p.OnModelSelect,
		HookOnComboResolve:  p.OnComboResolve,
		HookOnRateLimit:     p.OnRateLimit,
		HookOnQuotaExhaust:  p.OnQuotaExhaust,
		HookOnProviderError: p.OnProviderError,
		HookOnStreamStart:   p.OnStreamStart,
		HookOnStreamEnd:     p.OnStreamEnd,
		HookOnInstall:       p.OnInstall,
		HookOnActivate:      p.OnActivate,
		HookOnDeactivate:    p.OnDeactivate,
		HookOnUninstall:     p.OnUninstall,
	}
}

// addHookUnlocked 添加 hook 注册（不加锁）。
func (r *Registry) addHookUnlocked(event HookEvent, pluginName string, handler HookHandler, priority int) {
	list, ok := r.hookMap[event]
	if !ok {
		list = []HookRegistration{}
	}

	// 防止重复注册
	for _, reg := range list {
		if reg.PluginName == pluginName {
			return
		}
	}

	list = append(list, HookRegistration{
		PluginName: pluginName,
		Handler:    handler,
		Priority:   priority,
	})

	// 按 priority 升序排序（数值小的先执行）
	sort.Slice(list, func(i, j int) bool {
		return list[i].Priority < list[j].Priority
	})

	r.hookMap[event] = list
}

// Unregister 注销插件。
func (r *Registry) Unregister(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.plugins, pluginName)

	// 从所有 hook 列表中移除
	for event, list := range r.hookMap {
		filtered := make([]HookRegistration, 0, len(list))
		for _, reg := range list {
			if reg.PluginName != pluginName {
				filtered = append(filtered, reg)
			}
		}
		r.hookMap[event] = filtered
	}

	// 清除限流状态
	r.rateLimiter.Reset(pluginName)
}

// Enable 启用插件。
func (r *Registry) Enable(pluginName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[pluginName]
	if !ok {
		return false
	}

	p.Enabled = true
	r.registerHooksUnlocked(p)
	return true
}

// Disable 禁用插件。
func (r *Registry) Disable(pluginName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[pluginName]
	if !ok {
		return false
	}

	p.Enabled = false

	// 从所有 hook 列表中移除
	for event, list := range r.hookMap {
		filtered := make([]HookRegistration, 0, len(list))
		for _, reg := range list {
			if reg.PluginName != pluginName {
				filtered = append(filtered, reg)
			}
		}
		r.hookMap[event] = filtered
	}

	return true
}

// IsEnabled 检查插件是否启用。
func (r *Registry) IsEnabled(pluginName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[pluginName]
	if !ok {
		return false
	}
	return p.Enabled
}

// GetPlugin 获取插件。
func (r *Registry) GetPlugin(pluginName string) *Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[pluginName]
}

// ListPlugins 列出所有插件。
func (r *Registry) ListPlugins() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// GetHooks 获取某事件的所有 hook（按优先级排序）。
func (r *Registry) GetHooks(event HookEvent) []HookRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.hookMap[event]
}

// GetActiveEvents 获取有注册 handler 的事件列表。
func (r *Registry) GetActiveEvents() []HookEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var events []HookEvent
	for event, list := range r.hookMap {
		if len(list) > 0 {
			events = append(events, event)
		}
	}
	return events
}

// Reset 清空所有注册（用于测试）。
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.plugins = make(map[string]*Plugin)
	r.hookMap = make(map[HookEvent][]HookRegistration)
	r.rateLimiter.ResetAll()
}
