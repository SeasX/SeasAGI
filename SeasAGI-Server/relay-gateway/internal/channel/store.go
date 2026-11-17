package channel

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
	"gopkg.in/yaml.v3"
)

type Channel struct {
	ChannelID    string            `json:"channel_id"`
	ProviderType string            `json:"provider_type"`
	DisplayName  string            `json:"display_name"`
	BaseURL      string            `json:"base_url"`
	APIKey       string            `json:"api_key"`
	Enabled      bool              `json:"enabled"`
	Models       []string          `json:"models,omitempty"`
	Weight       int               `json:"weight,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	GrayPercent  int               `json:"gray_percent,omitempty"`
	ReadOnly     bool              `json:"read_only,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
	Health       HealthStatus      `json:"health"`
	LastCheck    time.Time         `json:"last_check"`

	// Source 标记配置来源：SourceControlPlane 表示由 platform-api 同步下发并受其管理；
	// 空值表示本地/历史配置，同步不会删除它。
	Source string `json:"source,omitempty"`
	// ConfigVersion 是控制面对该渠道配置（含凭证）的指纹，用于判断是否需要覆盖本地状态。
	ConfigVersion string `json:"config_version,omitempty"`
	// CredentialError 非空表示凭证不可用（如密文无法解密），该渠道不参与路由。
	CredentialError string `json:"-"`

	// storedAPIKey 保存解密失败时的原始密文，落库时原样写回，避免误删凭证。
	storedAPIKey string
}

// SourceControlPlane 标记由控制面同步管理的渠道。
const SourceControlPlane = "control_plane"

// Routable 判断渠道当前能否承接请求：启用、非只读、凭证可用。
func (ch Channel) Routable() bool {
	return ch.Enabled && !ch.ReadOnly && ch.CredentialError == ""
}

type HealthStatus struct {
	Healthy    bool   `json:"healthy"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
	ConsecFail int    `json:"consec_fail"`
}

// PublicChannel 是对外投影：移除 APIKey 等上游凭证，供运维/列表接口使用。
type PublicChannel struct {
	ChannelID    string            `json:"channel_id"`
	ProviderType string            `json:"provider_type"`
	DisplayName  string            `json:"display_name"`
	BaseURL      string            `json:"base_url"`
	Enabled      bool              `json:"enabled"`
	Models       []string          `json:"models,omitempty"`
	Weight       int               `json:"weight,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	GrayPercent  int               `json:"gray_percent,omitempty"`
	ReadOnly     bool              `json:"read_only,omitempty"`
	Extra        map[string]string `json:"extra,omitempty"`
	Health       HealthStatus      `json:"health"`
	LastCheck    time.Time         `json:"last_check"`
	HasAPIKey    bool              `json:"has_api_key"`
	Source       string            `json:"source,omitempty"`
	// CredentialError 只描述错误类别，从不包含凭证内容。
	CredentialError string `json:"credential_error,omitempty"`
}

// Public 返回不含凭证的渠道视图。
func (ch Channel) Public() PublicChannel {
	return PublicChannel{
		ChannelID:    ch.ChannelID,
		ProviderType: ch.ProviderType,
		DisplayName:  ch.DisplayName,
		BaseURL:      ch.BaseURL,
		Enabled:      ch.Enabled,
		Models:       ch.Models,
		Weight:       ch.Weight,
		Priority:     ch.Priority,
		GrayPercent:  ch.GrayPercent,
		ReadOnly:     ch.ReadOnly,
		Extra:        ch.Extra,
		Health:       ch.Health,
		LastCheck:    ch.LastCheck,
		HasAPIKey:    ch.APIKey != "",
		Source:       ch.Source,

		CredentialError: ch.CredentialError,
	}
}

// PublicList 批量投影为对外视图。
func PublicList(in []Channel) []PublicChannel {
	out := make([]PublicChannel, 0, len(in))
	for _, ch := range in {
		out = append(out, ch.Public())
	}
	return out
}

