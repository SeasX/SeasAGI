package config

import (
	"sync"
)

type AppConfig struct {
	ListenPort            int                 `json:"listen_port"`
	DefaultModel          string              `json:"default_model"`
	DefaultChannelID      string              `json:"default_channel_id"`
	RoutingStrategy       string              `json:"routing_strategy"`
	StickyChannelUse      int                 `json:"sticky_channel_use"`
	AutoLaunch            bool                `json:"auto_launch"`
	AutoUpdate            bool                `json:"auto_update"`
	LogRetentionDays      int                 `json:"log_retention_days"`
	AnalyticsEnabled      bool                `json:"analytics_enabled"`
	Locale                string              `json:"locale"`
	RTKEnabled            bool                `json:"rtk_enabled"`
	RTKMaxOutputChars     int                 `json:"rtk_max_output_chars"`
	CavemanEnabled        bool                `json:"caveman_enabled"`
	CavemanStyle          string              `json:"caveman_style"`
	ModelCombos           []ModelCombo        `json:"model_combos,omitempty"`
	ComboTemplates        []ModelCombo        `json:"combo_templates,omitempty"`
	Optimizations         *OptimizationConfig `json:"optimizations,omitempty"`
	PlatformAPIBaseURL    string              `json:"platform_api_base_url,omitempty"`
	DefaultComboName      string              `json:"default_combo_name,omitempty"`
	RateLimit             *RateLimitConfig    `json:"rate_limit,omitempty"`
	Security              *SecurityConfig     `json:"security,omitempty"`
	MITMRouting           *MITMRoutingConfig  `json:"mitm_routing,omitempty"`
	SelectedGrantID       string              `json:"selected_grant_id,omitempty"`
	SelectedGrantRelayURL string              `json:"selected_grant_relay_url,omitempty"`
	// MarketNode 是 A 端节点（Inbound Relay）配置；共享密钥只存系统钥匙串，不落配置文件。
	MarketNode *MarketNodeConfig `json:"market_node,omitempty"`
}

// MarketNodeConfig 描述 A 端节点的注册信息与监听端口。
type MarketNodeConfig struct {
	NodeID        string   `json:"node_id,omitempty"`
	NodeURL       string   `json:"node_url,omitempty"`
	ListenPort    int      `json:"listen_port,omitempty"`
	Models        []string `json:"models,omitempty"`
	WireProtocols []string `json:"wire_protocols,omitempty"`
	Enabled       bool     `json:"enabled"`
}

// RateLimitConfig 全局速率限制配置，借鉴 OmniRoute per-connection rateLimitOverrides。
type RateLimitConfig struct {
	Enabled             bool                         `json:"enabled"`
	DefaultRPM          int                          `json:"default_rpm"`                 // 每分钟请求数，0=不限
	DefaultTPM          int                          `json:"default_tpm"`                 // 每分钟 token 数，0=不限
	MinIntervalMs       int                          `json:"min_interval_ms"`             // 请求间最小间隔（毫秒），0=不限
	MaxConcurrent       int                          `json:"max_concurrent"`              // 最大并发数，0=不限
	MaxWaitMs           int                          `json:"max_wait_ms"`                 // 队列最大等待（毫秒），默认 15000
	MonthlyCostLimitUSD float64                      `json:"monthly_cost_limit_usd"`      // 月度成本硬上限（USD），0=不限
	ChannelOverrides    map[string]*ChannelRateLimit `json:"channel_overrides,omitempty"` // per-channel 覆盖
}

// ChannelRateLimit 单个 Channel 的速率限制覆盖。
type ChannelRateLimit struct {
	RPM           int `json:"rpm"`             // 0=使用全局默认
	TPM           int `json:"tpm"`             // 0=使用全局默认
	MinIntervalMs int `json:"min_interval_ms"` // 0=使用全局默认
	MaxConcurrent int `json:"max_concurrent"`  // 0=使用全局默认
}

// 提示注入处置策略。
const (
	PromptInjectionOff   = "off"   // 不检测
	PromptInjectionLog   = "log"   // 命中告警但不拦截（默认）
	PromptInjectionBlock = "block" // 命中直接拦截
)

