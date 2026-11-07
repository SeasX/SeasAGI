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
	filePath string
	dbPath   string
	db       *sql.DB

	// saveCh 只做「有变更」标记；persistLoop 是唯一写库协程，
	// 合并高频健康更新后批量落库，避免每请求全表重写与写冲突。
	saveCh          chan struct{}
	stopCh          chan struct{}
	doneCh          chan struct{}
	closeOnce       sync.Once
	persistInterval time.Duration
}

// defaultPersistInterval 是内存状态合并落库的间隔。
const defaultPersistInterval = 500 * time.Millisecond

var globalStore *Store

func NewStore() *Store {
	homeDir, _ := os.UserHomeDir()
	return newStoreAt(filepath.Join(homeDir, ".seasagi"))
}

// newStoreAt 在指定目录初始化存储；测试可用临时目录隔离。
func newStoreAt(baseDir string) *Store {
	s := &Store{
		channels:        make(map[string]*Channel),
		mappings:        make([]ModelMapping, 0),
		filePath:        filepath.Join(baseDir, "relay-channels.json"),
		dbPath:          filepath.Join(baseDir, "relay-gateway.db"),
		saveCh:          make(chan struct{}, 1),
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
		persistInterval: defaultPersistInterval,
	}
	s.openDB()
	s.load()
	go s.persistLoop()
	globalStore = s
	return s
}

func GetGlobalStore() *Store {
	return globalStore
}

func (s *Store) DBPath() string {
	return s.dbPath
}

func (s *Store) openDB() {
	_ = os.MkdirAll(filepath.Dir(s.dbPath), 0755)
	db, err := openSQLite(s.dbPath)
	if err != nil {
		return
	}
	s.db = db
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS relay_channels (
		channel_id TEXT PRIMARY KEY,
		provider_type TEXT NOT NULL,
		display_name TEXT NOT NULL,
		base_url TEXT NOT NULL,
		api_key TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		models_json TEXT NOT NULL DEFAULT '[]',
		weight INTEGER NOT NULL DEFAULT 100,
		priority INTEGER NOT NULL DEFAULT 0,
		gray_percent INTEGER NOT NULL DEFAULT 0,
		read_only INTEGER NOT NULL DEFAULT 0,
		extra_json TEXT NOT NULL DEFAULT '{}',
		healthy INTEGER NOT NULL DEFAULT 1,
		status_code INTEGER NOT NULL DEFAULT 0,
		error TEXT NOT NULL DEFAULT '',
		consec_fail INTEGER NOT NULL DEFAULT 0,
		last_check DATETIME
	)`)
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS relay_model_mappings (
		logical_model TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		upstream_model TEXT NOT NULL,
		PRIMARY KEY(logical_model, channel_id)
	)`)
	_, _ = s.db.Exec(`CREATE TABLE IF NOT EXISTS model_catalog (
		model_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL DEFAULT '',
		family TEXT NOT NULL DEFAULT '',
		provider TEXT NOT NULL DEFAULT '',
		modality TEXT NOT NULL DEFAULT 'chat',
		context_window INTEGER NOT NULL DEFAULT 0,
		max_output_tokens INTEGER NOT NULL DEFAULT 0,
		input_price_usd_per_1m REAL NOT NULL DEFAULT 0,
		output_price_usd_per_1m REAL NOT NULL DEFAULT 0,
		capabilities TEXT NOT NULL DEFAULT '[]',
		input_modalities TEXT NOT NULL DEFAULT '[]',
		output_modalities TEXT NOT NULL DEFAULT '[]',
		supported_parameters TEXT NOT NULL DEFAULT '[]',
		metadata TEXT NOT NULL DEFAULT '{}'
	)`)
}

func (s *Store) load() {
	if s.db != nil && s.loadFromDB() {
		return
	}
	s.loadFromDisk()
	if s.db != nil {
		s.saveToDB()
	}
}

