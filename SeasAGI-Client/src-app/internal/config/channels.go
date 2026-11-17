package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Client/internal/keychain"
	"github.com/SeasAGI/SeasAGI-Client/internal/logging"
)

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

func oauthRefreshKeyName(channelID string) string { return "oauth_refresh_" + channelID }

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

// Sanitize 返回剔除全部凭证字段的渠道副本（单/多 API Key、OAuth refresh token、
// client secret 等敏感字段全部归零）。任何对外输出都必须经它构造（CL-S1）。
func (ch Channel) Sanitize() Channel {
	return ch.Public().toChannel()
}

func sanitizeChannel(ch Channel) Channel {
	// 凭证一律不外传：通过白名单 DTO 重建敏感字段全部归零（CL-S1）。
	return ch.Sanitize()
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