// SecurityConfig 网关内容治理策略（默认非破坏式：仅告警 + 错误脱敏）。
type SecurityConfig struct {
	PIIMaskingEnabled     bool   `json:"pii_masking_enabled"`     // 对请求 messages 做 PII/DLP 脱敏（默认关闭）
	PromptInjectionAction string `json:"prompt_injection_action"` // off | log | block（默认 log）
	ErrorSanitizeEnabled  bool   `json:"error_sanitize_enabled"`  // 对外错误信息脱敏（默认开启）
}

// MITM 路由动作。
const (
	MITMActionPassThrough = "passthrough"  // 不改写（默认）
	MITMActionCombo       = "combo"        // 改写为 Combo 名
	MITMActionModel       = "model"        // 改写为逻辑模型名
	MITMActionMarketAsset = "market_asset" // 直选 Token 市场订阅资产（grant_id）
)

// MITM 规则来源：本地规则（可编辑、置顶）与企业下发规则（只读、置底）。
const (
	MITMRuleSourceLocal      = "local"
	MITMRuleSourceEnterprise = "enterprise"
)

// MITM 头部一致性校验模式：
//   - lenient（默认）：仅记审计日志，不阻断请求；
//   - strict：`Target` 与 `Original-Host` 不一致时拒绝并返回 4xx。
const (
	MITMSecurityLenient = "lenient"
	MITMSecurityStrict  = "strict"
)

// MarketGrantChannelPrefix 是 Combo 步骤中引用 Token 市场资产（Grant）的虚拟渠道前缀，
// 形如 `grant://{grant_id}`。由 resolver 识别为虚拟渠道并经企业 relay 转发。
const MarketGrantChannelPrefix = "grant://"

// MITM 来源标记头：由 MITM proxy 注入，gateway 侧据此做「来源 → 目标」路由映射。
const (
	HeaderMITMSource   = "X-SeasAGI-MITM"
	HeaderMITMTarget   = "X-SeasAGI-MITM-Target"
	HeaderMITMHost     = "X-SeasAGI-MITM-Original-Host"
	HeaderMITMEndpoint = "X-SeasAGI-MITM-Endpoint"
)

// MITMRoutingConfig MITM 接管请求的路由映射策略。默认（nil / Enabled=false）行为与未启用
// 时完全一致。
type MITMRoutingConfig struct {
	Enabled bool `json:"enabled"`
	// DefaultAction 未命中任何规则时的动作：passthrough（默认）| combo | model。
	DefaultAction string `json:"default_action"`
	DefaultCombo  string `json:"default_combo_name,omitempty"`
	DefaultModel  string `json:"default_model,omitempty"`
	// SecurityMode 头部一致性校验模式：lenient（默认，仅审计）| strict（拒绝 4xx）。
	SecurityMode string            `json:"security_mode,omitempty"`
	Rules        []MITMRoutingRule `json:"rules,omitempty"`
}

// MITMRoutingRule 单条「来源 → 目标」映射规则。
type MITMRoutingRule struct {
	ID           string `json:"id"`
	Enabled      bool   `json:"enabled"`
	Priority     int    `json:"priority"`                // 小者优先；同级按配置顺序
	TargetID     string `json:"target_id,omitempty"`     // 来源工具；空/`*` = 任意
	EndpointPath string `json:"endpoint_path,omitempty"` // 路径前缀；空 = 任意
	RequestModel string `json:"request_model,omitempty"` // 请求模型；空 = 任意；支持 `*` 后缀通配
	Action       string `json:"action"`                  // combo | model | market_asset | passthrough
	ComboName    string `json:"combo_name,omitempty"`    // action=combo
	Model        string `json:"model,omitempty"`         // action=model（逻辑模型名）
	// GrantID 用于 action=market_asset：直选某个 Token 市场订阅资产。
	GrantID string `json:"grant_id,omitempty"`
	// Constraints 随映射注入的请求级约束（复用 FilterCandidatesByConstraints）。
	Constraints map[string]any `json:"constraints,omitempty"`
	// Source 规则来源：local（默认，可编辑、置顶）| enterprise（企业下发、只读、置底）。
	Source string `json:"source,omitempty"`
}

