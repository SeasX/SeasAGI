package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
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

// IsMarketGrantChannelID 判断渠道 ID 是否为 Token 市场 Grant 引用。
func IsMarketGrantChannelID(channelID string) bool {
	return strings.HasPrefix(channelID, MarketGrantChannelPrefix)
}

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

// Public 从完整渠道提取对外展示字段（凭证一律丢弃）。
func (ch Channel) Public() ChannelPublic {
	authType := "none"
	switch {
	case ch.OAuthRefreshToken != "" || ch.OAuthTokenURL != "":
		authType = "oauth"
	case ch.APIKey != "" || len(ch.APIKeys) > 0:
		authType = "api_key"
	}
	return ChannelPublic{
		ChannelID:              ch.ChannelID,
		ChannelType:            ch.ChannelType,
		ProviderType:           ch.ProviderType,
		DisplayName:            ch.DisplayName,
		BaseURL:                ch.BaseURL,
		Enabled:                ch.Enabled,
		HealthStatus:           ch.HealthStatus,
		Source:                 ch.Source,
		ProviderSpecificConfig: ch.ProviderSpecificConfig,
		SupportedModalities:    ch.SupportedModalities,
		Models:                 ch.Models,
		RetryConfig:            ch.RetryConfig,
		Weight:                 ch.Weight,
		Priority:               ch.Priority,
		MaxConcurrent:          ch.MaxConcurrent,
		OAuthClientID:          ch.OAuthClientID,
		OAuthTokenURL:          ch.OAuthTokenURL,
		AuthType:               authType,
	}
}

// toChannel 把白名单 DTO 还原为 Channel（凭证字段为零值），供保持 []Channel API 兼容的列表接口使用。
func (p ChannelPublic) toChannel() Channel {
	return Channel{
		ChannelID:              p.ChannelID,
		ChannelType:            p.ChannelType,
		ProviderType:           p.ProviderType,
		DisplayName:            p.DisplayName,
		BaseURL:                p.BaseURL,
		Enabled:                p.Enabled,
		HealthStatus:           p.HealthStatus,
		Source:                 p.Source,
		ProviderSpecificConfig: p.ProviderSpecificConfig,
		SupportedModalities:    p.SupportedModalities,
		Models:                 p.Models,
		RetryConfig:            p.RetryConfig,
		Weight:                 p.Weight,
		Priority:               p.Priority,
		MaxConcurrent:          p.MaxConcurrent,
		OAuthClientID:          p.OAuthClientID,
		OAuthTokenURL:          p.OAuthTokenURL,
	}
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

func (keychainCredentialStore) SaveChannelKey(channelID, apiKey string) error {
	return keychain.SaveChannelKey(channelID, apiKey)
}
func (keychainCredentialStore) GetChannelKey(channelID string) (string, error) {
	return keychain.GetChannelKey(channelID)
}
func (keychainCredentialStore) SaveChannelKeys(channelID string, apiKeys []string) error {
	return keychain.SaveChannelKeys(channelID, apiKeys)
}
func (keychainCredentialStore) GetChannelKeys(channelID string) ([]string, error) {
	return keychain.GetChannelKeys(channelID)
}
func (keychainCredentialStore) SaveSecret(name, value string) error {
	return keychain.SaveSecret(name, value)
}
func (keychainCredentialStore) GetSecret(name string) (string, error) {
	return keychain.GetSecret(name)
}

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

func NewService(cfg AppConfig) *Service {
	return NewServiceWithPath(cfg, defaultConfigPath())
}

// NewServiceWithPath 与 NewService 相同，但配置持久化到指定路径（测试隔离用）。
func NewServiceWithPath(cfg AppConfig, path string) *Service {
	svc := &Service{
		config:         cfg,
		channels:       []Channel{},
		path:           path,
		secrets:        keychainCredentialStore{},
		pendingSecrets: make(map[string]pendingChannelSecrets),
		savedSecretSig: make(map[string]string),
	}
	_ = svc.load()
	svc.ensureOptimizationConfig()
	svc.ensureSecurityConfig()
	svc.ensureBuiltinTemplates()
	svc.migrateModelCombos()
	return svc
}

func defaultOptimizationConfig() OptimizationConfig {
	return OptimizationConfig{
		Mode:               "value_first",
		PenaltyEnabled:     true,
		PenaltyDecaySec:    120,
		HealthCheckEnabled: true,
		HealthCheckSec:     300,
		HealthMaxFailures:  3,
		CooldownEnabled:    true,
		CooldownSec:        120,
		StickyEnabled:      true,
		StickyTTLSec:       1800,
		PresetEnabled:      true,
		DefaultPreset:      "budget",
	}
}

func (s *Service) ensureOptimizationConfig() {
	if s.config.Optimizations == nil {
		def := defaultOptimizationConfig()
		s.config.Optimizations = &def
		_ = s.saveLocked()
	}
}

func defaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		PIIMaskingEnabled:     false,
		PromptInjectionAction: PromptInjectionLog,
		ErrorSanitizeEnabled:  true,
	}
}

func (s *Service) ensureSecurityConfig() {
	if s.config.Security == nil {
		def := defaultSecurityConfig()
		s.config.Security = &def
		_ = s.saveLocked()
		return
	}
	if s.config.Security.PromptInjectionAction == "" {
		s.config.Security.PromptInjectionAction = PromptInjectionLog
		_ = s.saveLocked()
	}
}

