package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/SeasAGI/SeasAGI-Client/internal/urlguard"
)

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