type ModelMapping struct {
	LogicalModel  string `json:"logical_model"`
	ChannelID     string `json:"channel_id"`
	UpstreamModel string `json:"upstream_model"`
}

type Store struct {
	mu       sync.RWMutex
	channels map[string]*Channel
	mappings []ModelMapping
	dataDir  string
	filePath string // 历史 JSON 存储，仅用于一次性导入，不再写入
	dbPath   string
	db       *sql.DB
	cipher   credentialCipher

	// persistLoop 是唯一写库协程：saveCh 只做「有变更」标记，flushCh 用于同步落库。
	saveCh          chan struct{}
	flushCh         chan chan error
	stopCh          chan struct{}
	doneCh          chan struct{}
	closeOnce       sync.Once
	closeErr        error
	persistInterval time.Duration

	// 以下字段只由 persistLoop（以及启动前的 load）访问，无需加锁。
	persisted         map[string]string // channel_id -> 已落库行指纹
	persistedMappings string
	lastPersistErr    time.Time
}

// defaultPersistInterval 是内存状态合并落库的间隔。
const defaultPersistInterval = 500 * time.Millisecond

var (
	globalMu    sync.RWMutex
	globalStore *Store
)

// NewStore 在 ResolveDataDir 决定的目录下打开（或创建）relay 存储。
// 数据库打不开、历史数据无法读取时直接返回错误，而不是静默降级为空配置。
func NewStore() (*Store, error) {
	dir, err := ResolveDataDir()
	if err != nil {
		return nil, err
	}
	return newStoreAt(dir)
}

// newStoreAt 在指定目录初始化存储；测试可用临时目录隔离。
func newStoreAt(baseDir string) (*Store, error) {
	s := &Store{
		channels:        make(map[string]*Channel),
		mappings:        make([]ModelMapping, 0),
		dataDir:         baseDir,
		filePath:        filepath.Join(baseDir, "relay-channels.json"),
		dbPath:          filepath.Join(baseDir, "relay-gateway.db"),
		cipher:          newCredentialCipher(),
		saveCh:          make(chan struct{}, 1),
		flushCh:         make(chan chan error),
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
		persistInterval: defaultPersistInterval,
		persisted:       make(map[string]string),
	}
	if err := s.openDB(); err != nil {
		return nil, err
	}
	if err := s.load(); err != nil {
		_ = s.db.Close()
		return nil, err
	}
	go s.persistLoop()

	globalMu.Lock()
	globalStore = s
	globalMu.Unlock()
	return s, nil
}

func GetGlobalStore() *Store {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalStore
}

func (s *Store) DBPath() string {
	return s.dbPath
}

// DataDir 返回存储所在目录。
func (s *Store) DataDir() string {
	return s.dataDir
}

func (s *Store) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	enabled := 0
	for _, ch := range s.channels {
		if ch.Enabled {
			enabled++
		}
	}
	return map[string]interface{}{
		"channels_total":   len(s.channels),
		"channels_enabled": enabled,
		"mappings_total":   len(s.mappings),
	}
}

func (s *Store) AddChannel(ch Channel) {
	if ch.Weight <= 0 {
		ch.Weight = 100
	}
	s.mu.Lock()
	s.channels[ch.ChannelID] = &ch
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) RemoveChannel(channelID string) {
	s.mu.Lock()
	delete(s.channels, channelID)
	s.mu.Unlock()
	s.requestSave()
}

// GetChannel 返回渠道副本，避免调用方直接修改内部状态造成读写竞争。
func (s *Store) GetChannel(channelID string) (*Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch, ok := s.channels[channelID]
	if !ok {
		return nil, false
	}
	cp := *ch
	return &cp, true
}

func (s *Store) ListChannels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		result = append(result, *ch)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority == result[j].Priority {
			return result[i].ChannelID < result[j].ChannelID
		}
		return result[i].Priority < result[j].Priority
	})
	return result
}