func (s *Store) loadFromDisk() {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}
	var payload struct {
		Channels []Channel      `json:"channels"`
		Mappings []ModelMapping `json:"mappings"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	for i := range payload.Channels {
		if payload.Channels[i].Weight <= 0 {
			payload.Channels[i].Weight = 100
		}
		s.channels[payload.Channels[i].ChannelID] = &payload.Channels[i]
	}
	s.mappings = payload.Mappings
}

func (s *Store) loadFromDB() bool {
	rows, err := s.db.Query(`SELECT channel_id, provider_type, display_name, base_url, api_key, enabled, models_json, weight, priority, gray_percent, read_only, extra_json, healthy, status_code, error, consec_fail, last_check FROM relay_channels`)
	if err != nil {
		return false
	}
	defer rows.Close()

	loaded := false
	for rows.Next() {
		var ch Channel
		var enabledInt, readOnlyInt, healthyInt int
		var modelsJSON, extraJSON string
		if err := rows.Scan(&ch.ChannelID, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &ch.APIKey, &enabledInt, &modelsJSON, &ch.Weight, &ch.Priority, &ch.GrayPercent, &readOnlyInt, &extraJSON, &healthyInt, &ch.Health.StatusCode, &ch.Health.Error, &ch.Health.ConsecFail, &ch.LastCheck); err != nil {
			continue
		}
		ch.Enabled = enabledInt == 1
		ch.ReadOnly = readOnlyInt == 1
		ch.Health.Healthy = healthyInt == 1
		_ = json.Unmarshal([]byte(modelsJSON), &ch.Models)
		_ = json.Unmarshal([]byte(extraJSON), &ch.Extra)
		s.channels[ch.ChannelID] = &ch
		loaded = true
	}

	mappingRows, err := s.db.Query(`SELECT logical_model, channel_id, upstream_model FROM relay_model_mappings`)
	if err == nil {
		defer mappingRows.Close()
		for mappingRows.Next() {
			var m ModelMapping
			if mappingRows.Scan(&m.LogicalModel, &m.ChannelID, &m.UpstreamModel) == nil {
				s.mappings = append(s.mappings, m)
			}
		}
	}

	return loaded
}

func (s *Store) save() {
	s.saveToDisk()
	s.saveToDB()
}

// persistLoop 是唯一落库协程：把短时间内的多次变更合并成一次写入。
func (s *Store) persistLoop() {
	ticker := time.NewTicker(s.persistInterval)
	defer ticker.Stop()

	dirty := false
	for {
		select {
		case <-s.saveCh:
			dirty = true
		case <-ticker.C:
			if dirty {
				s.save()
				dirty = false
			}
		case <-s.stopCh:
			// 停止前把尚在通道中的变更标记合并进本次落库。
			select {
			case <-s.saveCh:
				dirty = true
			default:
			}
			if dirty {
				s.save()
			}
			close(s.doneCh)
			return
		}
	}
}

// requestSave 非阻塞地标记「需要落库」，由 persistLoop 合并处理。
func (s *Store) requestSave() {
	select {
	case s.saveCh <- struct{}{}:
	default:
	}
}

func (s *Store) saveToDisk() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels := make([]Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		channels = append(channels, *ch)
	}
	payload := struct {
		Channels []Channel      `json:"channels"`
		Mappings []ModelMapping `json:"mappings"`
	}{Channels: channels, Mappings: s.mappings}

	data, _ := json.MarshalIndent(payload, "", "  ")
	_ = os.MkdirAll(filepath.Dir(s.filePath), 0755)
	tmpFile := s.filePath + ".tmp"
	_ = os.WriteFile(tmpFile, data, 0644)
	_ = os.Rename(tmpFile, s.filePath)
}

func (s *Store) saveToDB() {
	if s.db == nil {
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()

	_, _ = tx.Exec(`DELETE FROM relay_channels`)
	_, _ = tx.Exec(`DELETE FROM relay_model_mappings`)
	for _, ch := range s.channels {
		modelsJSON, _ := json.Marshal(ch.Models)
		extraJSON, _ := json.Marshal(ch.Extra)
		enabledInt := 0
		if ch.Enabled {
			enabledInt = 1
		}
		readOnlyInt := 0
		if ch.ReadOnly {
			readOnlyInt = 1
		}
		healthyInt := 0
		if ch.Health.Healthy {
			healthyInt = 1
		}
		_, _ = tx.Exec(`INSERT INTO relay_channels (channel_id, provider_type, display_name, base_url, api_key, enabled, models_json, weight, priority, gray_percent, read_only, extra_json, healthy, status_code, error, consec_fail, last_check) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, ch.ChannelID, ch.ProviderType, ch.DisplayName, ch.BaseURL, ch.APIKey, enabledInt, string(modelsJSON), ch.Weight, ch.Priority, ch.GrayPercent, readOnlyInt, string(extraJSON), healthyInt, ch.Health.StatusCode, ch.Health.Error, ch.Health.ConsecFail, ch.LastCheck)
	}
	for _, m := range s.mappings {
		_, _ = tx.Exec(`INSERT INTO relay_model_mappings (logical_model, channel_id, upstream_model) VALUES (?, ?, ?)`, m.LogicalModel, m.ChannelID, m.UpstreamModel)
	}
	_ = tx.Commit()
}

// Close 停止落库协程并关闭数据库；重复调用安全。
func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
		if s.db != nil {
			err = s.db.Close()
		}
	})
	return err
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
		if ch.Enabled {
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
			if ch, ok := s.channels[m.ChannelID]; ok && ch.Enabled && ch.Health.Healthy && !ch.ReadOnly {
				cp := *ch
				return &cp, m.UpstreamModel
			}
		}
	}

	candidates := make([]*Channel, 0)
	for _, ch := range s.channels {
		if ch.Enabled && ch.Health.Healthy && !ch.ReadOnly {
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

	for _, m := range cat.Models {
		caps, _ := json.Marshal(m.Capabilities)
		inMods, _ := json.Marshal(m.InputModalities)
		outMods, _ := json.Marshal(m.OutputModalities)
		params, _ := json.Marshal(m.SupportedParameters)
		meta, _ := json.Marshal(m.Metadata)
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO model_catalog
			(model_id, display_name, description, category, family, provider, modality,
			 context_window, max_output_tokens, input_price_usd_per_1m, output_price_usd_per_1m,
			 capabilities, input_modalities, output_modalities, supported_parameters, metadata)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.Name, m.DisplayName, m.Description, m.Category, m.Family, m.Provider, m.Modality,
			m.ContextWindow, m.MaxOutputTokens, m.InputPriceUSDPer1M, m.OutputPriceUSDPer1M,
			string(caps), string(inMods), string(outMods), string(params), string(meta))
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
