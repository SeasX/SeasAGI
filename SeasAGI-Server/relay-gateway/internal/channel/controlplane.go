package channel

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// ControlPlaneChannel 是控制面下发的单个渠道配置（含明文凭证，仅在内存中流转）。
type ControlPlaneChannel struct {
	ChannelID    string
	ProviderType string
	DisplayName  string
	BaseURL      string
	APIKey       string
	Enabled      bool
	Models       []string
	Weight       int
	Priority     int
	GrayPercent  int
	ReadOnly     bool
	// CredentialError 非空表示控制面自身无法提供凭证（如解密失败），该渠道不参与路由。
	CredentialError string
}

// Version 返回该渠道配置的指纹（含凭证），用于判断控制面是否改动过它。
func (c ControlPlaneChannel) Version() string {
	models := append([]string(nil), c.Models...)
	sort.Strings(models)
	h := sha256.New()
	for _, part := range []string{
		c.ChannelID, c.ProviderType, c.DisplayName, c.BaseURL, c.APIKey, strconv.FormatBool(c.Enabled),
		strings.Join(models, "\x02"), strconv.Itoa(c.Weight), strconv.Itoa(c.Priority),
		strconv.Itoa(c.GrayPercent), strconv.FormatBool(c.ReadOnly), c.CredentialError,
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ApplyResult 统计一次控制面快照应用的结果。
type ApplyResult struct {
	Added     int
	Updated   int
	Removed   int
	Unchanged int
}

// Changed 表示本次应用是否改动了路由配置。
func (r ApplyResult) Changed() bool {
	return r.Added+r.Updated+r.Removed > 0
}

// ApplyControlPlane 用控制面快照更新渠道表，规则如下：
//   - 快照中的渠道成为受管渠道（Source=control_plane）；
//   - 控制面配置未变化（版本相同）的受管渠道保持原样，保留本地运维调整与健康状态；
//   - 控制面配置变化或新增的渠道采用控制面配置，并重置健康状态以便立即重新探测；
//   - 不在快照中的受管渠道被移除；本地（非受管）渠道和模型映射不受影响。
//
// 调用方必须只传入完整、校验通过的快照：空快照会移除全部受管渠道。
func (s *Store) ApplyControlPlane(snapshot []ControlPlaneChannel) ApplyResult {
	var res ApplyResult
	seen := make(map[string]bool, len(snapshot))

	s.mu.Lock()
	for _, in := range snapshot {
		if in.ChannelID == "" {
			continue
		}
		seen[in.ChannelID] = true
		version := in.Version()

		existing, ok := s.channels[in.ChannelID]
		if ok && existing.Source == SourceControlPlane && existing.ConfigVersion == version {
			res.Unchanged++
			continue
		}

		weight := in.Weight
		if weight <= 0 {
			weight = 100
		}
		next := &Channel{
			ChannelID:       in.ChannelID,
			ProviderType:    in.ProviderType,
			DisplayName:     in.DisplayName,
			BaseURL:         in.BaseURL,
			APIKey:          in.APIKey,
			Enabled:         in.Enabled,
			Models:          append([]string(nil), in.Models...),
			Weight:          weight,
			Priority:        in.Priority,
			GrayPercent:     in.GrayPercent,
			ReadOnly:        in.ReadOnly,
			Health:          HealthStatus{Healthy: true},
			Source:          SourceControlPlane,
			ConfigVersion:   version,
			CredentialError: in.CredentialError,
		}
		if ok {
			// 保留本地扩展字段（如 Azure api-version 等），控制面目前不下发 Extra。
			next.Extra = existing.Extra
			res.Updated++
		} else {
			res.Added++
		}
		s.channels[in.ChannelID] = next
	}
	for id, ch := range s.channels {
		if ch.Source == SourceControlPlane && !seen[id] {
			delete(s.channels, id)
			res.Removed++
		}
	}
	s.mu.Unlock()

	if res.Changed() {
		s.requestSave()
	}
	return res
}
