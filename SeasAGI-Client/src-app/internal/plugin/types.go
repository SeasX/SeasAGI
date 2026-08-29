package plugin

// HookEvent Hook 事件类型。
type HookEvent string

const (
	HookOnRequest       HookEvent = "onRequest"
	HookOnResponse      HookEvent = "onResponse"
	HookOnError         HookEvent = "onError"
	HookOnModelSelect   HookEvent = "onModelSelect"
	HookOnComboResolve  HookEvent = "onComboResolve"
	HookOnRateLimit     HookEvent = "onRateLimit"
	HookOnQuotaExhaust  HookEvent = "onQuotaExhaust"
	HookOnProviderError HookEvent = "onProviderError"
	HookOnStreamStart   HookEvent = "onStreamStart"
	HookOnStreamEnd     HookEvent = "onStreamEnd"
	HookOnInstall       HookEvent = "onInstall"
	HookOnActivate      HookEvent = "onActivate"
	HookOnDeactivate    HookEvent = "onDeactivate"
	HookOnUninstall     HookEvent = "onUninstall"
)

// BuiltinEvents 内置事件列表（14 个）。
var BuiltinEvents = []HookEvent{
	HookOnRequest,
	HookOnResponse,
	HookOnError,
	HookOnModelSelect,
	HookOnComboResolve,
	HookOnRateLimit,
	HookOnQuotaExhaust,
	HookOnProviderError,
	HookOnStreamStart,
	HookOnStreamEnd,
	HookOnInstall,
	HookOnActivate,
	HookOnDeactivate,
	HookOnUninstall,
}

// PluginContext 插件上下文（传递给 hook handler）。
type PluginContext struct {
	RequestID  string
	Body       interface{}
	Model      string
	Provider   string
	APIKeyInfo interface{}
	Metadata   map[string]interface{}
}

// BlockingResult 阻塞式 hook 的返回结果。
type BlockingResult struct {
	Blocked  bool
	Response interface{}
	Body     interface{}
	Metadata map[string]interface{}
}

// HookHandler hook 处理函数类型。
type HookHandler func(ctx *PluginContext) *BlockingResult

// Plugin 插件接口。
type Plugin struct {
	Name     string
	Priority int
	Enabled  bool

	OnRequest       HookHandler
	OnResponse      HookHandler
	OnError         HookHandler
	OnModelSelect   HookHandler
	OnComboResolve  HookHandler
	OnRateLimit     HookHandler
	OnQuotaExhaust  HookHandler
	OnProviderError HookHandler
	OnStreamStart   HookHandler
	OnStreamEnd     HookHandler
	OnInstall       HookHandler
	OnActivate      HookHandler
	OnDeactivate    HookHandler
	OnUninstall     HookHandler
}

// HookRegistration hook 注册记录。
type HookRegistration struct {
	PluginName string
	Handler    HookHandler
	Priority   int
}