func (s *Service) ensureBuiltinTemplates() {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing := make(map[string]struct{})
	for _, t := range s.config.ComboTemplates {
		existing[t.Name] = struct{}{}
	}

	builtins := []ModelCombo{
		{
			Name:        "gpt-4o-fallback",
			Description: "GPT-4o 主力 + 备用回退，适合生产环境",
			Tags:        []string{"生产", "高可用", "OpenAI"},
			Steps: []ModelComboStep{
				{Model: "gpt-4o"},
				{Model: "gpt-4o-mini"},
			},
			Models:     []string{"gpt-4o", "gpt-4o-mini"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
		{
			Name:        "claude-opus-4-8-fallback",
			Description: "Claude Opus 4.8 主力 + Sonnet 5 回退，企业级智能",
			Tags:        []string{"生产", "企业", "Anthropic", "前沿"},
			Steps: []ModelComboStep{
				{Model: "claude-opus-4-8"},
				{Model: "claude-sonnet-5"},
			},
			Models:     []string{"claude-opus-4-8", "claude-sonnet-5"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
		{
			Name:        "claude-sonnet-round-robin",
			Description: "Claude Sonnet 5 轮询，适合高并发场景",
			Tags:        []string{"生产", "高并发", "Anthropic"},
			Steps: []ModelComboStep{
				{Model: "claude-sonnet-5"},
			},
			Models:     []string{"claude-sonnet-5"},
			Strategy:   "round_robin",
			StickyUses: 3,
		},
		{
			Name:        "fast-cheap-fallback",
			Description: "轻量模型优先，失败回退到更小模型，适合批量任务",
			Tags:        []string{"批量", "低成本"},
			Steps: []ModelComboStep{
				{Model: "gpt-4o-mini"},
				{Model: "gpt-3.5-turbo"},
			},
			Models:     []string{"gpt-4o-mini", "gpt-3.5-turbo"},
			Strategy:   "fallback",
			StickyUses: 1,
		},
	}

	changed := false
	for _, builtin := range builtins {
		if _, ok := existing[builtin.Name]; !ok {
			s.config.ComboTemplates = append(s.config.ComboTemplates, builtin)
			changed = true
		}
	}
	if changed {
		_ = s.saveLocked()
	}
}

func LoadOrDefault() (AppConfig, error) {
	svc := NewService(AppConfig{
		ListenPort:         4318,
		RoutingStrategy:    "fallback",
		StickyChannelUse:   2,
		AutoLaunch:         false,
		AutoUpdate:         true,
		LogRetentionDays:   30,
		AnalyticsEnabled:   false,
		PlatformAPIBaseURL: "https://seasagi.seasx.ai/api/v1",
		// RTK/Caveman 默认值须与前端兜底一致（SettingsPage：rtk_enabled ?? true、
		// caveman_style ?? "concise"），否则新鲜安装时后端关闭而 UI 显示开启，
		// 启用开关第一次点击不生效。已有配置文件由 load() 覆盖，不受影响。
		RTKEnabled:        true,
		RTKMaxOutputChars: 8000,
		CavemanStyle:      "concise",
	})
	if err := svc.load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return svc.config, err
	}
	return svc.config, nil
}

// generateComboID creates a unique combo identifier
func generateComboID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// migrateModelCombos migrates existing combos to the unified schema.
// It fills in missing fields (combo_id, logical_name, display_name, status, source, version).
func (s *Service) migrateModelCombos() {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	for i := range s.config.ModelCombos {
		c := &s.config.ModelCombos[i]

		// Generate combo_id if missing
		if c.ComboID == "" {
			c.ComboID = generateComboID()
			changed = true
		}

		// Set logical_name from name if empty
		if c.LogicalName == "" && c.Name != "" {
			c.LogicalName = strings.ToLower(strings.ReplaceAll(c.Name, " ", "-"))
			changed = true
		}

		// Set display_name from name if empty
		if c.DisplayName == "" && c.Name != "" {
			c.DisplayName = c.Name
			changed = true
		}

		// Set status to active if empty
		if c.Status == "" {
			c.Status = "active"
			changed = true
		}

		// Set source to local if empty
		if c.Source == "" {
			c.Source = "local"
			changed = true
		}

		// Set version to 1 if 0
		if c.Version == 0 {
			c.Version = 1
			changed = true
		}
	}

	if changed {
		_ = s.saveLocked()
	}
}

func (s *Service) GetConfig() AppConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg := s.config
	if cfg.RoutingStrategy == "" {
		cfg.RoutingStrategy = "fallback"
	}
	if cfg.StickyChannelUse <= 0 {
		cfg.StickyChannelUse = 2
	}
	return cfg
}

func (s *Service) GetOptimizationConfig() OptimizationConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.Optimizations == nil {
		return defaultOptimizationConfig()
	}
	return *s.config.Optimizations
}

func (s *Service) SetOptimizationConfig(cfg OptimizationConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.Mode == "" {
		cfg.Mode = "value_first"
	}
	s.config.Optimizations = &cfg
	return s.saveLocked()
}

func (s *Service) GetRateLimitConfig() RateLimitConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.RateLimit == nil {
		return RateLimitConfig{
			Enabled:       false,
			DefaultRPM:    60,
			MinIntervalMs: 350,
			MaxConcurrent: 6,
			MaxWaitMs:     15000,
		}
	}
	return *s.config.RateLimit
}

func (s *Service) SetRateLimitConfig(cfg RateLimitConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.RateLimit = &cfg
	return s.saveLocked()
}

func (s *Service) GetSecurityConfig() SecurityConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.Security == nil {
		return defaultSecurityConfig()
	}
	return *s.config.Security
}

func (s *Service) SetSecurityConfig(cfg SecurityConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cfg.PromptInjectionAction == "" {
		cfg.PromptInjectionAction = PromptInjectionLog
	}
	s.config.Security = &cfg
	return s.saveLocked()
}

// GetMITMRoutingConfig 返回 MITM 路由映射配置。未配置时返回 passthrough 默认值。
func (s *Service) GetMITMRoutingConfig() MITMRoutingConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.MITMRouting == nil {
		return MITMRoutingConfig{DefaultAction: MITMActionPassThrough}
	}
	return *s.config.MITMRouting
}

