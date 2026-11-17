package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsMarketGrantChannelID 判断渠道 ID 是否为 Token 市场 Grant 引用。
func IsMarketGrantChannelID(channelID string) bool {
	return strings.HasPrefix(channelID, MarketGrantChannelPrefix)
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

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "seasagi-config.json"
	}
	return filepath.Join(home, "Library", "Application Support", "SeasAGI", "config.json")
}