func (s *Store) ListEnabledChannels() []Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Channel, 0)
	for _, ch := range s.channels {
		// 凭证不可用的渠道既不能路由也不应被健康检查（无凭证探测必然失败并被误判）。
		if ch.Enabled && ch.CredentialError == "" {
			result = append(result, *ch)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Priority == result[j].Priority {
			return result[i].Weight > result[j].Weight
		}
		return result[i].Priority < result[j].Priority
	})
	return result
}

func (s *Store) UpdateHealth(channelID string, health HealthStatus) {
	s.mu.Lock()
	if ch, ok := s.channels[channelID]; ok {
		ch.Health = health
		ch.LastCheck = time.Now()
		if health.ConsecFail >= 3 {
			ch.Enabled = false
		}
	}
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) RestoreChannel(channelID string) {
	s.mu.Lock()
	if ch, ok := s.channels[channelID]; ok {
		ch.Enabled = true
		ch.Health.ConsecFail = 0
		ch.Health.Error = ""
	}
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) UpdateChannelPolicy(channelID string, weight, priority, grayPercent int, enabled, readOnly *bool) {
	s.mu.Lock()
	if ch, ok := s.channels[channelID]; ok {
		if weight > 0 {
			ch.Weight = weight
		}
		if priority >= 0 {
			ch.Priority = priority
		}
		if grayPercent >= 0 {
			ch.GrayPercent = grayPercent
		}
		if enabled != nil {
			ch.Enabled = *enabled
		}
		if readOnly != nil {
			ch.ReadOnly = *readOnly
		}
	}
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) UpdateChannel(ch Channel) {
	s.mu.Lock()
	if _, ok := s.channels[ch.ChannelID]; ok {
		s.channels[ch.ChannelID] = &ch
	}
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) AddMapping(m ModelMapping) {
	s.mu.Lock()
	s.mappings = append(s.mappings, m)
	s.mu.Unlock()
	s.requestSave()
}

func (s *Store) ResolveChannel(logicalModel string) (*Channel, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, m := range s.mappings {
		if m.LogicalModel == logicalModel {
			if ch, ok := s.channels[m.ChannelID]; ok && ch.Routable() && ch.Health.Healthy {
				cp := *ch
				return &cp, m.UpstreamModel
			}
		}
	}

	candidates := make([]*Channel, 0)
	for _, ch := range s.channels {
		if ch.Routable() && ch.Health.Healthy {
			for _, model := range ch.Models {
				if model == logicalModel {
					candidates = append(candidates, ch)
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].Weight > candidates[j].Weight
		}
		return candidates[i].Priority < candidates[j].Priority
	})

	cp := *candidates[0]
	return &cp, logicalModel
}

type catalogModel struct {
	Name                string            `yaml:"name"`
	DisplayName         string            `yaml:"display_name"`
	Description         string            `yaml:"description"`
	Category            string            `yaml:"category"`
	Family              string            `yaml:"family"`
	Provider            string            `yaml:"provider"`
	Modality            string            `yaml:"modality"`
	ContextWindow       int64             `yaml:"context_window"`
	MaxOutputTokens     int64             `yaml:"max_output_tokens"`
	InputPriceUSDPer1M  float64           `yaml:"input_price_usd_per_1m"`
	OutputPriceUSDPer1M float64           `yaml:"output_price_usd_per_1m"`
	InputModalities     []string          `yaml:"input_modalities"`
	OutputModalities    []string          `yaml:"output_modalities"`
	Capabilities        []string          `yaml:"capabilities"`
	SupportedParameters []string          `yaml:"supported_parameters"`
	Metadata            map[string]string `yaml:"metadata"`
}

type catalogFile struct {
	Version int            `yaml:"version"`
	Models  []catalogModel `yaml:"models"`
}