// SetMITMRoutingConfig 校验并持久化 MITM 路由映射配置。
func (s *Service) SetMITMRoutingConfig(cfg MITMRoutingConfig) error {
	normalized, err := normalizeMITMRoutingConfig(cfg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.MITMRouting = &normalized
	return s.saveLocked()
}

func isValidMITMAction(action string) bool {
	switch action {
	case MITMActionPassThrough, MITMActionCombo, MITMActionModel, MITMActionMarketAsset:
		return true
	}
	return false
}

func normalizeMITMRoutingConfig(cfg MITMRoutingConfig) (MITMRoutingConfig, error) {
	cfg.DefaultAction = strings.ToLower(strings.TrimSpace(cfg.DefaultAction))
	if cfg.DefaultAction == "" {
		cfg.DefaultAction = MITMActionPassThrough
	}
	if !isValidMITMAction(cfg.DefaultAction) {
		return cfg, fmt.Errorf("invalid mitm default_action: %s", cfg.DefaultAction)
	}
	cfg.DefaultCombo = strings.TrimSpace(cfg.DefaultCombo)
	cfg.DefaultModel = strings.TrimSpace(cfg.DefaultModel)
	if cfg.DefaultAction == MITMActionCombo && cfg.DefaultCombo == "" {
		return cfg, errors.New("mitm default_action=combo requires default_combo_name")
	}
	if cfg.DefaultAction == MITMActionModel && cfg.DefaultModel == "" {
		return cfg, errors.New("mitm default_action=model requires default_model")
	}

	cfg.SecurityMode = strings.ToLower(strings.TrimSpace(cfg.SecurityMode))
	if cfg.SecurityMode == "" {
		cfg.SecurityMode = MITMSecurityLenient
	}
	if cfg.SecurityMode != MITMSecurityLenient && cfg.SecurityMode != MITMSecurityStrict {
		return cfg, fmt.Errorf("invalid mitm security_mode: %s", cfg.SecurityMode)
	}

	seen := make(map[string]struct{}, len(cfg.Rules))
	rules := make([]MITMRoutingRule, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			rule.ID = generateComboID()
		}
		if _, dup := seen[rule.ID]; dup {
			continue
		}
		seen[rule.ID] = struct{}{}
		rule.TargetID = strings.TrimSpace(rule.TargetID)
		rule.EndpointPath = strings.TrimSpace(rule.EndpointPath)
		rule.RequestModel = strings.TrimSpace(rule.RequestModel)
		rule.ComboName = strings.TrimSpace(rule.ComboName)
		rule.Model = strings.TrimSpace(rule.Model)
		rule.GrantID = strings.TrimSpace(rule.GrantID)
		// 持久化的始终为本地规则（企业规则另存于内存，不落盘）。
		rule.Source = MITMRuleSourceLocal
		rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
		if rule.Action == "" {
			rule.Action = MITMActionPassThrough
		}
		if !isValidMITMAction(rule.Action) {
			return cfg, fmt.Errorf("invalid mitm rule action: %s", rule.Action)
		}
		if rule.Action == MITMActionCombo && rule.ComboName == "" {
			return cfg, fmt.Errorf("mitm rule %s action=combo requires combo_name", rule.ID)
		}
		if rule.Action == MITMActionModel && rule.Model == "" {
			return cfg, fmt.Errorf("mitm rule %s action=model requires model", rule.ID)
		}
		if rule.Action == MITMActionMarketAsset && rule.GrantID == "" {
			return cfg, fmt.Errorf("mitm rule %s action=market_asset requires grant_id", rule.ID)
		}
		rules = append(rules, rule)
	}
	cfg.Rules = rules
	return cfg, nil
}

// SetRTKConfig 设置并持久化 RTK Token 压缩配置。
func (s *Service) SetRTKConfig(enabled bool, maxOutputChars int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.RTKEnabled = enabled
	if maxOutputChars > 0 {
		s.config.RTKMaxOutputChars = maxOutputChars
	}
	return s.saveLocked()
}

// SetCavemanConfig 设置并持久化 Caveman 输出精简配置。
func (s *Service) SetCavemanConfig(enabled bool, style string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.CavemanEnabled = enabled
	if style != "" {
		s.config.CavemanStyle = style
	}
	return s.saveLocked()
}

func (s *Service) SetDefaultModel(modelName, channelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.DefaultModel = modelName
	s.config.DefaultChannelID = channelID
	_ = s.saveLocked()
}

func (s *Service) GetDefaultComboName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.DefaultComboName
}

func (s *Service) SetDefaultComboName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.DefaultComboName = name
	_ = s.saveLocked()
}

func (s *Service) GetPlatformAPIBaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.PlatformAPIBaseURL == "" {
		return "https://seasagi.seasx.ai/api/v1"
	}
	return s.config.PlatformAPIBaseURL
}

func (s *Service) SetPlatformAPIBaseURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	// 平台 API 地址会携带访问令牌，仅允许 https（本机回环可用 http）且不得指向链路本地地址。
	if err := urlguard.ValidateSecureURL(rawURL, true); err != nil {
		return fmt.Errorf("invalid platform api base url: %w", err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid platform api base url: %w", err)
	}
	if err := urlguard.RejectLinkLocalHost(u.Host); err != nil {
		return fmt.Errorf("invalid platform api base url: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.PlatformAPIBaseURL = rawURL
	return s.saveLocked()
}

// validateChannelBaseURL 校验渠道 BaseURL；空值表示沿用渠道默认端点，直接跳过。
func validateChannelBaseURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	if err := urlguard.ValidateSecureURL(rawURL, true); err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	if err := urlguard.RejectLinkLocalHost(u.Host); err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	return nil
}

func (s *Service) GetSelectedGrantID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.SelectedGrantID
}

func (s *Service) GetSelectedGrantRelayURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config.SelectedGrantRelayURL
}

func (s *Service) SetSelectedGrant(grantID, relayURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.SelectedGrantID = grantID
	s.config.SelectedGrantRelayURL = relayURL
	return s.saveLocked()
}

// GetMarketNodeConfig 返回 A 端节点配置（副本）；未配置时返回零值。
func (s *Service) GetMarketNodeConfig() MarketNodeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.config.MarketNode == nil {
		return MarketNodeConfig{}
	}
	return *s.config.MarketNode
}

// SetMarketNodeConfig 持久化 A 端节点配置。共享密钥不在此处，仅存于系统钥匙串。
func (s *Service) SetMarketNodeConfig(cfg MarketNodeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := cfg
	s.config.MarketNode = &copied
	return s.saveLocked()
}

// ListChannelsPublic 返回白名单 DTO 列表，供前端展示与配置导出使用（不含任何凭证）。
func (s *Service) ListChannelsPublic() ([]ChannelPublic, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ChannelPublic, 0, len(s.channels))
	for _, ch := range s.channels {
		result = append(result, ch.Public())
	}
	return result, nil
}

// ListChannels 保持原有 []Channel API 兼容；所有凭证字段均被白名单剔除（CL-S1）。
func (s *Service) ListChannels() ([]Channel, error) {
	public, err := s.ListChannelsPublic()
	if err != nil {
		return nil, err
	}
	result := make([]Channel, 0, len(public))
	for _, pub := range public {
		result = append(result, pub.toChannel())
	}
	return result, nil
}

// ListChannelsWithCredentials 返回带真实凭证的渠道副本，仅供进程内受控用途
// （健康检查、资产导出）使用。凭证优先取内存副本，缺失时从 keychain 读取，
// 绝不直接暴露给前端。
func (s *Service) ListChannelsWithCredentials() ([]Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listChannelsWithCredentialsLocked(), nil
}