type OptimizationConfig struct {
	Mode               string `json:"mode"`
	PenaltyEnabled     bool   `json:"penalty_enabled"`
	PenaltyDecaySec    int    `json:"penalty_decay_sec"`
	HealthCheckEnabled bool   `json:"health_check_enabled"`
	HealthCheckSec     int    `json:"health_check_sec"`
	HealthMaxFailures  int    `json:"health_max_failures"`
	CooldownEnabled    bool   `json:"cooldown_enabled"`
	CooldownSec        int    `json:"cooldown_sec"`
	StickyEnabled      bool   `json:"sticky_enabled"`
	StickyTTLSec       int    `json:"sticky_ttl_sec"`
	PresetEnabled      bool   `json:"preset_enabled"`
	DefaultPreset      string `json:"default_preset"`
}

type CandidateProvider struct {
	ChannelID    string `json:"channel_id"`
	Model        string `json:"model"`
	Priority     int    `json:"priority"`
	HealthStatus string `json:"health_status,omitempty"`
}

type ModelComboStep struct {
	ChannelID                  string              `json:"channel_id,omitempty"`
	Model                      string              `json:"model"`
	StepRole                   string              `json:"step_role,omitempty"`
	Providers                  []CandidateProvider `json:"providers,omitempty"`
	Channels                   []string            `json:"channels,omitempty"`
	SelectionPolicy            string              `json:"selection_policy,omitempty"`
	AllowProviderFallback      bool                `json:"allow_provider_fallback,omitempty"`
	AllowCrossProviderFallback bool                `json:"allow_cross_provider_fallback,omitempty"`
}