func (s *Store) LoadModelCatalog() error {
	candidates := []string{
		"data/model-catalog.yaml",
		"../data/model-catalog.yaml",
		"../../data/model-catalog.yaml",
		"../../../data/model-catalog.yaml",
	}
	var catalogPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			catalogPath = p
			break
		}
	}
	if catalogPath == "" {
		return fmt.Errorf("model-catalog.yaml not found in candidate paths")
	}

	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return fmt.Errorf("failed to read model catalog: %w", err)
	}

	var cat catalogFile
	if err := yaml.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("failed to parse model catalog: %w", err)
	}

	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// 目录整体在一个事务内写入：任一行失败即回滚，不留下半份目录。
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin model catalog tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range cat.Models {
		caps, _ := json.Marshal(m.Capabilities)
		inMods, _ := json.Marshal(m.InputModalities)
		outMods, _ := json.Marshal(m.OutputModalities)
		params, _ := json.Marshal(m.SupportedParameters)
		meta, _ := json.Marshal(m.Metadata)
		if _, err := tx.Exec(`INSERT OR REPLACE INTO model_catalog
			(model_id, display_name, description, category, family, provider, modality,
			 context_window, max_output_tokens, input_price_usd_per_1m, output_price_usd_per_1m,
			 capabilities, input_modalities, output_modalities, supported_parameters, metadata)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.Name, m.DisplayName, m.Description, m.Category, m.Family, m.Provider, m.Modality,
			m.ContextWindow, m.MaxOutputTokens, m.InputPriceUSDPer1M, m.OutputPriceUSDPer1M,
			string(caps), string(inMods), string(outMods), string(params), string(meta)); err != nil {
			return fmt.Errorf("store model catalog entry %q: %w", m.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model catalog: %w", err)
	}

	logging.Infof("model catalog loaded: %d models from %s", len(cat.Models), catalogPath)
	return nil
}

func (s *Store) GetModelMetadata(modelID string) map[string]interface{} {
	if s.db == nil {
		return nil
	}
	var displayName, description, category, family, provider, modality string
	var contextWindow, maxOutputTokens int64
	var inputPrice, outputPrice float64
	var capsJSON, inModJSON, outModJSON, paramsJSON, metaJSON string
	err := s.db.QueryRow(`SELECT display_name, description, category, family, provider, modality,
		context_window, max_output_tokens, input_price_usd_per_1m, output_price_usd_per_1m,
		capabilities, input_modalities, output_modalities, supported_parameters, metadata
		FROM model_catalog WHERE model_id = ?`, modelID).Scan(
		&displayName, &description, &category, &family, &provider, &modality,
		&contextWindow, &maxOutputTokens, &inputPrice, &outputPrice,
		&capsJSON, &inModJSON, &outModJSON, &paramsJSON, &metaJSON)
	if err != nil {
		return nil
	}

	var capabilities, inputModalities, outputModalities, supportedParameters []string
	var metadata map[string]string
	json.Unmarshal([]byte(capsJSON), &capabilities)
	json.Unmarshal([]byte(inModJSON), &inputModalities)
	json.Unmarshal([]byte(outModJSON), &outputModalities)
	json.Unmarshal([]byte(paramsJSON), &supportedParameters)
	json.Unmarshal([]byte(metaJSON), &metadata)

	return map[string]interface{}{
		"display_name":            displayName,
		"description":             description,
		"category":                category,
		"family":                  family,
		"provider":                provider,
		"modality":                modality,
		"context_window":          contextWindow,
		"max_output_tokens":       maxOutputTokens,
		"input_price_usd_per_1m":  inputPrice,
		"output_price_usd_per_1m": outputPrice,
		"capabilities":            capabilities,
		"input_modalities":        inputModalities,
		"output_modalities":       outputModalities,
		"supported_parameters":    supportedParameters,
		"metadata":                metadata,
	}
}