func (s *Service) listChannelsWithCredentialsLocked() []Channel {
	store := s.credentialStore()
	result := make([]Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		resolved := ch
		if resolved.APIKey == "" {
			if v, err := store.GetChannelKey(ch.ChannelID); err == nil {
				resolved.APIKey = v
			}
		}
		if len(resolved.APIKeys) == 0 {
			if keys, err := store.GetChannelKeys(ch.ChannelID); err == nil && len(keys) > 0 {
				resolved.APIKeys = keys
			}
		}
		if resolved.OAuthRefreshToken == "" {
			if v, err := store.GetSecret(oauthRefreshKeyName(ch.ChannelID)); err == nil {
				resolved.OAuthRefreshToken = v
			}
		}
		if resolved.OAuthClientSecret == "" {
			if v, err := store.GetSecret(oauthClientSecretKeyName(ch.ChannelID)); err == nil {
				resolved.OAuthClientSecret = v
			}
		}
		result = append(result, resolved)
	}
	return result
}

// NormalizeChannelWeights validates and normalizes Weight and MaxConcurrent for
// all channels. Weight defaults to 1 when <= 0; negative values are rejected.
// MaxConcurrent stays 0 (unlimited) unless explicitly set positive.
// Returns a slice of warning strings for channels that were auto-fixed.
func (s *Service) NormalizeChannelWeights() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var warnings []string
	for i := range s.channels {
		if s.channels[i].Weight < 0 {
			warnings = append(warnings, fmt.Sprintf("channel %s: negative weight %d → 1", s.channels[i].ChannelID, s.channels[i].Weight))
			s.channels[i].Weight = 1
		} else if s.channels[i].Weight == 0 {
			s.channels[i].Weight = 1
		}
		if s.channels[i].MaxConcurrent < 0 {
			warnings = append(warnings, fmt.Sprintf("channel %s: negative max_concurrent %d → 0 (unlimited)", s.channels[i].ChannelID, s.channels[i].MaxConcurrent))
			s.channels[i].MaxConcurrent = 0
		}
	}
	if len(warnings) > 0 {
		_ = s.saveLocked()
	}
	return warnings
}

// ValidateChannelWeights checks whether all channels in a group have valid
// weights for WRR. Returns an error if any weight is negative.
func (s *Service) ValidateChannelWeights() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ch := range s.channels {
		if ch.Weight < 0 {
			return fmt.Errorf("channel %s has negative weight %d", ch.ChannelID, ch.Weight)
		}
	}
	return nil
}

func (s *Service) SaveCustomChannel(ch Channel) (string, error) {
	// 渠道地址会被用于携带 API Key 的出站请求，先校验再落盘，避免 SSRF/明文传输。
	if err := validateChannelBaseURL(ch.BaseURL); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	ch.ChannelType = "custom"
	if ch.ChannelID == "" {
		ch.ChannelID = generateChannelID()
	}
	if ch.HealthStatus == "" {
		ch.HealthStatus = "unknown"
	}
	if ch.ProviderSpecificConfig == nil {
		ch.ProviderSpecificConfig = map[string]string{}
	}
	// Normalize weight: default to 1 when unset
	if ch.Weight <= 0 {
		ch.Weight = 1
	}
	// Normalize max_concurrent: 0 means unlimited
	if ch.MaxConcurrent < 0 {
		ch.MaxConcurrent = 0
	}

	for i := range s.channels {
		if s.channels[i].ChannelID == ch.ChannelID {
			existingModels := s.channels[i].Models
			if len(ch.Models) == 0 {
				ch.Models = existingModels
			}
			// 前端拿到的是脱敏后的渠道（不含明文 key）。入参未携带 key 时保留内存
			// 中已有的 key，避免仅编辑其它字段就把凭证清空。
			if ch.APIKey == "" {
				ch.APIKey = s.channels[i].APIKey
			}
			if len(ch.APIKeys) == 0 {
				ch.APIKeys = s.channels[i].APIKeys
			}
			s.channels[i] = ch
			return ch.ChannelID, s.saveLocked()
		}
	}

	s.channels = append(s.channels, ch)
	return ch.ChannelID, s.saveLocked()
}

func (s *Service) DeleteCustomChannel(channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.channels[:0]
	for _, ch := range s.channels {
		if ch.ChannelID != channelID {
			filtered = append(filtered, ch)
		}
	}
	s.channels = filtered
	if s.config.DefaultChannelID == channelID {
		s.config.DefaultChannelID = ""
	}
	// 同步清理系统安全存储中的渠道凭证，避免残留可用的孤立密钥。
	_ = keychain.DeleteChannelKey(channelID)
	_ = keychain.DeleteChannelKeys(channelID)
	_ = keychain.DeleteSecret(oauthRefreshKeyName(channelID))
	_ = keychain.DeleteSecret(oauthClientSecretKeyName(channelID))
	return s.saveLocked()
}

func (s *Service) ReorderChannels(orderedIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idIndex := make(map[string]int, len(orderedIDs))
	for i, id := range orderedIDs {
		idIndex[id] = i
	}

	sorted := make([]Channel, 0, len(s.channels))
	unsorted := make([]Channel, 0)

	for _, ch := range s.channels {
		if _, ok := idIndex[ch.ChannelID]; ok {
			unsorted = append(unsorted, ch)
		} else {
			sorted = append(sorted, ch)
		}
	}

	for i := 0; i < len(unsorted)-1; i++ {
		for j := i + 1; j < len(unsorted); j++ {
			if idIndex[unsorted[i].ChannelID] > idIndex[unsorted[j].ChannelID] {
				unsorted[i], unsorted[j] = unsorted[j], unsorted[i]
			}
		}
	}

	s.channels = append(unsorted, sorted...)
	return s.saveLocked()
}

func (s *Service) SetAutoLaunch(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.AutoLaunch = enabled
	return s.saveLocked()
}

func (s *Service) SetAnalyticsEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.AnalyticsEnabled = enabled
	_ = s.saveLocked()
}

func (s *Service) UpdateRoutingSettings(strategy string, stickyUses int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		strategy = "fallback"
	}
	if stickyUses <= 0 {
		stickyUses = 2
	}
	s.config.RoutingStrategy = strategy
	s.config.StickyChannelUse = stickyUses
	return s.saveLocked()
}