type ModelCombo struct {
	ComboID       string           `json:"combo_id,omitempty"`
	Name          string           `json:"name"`
	LogicalName   string           `json:"logical_name,omitempty"`
	DisplayName   string           `json:"display_name,omitempty"`
	Description   string           `json:"description,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Steps         []ModelComboStep `json:"steps,omitempty"`
	Models        []string         `json:"models,omitempty"`
	Strategy      string           `json:"strategy"`
	StickyUses    int              `json:"sticky_uses"`
	QuickStrategy string           `json:"quick_strategy,omitempty"`
	TaskProfile   map[string]any   `json:"task_profile,omitempty"`
	Status        string           `json:"status,omitempty"`
	Source        string           `json:"source,omitempty"`
	Version       int              `json:"version,omitempty"`
	// MarketPriority 控制 grant:// 市场资产步骤的优先级（P3-21）：
	// nil/true = 市场优先（先试市场资产，枯竭回退自有 Key）；显式 false = 忽略市场资产，仅用自有 Key。
	MarketPriority *bool `json:"market_priority,omitempty"`
}

type RetryConfig struct {
	MaxRetries    int    `json:"max_retries"`
	InitialDelay  string `json:"initial_delay"`
	MaxDelay      string `json:"max_delay"`
	RetryableOnly bool   `json:"retryable_only"`
}

type Channel struct {
	ChannelID              string            `json:"channel_id"`
	ChannelType            string            `json:"channel_type"`
	ProviderType           string            `json:"provider_type"`
	DisplayName            string            `json:"display_name"`
	BaseURL                string            `json:"base_url"`
	Enabled                bool              `json:"enabled"`
	HealthStatus           string            `json:"health_status"`
	Source                 string            `json:"source,omitempty"` // "enterprise", "platform", ""
	ProviderSpecificConfig map[string]string `json:"provider_specific_config"`
	SupportedModalities    []string          `json:"supported_modalities,omitempty"`
	Models                 []string          `json:"models,omitempty"`
	APIKey                 string            `json:"api_key,omitempty"`
	APIKeys                []string          `json:"api_keys,omitempty"`
	RetryConfig            *RetryConfig      `json:"retry_config,omitempty"`
	Weight                 int               `json:"weight,omitempty"`         // 0=default(1); higher=more traffic in WRR
	Priority               int               `json:"priority,omitempty"`       // lower=higher priority; 0=default
	MaxConcurrent          int               `json:"max_concurrent,omitempty"` // 0=unlimited; per-channel in-flight limit
	OAuthRefreshToken      string            `json:"oauth_refresh_token,omitempty"`
	OAuthClientID          string            `json:"oauth_client_id,omitempty"`
	OAuthClientSecret      string            `json:"oauth_client_secret,omitempty"`
	OAuthTokenURL          string            `json:"oauth_token_url,omitempty"`
}

// ChannelPublic 是渠道的对外白名单 DTO：只包含展示/路由所需的非敏感字段，
// 任何凭证（单/多 API Key、OAuth refresh token、client secret）都不在其中。
// 列表与配置导出统一经它构造，避免 sanitizeChannel 漏字段导致凭证外泄（CL-S1）。
type ChannelPublic struct {
	ChannelID              string            `json:"channel_id"`
	ChannelType            string            `json:"channel_type"`
	ProviderType           string            `json:"provider_type"`
	DisplayName            string            `json:"display_name"`
	BaseURL                string            `json:"base_url"`
	Enabled                bool              `json:"enabled"`
	HealthStatus           string            `json:"health_status"`
	Source                 string            `json:"source,omitempty"`
	ProviderSpecificConfig map[string]string `json:"provider_specific_config"`
	SupportedModalities    []string          `json:"supported_modalities,omitempty"`
	Models                 []string          `json:"models,omitempty"`
	RetryConfig            *RetryConfig      `json:"retry_config,omitempty"`
	Weight                 int               `json:"weight,omitempty"`
	Priority               int               `json:"priority,omitempty"`
	MaxConcurrent          int               `json:"max_concurrent,omitempty"`
	// OAuth 非敏感元数据：客户端 id 与 token 端点可用于展示，凭证本身不外传。
	OAuthClientID string `json:"oauth_client_id,omitempty"`
	OAuthTokenURL string `json:"oauth_token_url,omitempty"`
	// AuthType 描述该渠道使用的鉴权方式：api_key / oauth / none。
	AuthType string `json:"auth_type,omitempty"`
}

// credentialStore 抽象系统安全存储，便于在测试中注入写入失败，验证 fail-closed（CL-S3）。
type credentialStore interface {
	SaveChannelKey(channelID, apiKey string) error
	GetChannelKey(channelID string) (string, error)
	SaveChannelKeys(channelID string, apiKeys []string) error
	GetChannelKeys(channelID string) ([]string, error)
	SaveSecret(name, value string) error
	GetSecret(name string) (string, error)
}

// keychainCredentialStore 生产实现：委托给 macOS 钥匙串 / 环境变量。
type keychainCredentialStore struct{}

// pendingChannelSecrets 是 keychain 写入失败时保留的待重试凭证副本。
type pendingChannelSecrets struct {
	apiKey            string
	apiKeys           []string
	oauthRefreshToken string
	oauthClientSecret string
}

type Service struct {
	mu       sync.RWMutex
	config   AppConfig
	channels []Channel
	path     string
	// secrets 凭证安全存储（默认 keychain）；测试可注入失败实现。
	secrets credentialStore
	// pendingSecrets 记录 keychain 写入失败、等待下次保存重试的凭证（fail-closed，不清空内存）。
	pendingSecrets map[string]pendingChannelSecrets
	// savedSecretSig 记录各渠道上次成功写入 keychain 的凭证签名，避免每次保存重复写钥匙串。
	savedSecretSig map[string]string
}

type persistedState struct {
	Config   AppConfig `json:"config"`
	Channels []Channel `json:"channels"`
}

// credentialStore 返回当前凭证存储实现；零值 Service（测试字面量）回退 keychain。
func (s *Service) credentialStore() credentialStore {
	if s.secrets != nil {
		return s.secrets
	}
	return keychainCredentialStore{}
}