func (s *Service) SaveModelCombo(combo ModelCombo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	combo.Name = strings.TrimSpace(combo.Name)
	if combo.Name == "" {
		return errors.New("combo name is required")
	}
	combo.Description = strings.TrimSpace(combo.Description)
	combo.Tags = normalizeTags(combo.Tags)
	combo.Strategy = strings.TrimSpace(combo.Strategy)
	if combo.Strategy == "" {
		combo.Strategy = "fallback"
	}
	if combo.StickyUses <= 0 {
		combo.StickyUses = 1
	}
	combo.Steps = normalizeComboSteps(combo.Steps)
	if len(combo.Steps) == 0 && len(combo.Models) > 0 {
		models := normalizeModels(combo.Models)
		combo.Steps = make([]ModelComboStep, 0, len(models))
		for _, model := range models {
			combo.Steps = append(combo.Steps, ModelComboStep{Model: model})
		}
	}
	combo.Models = comboModelsFromSteps(combo.Steps)
	if len(combo.Steps) == 0 {
		return errors.New("combo must contain at least one model")
	}

	for i := range s.config.ModelCombos {
		if s.config.ModelCombos[i].Name == combo.Name {
			s.config.ModelCombos[i] = combo
			return s.saveLocked()
		}
	}

	s.config.ModelCombos = append(s.config.ModelCombos, combo)
	return s.saveLocked()
}

func (s *Service) DeleteModelCombo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.config.ModelCombos[:0]
	for _, combo := range s.config.ModelCombos {
		if combo.Name != name {
			filtered = append(filtered, combo)
		}
	}
	s.config.ModelCombos = filtered
	return s.saveLocked()
}

var modelTierMap = map[string]int{
	"gpt-5":                    11,
	"gpt-4o":                   10,
	"gpt-4-turbo":              10,
	"gpt-4":                    10,
	"claude-4-opus":            12,
	"claude-4-sonnet":          11,
	"claude-opus-4-20250514":   10,
	"claude-opus-4-8":          12,
	"claude-3-opus":            10,
	"claude-fable-5":           13,
	"claude-sonnet-5":          11,
	"claude-haiku-4-5":         5,
	"o3":                       10,
	"claude-3-5-sonnet":        8,
	"o3-mini":                  9,
	"o1":                       9,
	"o1-mini":                  9,
	"deepseek-v4-pro":          9,
	"glm-5.1":                  9,
	"claude-sonnet-4-20250514": 8,
	"gemini-2.5-pro":           8,
	"kimi-2.6":                 9,
	"qwen-max":                 8,
	"mistral-large":            8,
	"mistral-medium-3.5":       8,
	"llama-3.1-405b":           8,
	"llama-4-maverick":         8,
	"llama-4-scout":            7,
	"gpt-5-mini":               8,
	"gpt-4o-mini":              5,
	"claude-3-5-haiku":         5,
	"gemini-2.5-flash":         5,
	"deepseek-v4-flash":        7,
	"glm-5":                    8,
	"kimi-2.5":                 8,
	"deepseek-chat":            7, // Deprecated — use deepseek-v4-pro after 2026-07-24
	"deepseek-reasoner":        7, // Deprecated — use deepseek-v4-flash (thinking) after 2026-07-24
	"deepseek-coder":           6,
	"qwen-plus":                7,
	"llama-3.1-70b":            7,
	"gpt-5-nano":               6,
	"gemini-2.0-flash":         7,
	"gemini-1.5-pro":           9,
	"gpt-3.5-turbo":            3,
	"claude-3-haiku":           3,
	"gemini-1.5-flash":         3,
	"minimax-m2.7":             7,
	"minimax-m3":               9,
}

var modelCostMap = map[string]float64{
	"gpt-5":                    20.0,
	"gpt-4-turbo":              10.0,
	"gpt-4o":                   5.0,
	"gpt-4":                    30.0,
	"claude-4-opus":            80.0,
	"claude-4-sonnet":          18.0,
	"claude-opus-4-20250514":   15.0,
	"claude-opus-4-8":          30.0,
	"claude-3-opus":            15.0,
	"claude-fable-5":           60.0,
	"claude-sonnet-5":          18.0,
	"claude-haiku-4-5":         1.0,
	"o3":                       10.0,
	"o3-mini":                  1.1,
	"o1":                       15.0,
	"o1-mini":                  1.1,
	"claude-sonnet-4-20250514": 3.0,
	"claude-3-5-sonnet":        3.0,
	"deepseek-v4-pro":          2.0,
	"gemini-2.5-pro":           1.25,
	"glm-5.1":                  3.0,
	"kimi-2.6":                 4.0,
	"deepseek-chat":            0.27, // Deprecated — use deepseek-v4-pro after 2026-07-24
	"deepseek-reasoner":        0.40, // Deprecated — use deepseek-v4-flash after 2026-07-24
	"deepseek-coder":           0.14,
	"deepseek-v4-flash":        0.40,
	"glm-5":                    2.0,
	"kimi-2.5":                 2.5,
	"gpt-5-mini":               1.2,
	"gpt-4o-mini":              0.15,
	"claude-3-5-haiku":         0.8,
	"gemini-2.5-flash":         0.075,
	"gpt-5-nano":               0.40,
	"gpt-3.5-turbo":            0.5,
	"claude-3-haiku":           0.25,
	"gemini-1.5-flash":         0.075,
	"minimax-m2.7":             1.5,
	"minimax-m3":               1.5,
	"mistral-medium-3.5":       9.0,
	"llama-4-maverick":         1.1,
	"llama-4-scout":            0.4,
}

func modelTier(model string) int {
	if tier, ok := modelTierMap[model]; ok {
		return tier
	}
	for k, v := range modelTierMap {
		if strings.HasPrefix(model, k) {
			return v
		}
	}
	return 5
}

// modelCost returns a single cost signal (USD per 1M output tokens) for the
// given model. The canonical price table lives in internal/usage so that cost
// accounting and routing price constraints can never diverge; the map below is
// only a legacy fallback for models not present in that table.
func modelCost(model string) float64 {
	if p, ok := usage.LookupPricing(model); ok {
		return p.OutputPricePer1M
	}
	if cost, ok := modelCostMap[model]; ok {
		return cost
	}
	best := 1.0
	bestLen := -1
	for k, v := range modelCostMap {
		if strings.HasPrefix(model, k) && len(k) > bestLen {
			best = v
			bestLen = len(k)
		}
	}
	return best
}

// ModelCost returns the estimated cost per 1K tokens for the given model.
func ModelCost(model string) float64 {
	return modelCost(model)
}

func (s *Service) ApplyComboSortPreset(comboName string, preset string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	preset = strings.ToLower(strings.TrimSpace(preset))
	if preset != "intelligence" && preset != "speed" && preset != "budget" {
		return fmt.Errorf("unknown sort preset: %s (valid: intelligence, speed, budget)", preset)
	}

	idx := -1
	for i, combo := range s.config.ModelCombos {
		if combo.Name == comboName {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("combo not found: %s", comboName)
	}

	steps := s.config.ModelCombos[idx].Steps
	if len(steps) <= 1 {
		return nil
	}

	switch preset {
	case "intelligence":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelTier(steps[i].Model) > modelTier(steps[j].Model)
		})
	case "speed":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelTier(steps[i].Model) < modelTier(steps[j].Model)
		})
	case "budget":
		sort.SliceStable(steps, func(i, j int) bool {
			return modelCost(steps[i].Model) < modelCost(steps[j].Model)
		})
	}

	s.config.ModelCombos[idx].Steps = steps
	s.config.ModelCombos[idx].Models = comboModelsFromSteps(steps)
	return s.saveLocked()
}

func (s *Service) ListModelCombos() []ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModelCombo, 0, len(s.config.ModelCombos))
	for _, combo := range s.config.ModelCombos {
		combo = normalizeComboForRead(combo)
		result = append(result, combo)
	}
	return result
}

func (s *Service) GetModelCombo(name string) (ModelCombo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, combo := range s.config.ModelCombos {
		if combo.Name == name {
			combo = normalizeComboForRead(combo)
			return combo, true
		}
	}
	return ModelCombo{}, false
}

// FindComboByModel finds a combo that contains the given model in its steps
func (s *Service) FindComboByModel(model string) *ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := range s.config.ModelCombos {
		for _, step := range s.config.ModelCombos[i].Steps {
			if step.Model == model {
				c := s.config.ModelCombos[i]
				c = normalizeComboForRead(c)
				return &c
			}
		}
		for _, m := range s.config.ModelCombos[i].Models {
			if m == model {
				c := s.config.ModelCombos[i]
				c = normalizeComboForRead(c)
				return &c
			}
		}
	}
	return nil
}

func (s *Service) SaveComboTemplate(template ModelCombo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	template.Name = strings.TrimSpace(template.Name)
	if template.Name == "" {
		return errors.New("template name is required")
	}
	template.Description = strings.TrimSpace(template.Description)
	template.Tags = normalizeTags(template.Tags)
	template.Strategy = strings.TrimSpace(template.Strategy)
	if template.Strategy == "" {
		template.Strategy = "fallback"
	}
	if template.StickyUses <= 0 {
		template.StickyUses = 1
	}
	template.Steps = normalizeComboSteps(template.Steps)
	if len(template.Steps) == 0 && len(template.Models) > 0 {
		models := normalizeModels(template.Models)
		template.Steps = make([]ModelComboStep, 0, len(models))
		for _, model := range models {
			template.Steps = append(template.Steps, ModelComboStep{Model: model})
		}
	}
	template.Models = comboModelsFromSteps(template.Steps)
	if len(template.Steps) == 0 {
		return errors.New("template must contain at least one step")
	}

	for i := range s.config.ComboTemplates {
		if s.config.ComboTemplates[i].Name == template.Name {
			s.config.ComboTemplates[i] = template
			return s.saveLocked()
		}
	}

	s.config.ComboTemplates = append(s.config.ComboTemplates, template)
	return s.saveLocked()
}

func (s *Service) DeleteComboTemplate(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.config.ComboTemplates[:0]
	for _, template := range s.config.ComboTemplates {
		if template.Name != name {
			filtered = append(filtered, template)
		}
	}
	s.config.ComboTemplates = filtered
	return s.saveLocked()
}

func (s *Service) RenameComboTemplate(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	if oldName == "" || newName == "" {
		return errors.New("template names are required")
	}
	for i := range s.config.ComboTemplates {
		if s.config.ComboTemplates[i].Name == oldName {
			s.config.ComboTemplates[i].Name = newName
			return s.saveLocked()
		}
	}
	return errors.New("template not found")
}

func (s *Service) ListComboTemplates() []ModelCombo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]ModelCombo, 0, len(s.config.ComboTemplates))
	for _, template := range s.config.ComboTemplates {
		result = append(result, normalizeComboForRead(template))
	}
	return result
}

func (s *Service) UpdateChannelHealth(channelID, health string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.channels {
		if s.channels[i].ChannelID == channelID {
			s.channels[i].HealthStatus = health
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Service) UpsertPlatformChannels(channels []Channel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 平台下发的渠道同样会携带凭证出站，逐条校验 BaseURL（空值跳过）。
	for _, ch := range channels {
		if err := validateChannelBaseURL(ch.BaseURL); err != nil {
			return fmt.Errorf("platform channel %s: %w", ch.ChannelID, err)
		}
	}

	customChannels := make([]Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		if ch.ChannelType != "platform" {
			customChannels = append(customChannels, ch)
		}
	}

	for _, ch := range channels {
		ch.ChannelType = "platform"
		if ch.HealthStatus == "" {
			ch.HealthStatus = "healthy"
		}
		customChannels = append(customChannels, ch)
	}
	s.channels = customChannels
	return s.saveLocked()
}

func (s *Service) ClearPlatformChannels() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.channels[:0]
	for _, ch := range s.channels {
		if ch.ChannelType != "platform" {
			filtered = append(filtered, ch)
		}
	}
	s.channels = filtered
	if s.config.DefaultChannelID != "" {
		found := false
		for _, ch := range s.channels {
			if ch.ChannelID == s.config.DefaultChannelID {
				found = true
				break
			}
		}
		if !found {
			s.config.DefaultChannelID = ""
		}
	}
	return s.saveLocked()
}

func (s *Service) GetChannel(channelID string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.channels {
		if ch.ChannelID == channelID {
			return ch, true
		}
	}
	return Channel{}, false
}

func (s *Service) FirstEnabledChannel(channelType string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.channels {
		if ch.Enabled && (channelType == "" || ch.ChannelType == channelType) {
			return ch, true
		}
	}
	return Channel{}, false
}

func (s *Service) ResolvePreferredChannel() (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.config.DefaultChannelID != "" {
		for _, ch := range s.channels {
			if ch.ChannelID == s.config.DefaultChannelID && ch.Enabled {
				return ch, true
			}
		}
	}

	for _, ch := range s.channels {
		if ch.ChannelType == "custom" && ch.Enabled {
			return ch, true
		}
	}
	for _, ch := range s.channels {
		if ch.Enabled {
			return ch, true
		}
	}
	return Channel{}, false
}

func (s *Service) ResolveChannelsForModel(model string) ([]Channel, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normalizedModel := strings.TrimSpace(model)
	if normalizedModel == "" {
		normalizedModel = strings.TrimSpace(s.config.DefaultModel)
	}

	exact := make([]Channel, 0)
	generic := make([]Channel, 0)
	var preferred *Channel

	for _, ch := range s.channels {
		if !ch.Enabled {
			continue
		}

		if s.config.DefaultChannelID != "" && ch.ChannelID == s.config.DefaultChannelID {
			copyChannel := ch
			preferred = &copyChannel
		}

		if supportsModel(ch, normalizedModel) {
			exact = append(exact, ch)
			continue
		}
		if normalizedModel != "" && len(ch.Models) == 0 {
			generic = append(generic, ch)
		}
	}

	ordered := make([]Channel, 0, len(exact)+len(generic))
	if preferred != nil && preferred.Enabled {
		if supportsModel(*preferred, normalizedModel) || (normalizedModel != "" && len(preferred.Models) == 0) {
			ordered = append(ordered, *preferred)
		}
	}

	appendUnique := func(items []Channel) {
		for _, candidate := range items {
			exists := false
			for _, current := range ordered {
				if current.ChannelID == candidate.ChannelID {
					exists = true
					break
				}
			}
			if !exists {
				ordered = append(ordered, candidate)
			}
		}
	}

	appendUnique(exact)
	appendUnique(generic)

	if len(ordered) == 0 && normalizedModel == "" {
		for _, ch := range s.channels {
			if ch.Enabled {
				ordered = append(ordered, ch)
			}
		}
	}

	return ordered, normalizedModel
}

func (s *Service) UpdateChannelModels(channelID string, models []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.channels {
		if s.channels[i].ChannelID == channelID {
			s.channels[i].Models = append([]string(nil), models...)
			if len(models) > 0 {
				s.channels[i].HealthStatus = "healthy"
			}
			return s.saveLocked()
		}
	}
	return nil
}

func (s *Service) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = state.Config
	s.channels = state.Channels
	s.restoreChannelSecretsLocked()
	return nil
}

// restoreChannelSecretsLocked 恢复渠道密钥到内存。config.json 不再持久化明文凭证，
// 单/多 API Key 与 OAuth 令牌统一存放在 keychain；同时兼容旧配置里残留的明文（迁移进 keychain）。
// 迁移写入失败只告警、不清空内存凭证，后续保存会重试。
// 调用方需持有 s.mu。
func (s *Service) restoreChannelSecretsLocked() {
	store := s.credentialStore()
	if s.savedSecretSig == nil {
		s.savedSecretSig = make(map[string]string)
	}
	for i := range s.channels {
		ch := &s.channels[i]
		migrationFailed := false
		if len(ch.APIKeys) > 0 {
			if err := store.SaveChannelKeys(ch.ChannelID, ch.APIKeys); err != nil {
				migrationFailed = true
				logging.Warningf("config: migrate channel keys for %s failed: %v", ch.ChannelID, err)
			}
		} else if keys, err := store.GetChannelKeys(ch.ChannelID); err == nil && len(keys) > 0 {
			ch.APIKeys = keys
		}

		// 仅配置了单 key 的旧渠道同样要迁移进安全存储：否则签名已含该 key，
		// 后续 saveLocked 会因签名一致而跳过写入，明文被清空后钥匙串却没有值。
		if ch.APIKey != "" {
			if err := store.SaveChannelKey(ch.ChannelID, ch.APIKey); err != nil {
				migrationFailed = true
				logging.Warningf("config: migrate channel key for %s failed: %v", ch.ChannelID, err)
			}
		}

		// OAuth 刷新令牌与客户端密钥同样是敏感凭证，迁移到系统安全存储。
		if token := strings.TrimSpace(ch.OAuthRefreshToken); token != "" {
			if err := store.SaveSecret(oauthRefreshKeyName(ch.ChannelID), token); err != nil {
				migrationFailed = true
				logging.Warningf("config: migrate oauth refresh token for %s failed: %v", ch.ChannelID, err)
			}
		} else if token, err := store.GetSecret(oauthRefreshKeyName(ch.ChannelID)); err == nil {
			ch.OAuthRefreshToken = token
		}
		if secret := strings.TrimSpace(ch.OAuthClientSecret); secret != "" {
			if err := store.SaveSecret(oauthClientSecretKeyName(ch.ChannelID), secret); err != nil {
				migrationFailed = true
				logging.Warningf("config: migrate oauth client secret for %s failed: %v", ch.ChannelID, err)
			}
		} else if secret, err := store.GetSecret(oauthClientSecretKeyName(ch.ChannelID)); err == nil {
			ch.OAuthClientSecret = secret
		}
		// 记录当前已落库指纹，避免后续 saveLocked 重复写钥匙串；迁移失败时不记录，
		// 让下一次 saveLocked 继续重试。
		if !migrationFailed {
			s.savedSecretSig[ch.ChannelID] = secretSignature(*ch)
		}
	}
}

func oauthRefreshKeyName(channelID string) string      { return "oauth_refresh_" + channelID }
func oauthClientSecretKeyName(channelID string) string { return "oauth_client_secret_" + channelID }

// secretSignature 是渠道凭证的内容指纹，用于跳过未变更的 keychain 写入。
func secretSignature(ch Channel) string {
	return strings.Join(ch.APIKeys, "\x00") + "\x01" + ch.APIKey + "\x01" +
		ch.OAuthRefreshToken + "\x01" + ch.OAuthClientSecret
}

// persistChannelSecretsLocked 把渠道凭证写入安全存储。任何写入失败都返回错误，
// 由调用方 fail-closed 处理（不落盘、不清空内存、保留待重试副本）。
func (s *Service) persistChannelSecretsLocked(store credentialStore, ch Channel) error {
	if len(ch.APIKeys) > 0 {
		if err := store.SaveChannelKeys(ch.ChannelID, ch.APIKeys); err != nil {
			return err
		}
		// 首个 key 同时写入单 key 槽位，兼容只读单 key 的调用路径。
		if ch.APIKey == "" && ch.APIKeys[0] != "" {
			if err := store.SaveChannelKey(ch.ChannelID, ch.APIKeys[0]); err != nil {
				return err
			}
		}
	}
	if ch.APIKey != "" {
		if err := store.SaveChannelKey(ch.ChannelID, ch.APIKey); err != nil {
			return err
		}
	}
	if ch.OAuthRefreshToken != "" {
		if err := store.SaveSecret(oauthRefreshKeyName(ch.ChannelID), ch.OAuthRefreshToken); err != nil {
			return err
		}
	}
	if ch.OAuthClientSecret != "" {
		if err := store.SaveSecret(oauthClientSecretKeyName(ch.ChannelID), ch.OAuthClientSecret); err != nil {
			return err
		}
	}
	return nil
}

// rememberPendingSecretsLocked 保留写入失败的凭证副本，等待下次保存重试。
func (s *Service) rememberPendingSecretsLocked(ch Channel) {
	if s.pendingSecrets == nil {
		s.pendingSecrets = make(map[string]pendingChannelSecrets)
	}
	s.pendingSecrets[ch.ChannelID] = pendingChannelSecrets{
		apiKey:            ch.APIKey,
		apiKeys:           append([]string(nil), ch.APIKeys...),
		oauthRefreshToken: ch.OAuthRefreshToken,
		oauthClientSecret: ch.OAuthClientSecret,
	}
}

// flushPendingSecretsLocked 重试此前失败的 keychain 写入；仍失败则返回错误。
func (s *Service) flushPendingSecretsLocked(store credentialStore) error {
	for channelID, pending := range s.pendingSecrets {
		ch := Channel{
			ChannelID:         channelID,
			APIKey:            pending.apiKey,
			APIKeys:           pending.apiKeys,
			OAuthRefreshToken: pending.oauthRefreshToken,
			OAuthClientSecret: pending.oauthClientSecret,
		}
		if err := s.persistChannelSecretsLocked(store, ch); err != nil {
			return err
		}
		delete(s.pendingSecrets, channelID)
	}
	return nil
}

func (s *Service) saveLocked() error {
	// 配置目录/文件包含渠道与令牌元数据，仅限当前用户访问。
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}

	store := s.credentialStore()
	if s.savedSecretSig == nil {
		s.savedSecretSig = make(map[string]string)
	}
	// 先重试此前失败的凭证写入，避免带着未落库的凭证继续走下去。
	if err := s.flushPendingSecretsLocked(store); err != nil {
		return fmt.Errorf("retry pending keychain writes: %w", err)
	}

	channels := make([]Channel, 0, len(s.channels))
	for i := range s.channels {
		ch := s.channels[i]
		// 明文凭证不落盘：单 key、多 key 与 OAuth 令牌都在 keychain 中保存。
		// 写入失败必须 fail-closed：返回错误、保留待重试副本，且不把凭证从内存里清掉。
		if secretSignature(ch) != s.savedSecretSig[ch.ChannelID] {
			if err := s.persistChannelSecretsLocked(store, ch); err != nil {
				s.rememberPendingSecretsLocked(ch)
				return fmt.Errorf("persist channel %s credentials to keychain: %w", ch.ChannelID, err)
			}
			s.savedSecretSig[ch.ChannelID] = secretSignature(ch)
		}
		ch.APIKey = ""
		ch.APIKeys = nil
		ch.OAuthRefreshToken = ""
		ch.OAuthClientSecret = ""
		channels = append(channels, ch)
	}

	state := persistedState{
		Config:   s.config,
		Channels: channels,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(s.path, data, 0o600); err != nil {
		return err
	}
	// 既有配置文件可能是旧版本以 0644 写入的，显式收紧权限。
	return os.Chmod(s.path, 0o600)
}

// writeFileAtomic 在同目录写临时文件后 rename，避免崩溃时留下半截配置；
// 临时文件与最终文件权限均固定为 perm。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *Service) UpdateSetting(key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch key {
	case "locale":
		if v, ok := value.(string); ok {
			s.config.Locale = v
		}
	case "auto_update":
		if v, ok := value.(bool); ok {
			s.config.AutoUpdate = v
		}
	case "analytics_enabled":
		if v, ok := value.(bool); ok {
			s.config.AnalyticsEnabled = v
		}
	case "log_retention_days":
		if v, ok := value.(int); ok {
			s.config.LogRetentionDays = v
		}
	}
	return s.saveLocked()
}

func (s *Service) GetSetting(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	switch key {
	case "locale":
		return s.config.Locale
	case "auto_update":
		if s.config.AutoUpdate {
			return "true"
		}
		return "false"
	case "analytics_enabled":
		if s.config.AnalyticsEnabled {
			return "true"
		}
		return "false"
	case "log_retention_days":
		return fmt.Sprintf("%d", s.config.LogRetentionDays)
	}
	return ""
}

// credentialStore 返回当前凭证存储实现；零值 Service（测试字面量）回退 keychain。
func (s *Service) credentialStore() credentialStore {
	if s.secrets != nil {
		return s.secrets
	}
	return keychainCredentialStore{}
}

// Sanitize 返回剔除全部凭证字段的渠道副本（单/多 API Key、OAuth refresh token、
// client secret 等敏感字段全部归零）。任何对外输出都必须经它构造（CL-S1）。
func (ch Channel) Sanitize() Channel {
	return ch.Public().toChannel()
}

func sanitizeChannel(ch Channel) Channel {
	// 凭证一律不外传：通过白名单 DTO 重建敏感字段全部归零（CL-S1）。
	return ch.Sanitize()
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "seasagi-config.json"
	}
	return filepath.Join(home, "Library", "Application Support", "SeasAGI", "config.json")
}

func generateChannelID() string {
	replacer := strings.NewReplacer(" ", "-", "_", "-")
	return "ch_" + replacer.Replace(strings.ToLower(time.Now().Format("20060102150405.000000")))
}

func supportsModel(ch Channel, model string) bool {
	if model == "" {
		return true
	}
	for _, candidate := range ch.Models {
		if candidate == model {
			return true
		}
	}
	return false
}

func normalizeModels(models []string) []string {
	result := make([]string, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result
}

func normalizeComboSteps(steps []ModelComboStep) []ModelComboStep {
	result := make([]ModelComboStep, 0, len(steps))
	seen := map[string]struct{}{}
	for _, step := range steps {
		step.ChannelID = strings.TrimSpace(step.ChannelID)
		step.Model = strings.TrimSpace(step.Model)
		if step.Model == "" {
			continue
		}
		key := step.ChannelID + "::" + step.Model
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, step)
	}
	return result
}

func comboModelsFromSteps(steps []ModelComboStep) []string {
	models := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Model != "" {
			models = append(models, step.Model)
		}
	}
	return models
}

func normalizeComboForRead(combo ModelCombo) ModelCombo {
	combo.Steps = normalizeComboSteps(combo.Steps)
	combo.Description = strings.TrimSpace(combo.Description)
	combo.Tags = normalizeTags(combo.Tags)
	if len(combo.Steps) == 0 && len(combo.Models) > 0 {
		models := normalizeModels(combo.Models)
		for _, model := range models {
			combo.Steps = append(combo.Steps, ModelComboStep{Model: model})
		}
	}
	combo.Models = comboModelsFromSteps(combo.Steps)
	return combo
}

func normalizeTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}
