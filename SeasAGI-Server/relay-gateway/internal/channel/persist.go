package channel

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

// persistErrLogInterval 限制持久化失败日志的频率，避免磁盘故障时刷屏。
const persistErrLogInterval = 30 * time.Second

// errStoreClosed 表示存储已关闭，不再接受同步落库请求。
var errStoreClosed = errors.New("channel store is closed")

func (s *Store) openDB() error {
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0o700); err != nil {
		return fmt.Errorf("create relay data dir: %w", err)
	}
	db, err := openSQLite(s.dbPath)
	if err != nil {
		return fmt.Errorf("open relay database %s: %w", s.dbPath, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("open relay database %s: %w", s.dbPath, err)
	}
	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

func migrateSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS relay_channels (
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
	)`,
		`CREATE TABLE IF NOT EXISTS relay_model_mappings (
		logical_model TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		upstream_model TEXT NOT NULL,
		PRIMARY KEY(logical_model, channel_id)
	)`,
		`CREATE TABLE IF NOT EXISTS model_catalog (
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
	)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("init relay schema: %w", err)
		}
	}

	// 增量列：老库没有时追加，已有时跳过；只加列、从不删改已有列。
	cols, err := tableColumns(db, "relay_channels")
	if err != nil {
		return err
	}
	for _, col := range []struct{ name, ddl string }{
		{"source", `ALTER TABLE relay_channels ADD COLUMN source TEXT NOT NULL DEFAULT ''`},
		{"config_version", `ALTER TABLE relay_channels ADD COLUMN config_version TEXT NOT NULL DEFAULT ''`},
	} {
		if cols[col.name] {
			continue
		}
		if _, err := db.Exec(col.ddl); err != nil {
			return fmt.Errorf("add relay_channels.%s: %w", col.name, err)
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", table, err)
	}
	defer rows.Close()
	cols := make(map[string]bool)
	for rows.Next() {
		var (
			cid     int
			name    string
			typ     string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("inspect %s: %w", table, err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect %s: %w", table, err)
	}
	return cols, nil
}

// load 读取库中状态；库为空时一次性导入历史 JSON。历史明文凭证在密钥可用时
// 于启动阶段同步加密落库，失败则启动失败（数据保持原样，不会半写）。
func (s *Store) load() error {
	legacyPlain, err := s.loadFromDB()
	if err != nil {
		return err
	}

	s.mu.RLock()
	empty := len(s.channels) == 0 && len(s.mappings) == 0
	s.mu.RUnlock()
	if empty {
		imported, err := s.importLegacyJSON()
		if err != nil {
			// 历史 JSON 损坏时保留原文件不动，记录错误后以空配置启动（控制面同步会补齐）。
			logging.Errorf("relay legacy channel file %s could not be imported (left untouched): %v", s.filePath, err)
		}
		legacyPlain += imported
	}

	keyErr := s.cipher.available()
	if keyErr != nil {
		logging.Warningf("relay credential encryption key unavailable (%v): upstream API keys are kept in memory only and never written to disk", keyErr)
		if legacyPlain > 0 {
			logging.Warningf("relay found %d plaintext credential(s) from a previous version; they stay as-is until RELAY_DATA_KEY or SEASAGI_DATA_KEY is configured", legacyPlain)
		}
	}

	// 把导入的 JSON 与历史明文立即加密落库；未变化的行不会被重写。
	if err := s.saveToDB(); err != nil {
		return fmt.Errorf("persist relay state during startup: %w", err)
	}
	if legacyPlain > 0 && keyErr == nil {
		logging.Infof("relay migrated %d plaintext credential(s) to encrypted storage", legacyPlain)
	}
	return nil
}

// loadFromDB 读取全部渠道与映射，返回仍为明文存储的凭证数量（待迁移）。
func (s *Store) loadFromDB() (int, error) {
	rows, err := s.db.Query(`SELECT channel_id, provider_type, display_name, base_url, api_key, enabled, models_json, weight, priority, gray_percent, read_only, extra_json, healthy, status_code, error, consec_fail, last_check, source, config_version FROM relay_channels`)
	if err != nil {
		return 0, fmt.Errorf("load relay channels: %w", err)
	}
	defer rows.Close()

	legacyPlain := 0
	loaded := make(map[string]*Channel)
	fingerprints := make(map[string]string)
	for rows.Next() {
		var ch Channel
		var enabledInt, readOnlyInt, healthyInt int
		var modelsJSON, extraJSON, storedKey string
		var lastCheck sql.NullTime
		if err := rows.Scan(&ch.ChannelID, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &storedKey, &enabledInt, &modelsJSON, &ch.Weight, &ch.Priority, &ch.GrayPercent, &readOnlyInt, &extraJSON, &healthyInt, &ch.Health.StatusCode, &ch.Health.Error, &ch.Health.ConsecFail, &lastCheck, &ch.Source, &ch.ConfigVersion); err != nil {
			return 0, fmt.Errorf("load relay channels: %w", err)
		}
		ch.Enabled = enabledInt == 1
		ch.ReadOnly = readOnlyInt == 1
		ch.Health.Healthy = healthyInt == 1
		if lastCheck.Valid {
			ch.LastCheck = lastCheck.Time
		}
		if err := json.Unmarshal([]byte(modelsJSON), &ch.Models); err != nil {
			logging.Warningf("relay channel %s has malformed models_json; loading without models: %v", ch.ChannelID, err)
		}
		if err := json.Unmarshal([]byte(extraJSON), &ch.Extra); err != nil {
			logging.Warningf("relay channel %s has malformed extra_json; loading without extras: %v", ch.ChannelID, err)
		}

		plain, legacy, decErr := s.cipher.unseal(storedKey)
		switch {
		case decErr != nil:
			// 解密失败：不路由该渠道，并原样保留密文，修正密钥后重启即可恢复。
			ch.APIKey = ""
			ch.storedAPIKey = storedKey
			ch.CredentialError = credentialDecryptError
			logging.Errorf("relay channel %s: %s", ch.ChannelID, credentialDecryptError)
		case legacy:
			ch.APIKey = plain
			legacyPlain++
		default:
			ch.APIKey = plain
		}

		c := ch
		loaded[ch.ChannelID] = &c
		// 历史明文行不记录指纹，以便下一次落库把它改写为密文。
		if !legacy {
			fingerprints[ch.ChannelID] = rowFor(c).fingerprint()
		}
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("load relay channels: %w", err)
	}

	mappings, err := s.loadMappings()
	if err != nil {
		return 0, err
	}

	s.mu.Lock()
	s.channels = loaded
	s.mappings = mappings
	s.mu.Unlock()
	s.persisted = fingerprints
	s.persistedMappings = mappingsFingerprint(mappings)
	return legacyPlain, nil
}

func (s *Store) loadMappings() ([]ModelMapping, error) {
	rows, err := s.db.Query(`SELECT logical_model, channel_id, upstream_model FROM relay_model_mappings ORDER BY rowid`)
	if err != nil {
		return nil, fmt.Errorf("load relay model mappings: %w", err)
	}
	defer rows.Close()
	mappings := make([]ModelMapping, 0)
	for rows.Next() {
		var m ModelMapping
		if err := rows.Scan(&m.LogicalModel, &m.ChannelID, &m.UpstreamModel); err != nil {
			return nil, fmt.Errorf("load relay model mappings: %w", err)
		}
		mappings = append(mappings, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load relay model mappings: %w", err)
	}
	return mappings, nil
}

// importLegacyJSON 在数据库为空时导入升级前的 relay-channels.json。
// 文件本身不会被修改或删除；导入后不再写入该文件（它包含明文凭证）。
func (s *Store) importLegacyJSON() (int, error) {
	data, err := os.ReadFile(s.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var payload struct {
		Channels []Channel      `json:"channels"`
		Mappings []ModelMapping `json:"mappings"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, err
	}

	plain := 0
	s.mu.Lock()
	for i := range payload.Channels {
		ch := payload.Channels[i]
		if ch.ChannelID == "" {
			continue
		}
		if ch.Weight <= 0 {
			ch.Weight = 100
		}
		if ch.APIKey != "" {
			plain++
		}
		s.channels[ch.ChannelID] = &ch
	}
	s.mappings = append(s.mappings, payload.Mappings...)
	s.mu.Unlock()

	logging.Warningf("relay imported %d channel(s) from legacy %s; the file is no longer written and still contains plaintext credentials — delete it after verifying the migration", len(payload.Channels), s.filePath)
	return plain, nil
}

// persistLoop 是唯一落库协程：把短时间内的多次变更合并成一次写入；
// 写入失败时保留 dirty 标记，下一个周期重试，不丢变更。
func (s *Store) persistLoop() {
	ticker := time.NewTicker(s.persistInterval)
	defer ticker.Stop()

	dirty := false
	for {
		select {
		case <-s.saveCh:
			dirty = true
		case reply := <-s.flushCh:
			err := s.saveToDB()
			if err == nil {
				dirty = false
			} else {
				dirty = true
			}
			reply <- err
		case <-ticker.C:
			if !dirty {
				continue
			}
			if err := s.saveToDB(); err != nil {
				s.reportPersistErr(err)
				continue
			}
			dirty = false
		case <-s.stopCh:
			// 停止前做最后一次落库（无变更时为空操作），错误由 Close 返回。
			select {
			case <-s.saveCh:
			default:
			}
			s.closeErr = s.saveToDB()
			close(s.doneCh)
			return
		}
	}
}

func (s *Store) reportPersistErr(err error) {
	now := time.Now()
	if now.Sub(s.lastPersistErr) < persistErrLogInterval {
		return
	}
	s.lastPersistErr = now
	logging.Errorf("relay channel store persist failed, will retry: %v", err)
}

// requestSave 非阻塞地标记「需要落库」，由 persistLoop 合并处理。
func (s *Store) requestSave() {
	select {
	case s.saveCh <- struct{}{}:
	default:
	}
}

// Flush 同步落库当前内存状态，并返回落库结果。
func (s *Store) Flush() error {
	reply := make(chan error, 1)
	select {
	case s.flushCh <- reply:
		return <-reply
	case <-s.doneCh:
		return errStoreClosed
	}
}

// Close 停止落库协程（完成最后一次落库）并关闭数据库；重复调用安全。
func (s *Store) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
		err = errors.Join(s.closeErr, s.db.Close())
	})
	return err
}

// channelRow 是一行 relay_channels 的待写入内容（凭证为明文，仅在内存中使用）。
type channelRow struct {
	ch         Channel
	modelsJSON string
	extraJSON  string
}

func rowFor(ch Channel) channelRow {
	models := ch.Models
	if models == nil {
		models = []string{}
	}
	extra := ch.Extra
	if extra == nil {
		extra = map[string]string{}
	}
	modelsJSON, _ := json.Marshal(models)
	extraJSON, _ := json.Marshal(extra)
	return channelRow{ch: ch, modelsJSON: string(modelsJSON), extraJSON: string(extraJSON)}
}

// fingerprint 覆盖一行的全部持久化内容（含凭证），只用于判断该行是否需要重写；
// 结果是哈希值，不可逆推出凭证，且只存在于进程内存中。
func (r channelRow) fingerprint() string {
	ch := r.ch
	h := sha256.New()
	for _, part := range []string{
		ch.ChannelID, ch.ProviderType, ch.DisplayName, ch.BaseURL, ch.APIKey, ch.storedAPIKey,
		strconv.FormatBool(ch.Enabled), r.modelsJSON, strconv.Itoa(ch.Weight), strconv.Itoa(ch.Priority),
		strconv.Itoa(ch.GrayPercent), strconv.FormatBool(ch.ReadOnly), r.extraJSON,
		strconv.FormatBool(ch.Health.Healthy), strconv.Itoa(ch.Health.StatusCode), ch.Health.Error,
		strconv.Itoa(ch.Health.ConsecFail), ch.LastCheck.UTC().Format(time.RFC3339Nano), ch.Source, ch.ConfigVersion,
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func mappingsFingerprint(mappings []ModelMapping) string {
	h := sha256.New()
	for _, m := range mappings {
		h.Write([]byte(m.LogicalModel + "\x00" + m.ChannelID + "\x00" + m.UpstreamModel + "\x01"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

const upsertChannelSQL = `INSERT INTO relay_channels (channel_id, provider_type, display_name, base_url, api_key, enabled, models_json, weight, priority, gray_percent, read_only, extra_json, healthy, status_code, error, consec_fail, last_check, source, config_version)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id) DO UPDATE SET provider_type=excluded.provider_type, display_name=excluded.display_name, base_url=excluded.base_url,
%s enabled=excluded.enabled, models_json=excluded.models_json, weight=excluded.weight, priority=excluded.priority,
gray_percent=excluded.gray_percent, read_only=excluded.read_only, extra_json=excluded.extra_json, healthy=excluded.healthy,
status_code=excluded.status_code, error=excluded.error, consec_fail=excluded.consec_fail, last_check=excluded.last_check,
source=excluded.source, config_version=excluded.config_version`

var (
	upsertWithKeySQL    = fmt.Sprintf(upsertChannelSQL, "api_key=excluded.api_key,")
	upsertKeepingKeySQL = fmt.Sprintf(upsertChannelSQL, "")
)

// storedCredential 决定一行要写入的 api_key 列值。keepExisting=true 表示更新时
// 不触碰已有 api_key 列（无加密密钥时绝不写入明文）。
func (s *Store) storedCredential(ch Channel, keyAvailable bool) (value string, keepExisting bool, err error) {
	switch {
	case ch.storedAPIKey != "" && ch.APIKey == "":
		return ch.storedAPIKey, false, nil
	case ch.APIKey == "":
		return "", false, nil
	case keyAvailable:
		sealed, err := s.cipher.seal(ch.APIKey)
		if err != nil {
			return "", false, fmt.Errorf("encrypt credential for channel %s: %w", ch.ChannelID, err)
		}
		return sealed, false, nil
	default:
		return "", true, nil
	}
}

// saveToDB 以单个事务增量落库：只写指纹变化的行、只删已移除的行，任一步失败即
// 回滚并返回错误，库中保持上一次成功提交的完整状态。
func (s *Store) saveToDB() error {
	s.mu.RLock()
	rows := make(map[string]channelRow, len(s.channels))
	for id, ch := range s.channels {
		rows[id] = rowFor(*ch)
	}
	mappings := append([]ModelMapping(nil), s.mappings...)
	s.mu.RUnlock()

	type pending struct {
		row channelRow
		fp  string
	}
	var upserts []pending
	for id, r := range rows {
		if fp := r.fingerprint(); s.persisted[id] != fp {
			upserts = append(upserts, pending{row: r, fp: fp})
		}
	}
	var deletes []string
	for id := range s.persisted {
		if _, ok := rows[id]; !ok {
			deletes = append(deletes, id)
		}
	}
	mfp := mappingsFingerprint(mappings)
	mappingsChanged := mfp != s.persistedMappings
	if len(upserts) == 0 && len(deletes) == 0 && !mappingsChanged {
		return nil
	}
	sort.Slice(upserts, func(i, j int) bool { return upserts[i].row.ch.ChannelID < upserts[j].row.ch.ChannelID })
	sort.Strings(deletes)

	keyAvailable := s.cipher.available() == nil

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin relay persist tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, p := range upserts {
		ch := p.row.ch
		apiKey, keepExisting, err := s.storedCredential(ch, keyAvailable)
		if err != nil {
			return err
		}
		query := upsertWithKeySQL
		if keepExisting {
			query = upsertKeepingKeySQL
		}
		var lastCheck any
		if !ch.LastCheck.IsZero() {
			lastCheck = ch.LastCheck.UTC()
		}
		if _, err := tx.Exec(query,
			ch.ChannelID, ch.ProviderType, ch.DisplayName, ch.BaseURL, apiKey, boolInt(ch.Enabled), p.row.modelsJSON,
			ch.Weight, ch.Priority, ch.GrayPercent, boolInt(ch.ReadOnly), p.row.extraJSON, boolInt(ch.Health.Healthy),
			ch.Health.StatusCode, ch.Health.Error, ch.Health.ConsecFail, lastCheck, ch.Source, ch.ConfigVersion,
		); err != nil {
			return fmt.Errorf("persist relay channel %s: %w", ch.ChannelID, err)
		}
	}
	for _, id := range deletes {
		if _, err := tx.Exec(`DELETE FROM relay_channels WHERE channel_id = ?`, id); err != nil {
			return fmt.Errorf("delete relay channel %s: %w", id, err)
		}
	}
	if mappingsChanged {
		if _, err := tx.Exec(`DELETE FROM relay_model_mappings`); err != nil {
			return fmt.Errorf("replace relay model mappings: %w", err)
		}
		for _, m := range mappings {
			if _, err := tx.Exec(`INSERT OR REPLACE INTO relay_model_mappings (logical_model, channel_id, upstream_model) VALUES (?, ?, ?)`,
				m.LogicalModel, m.ChannelID, m.UpstreamModel); err != nil {
				return fmt.Errorf("persist relay model mapping %s/%s: %w", m.LogicalModel, m.ChannelID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit relay persist tx: %w", err)
	}
	committed = true

	for _, p := range upserts {
		s.persisted[p.row.ch.ChannelID] = p.fp
	}
	for _, id := range deletes {
		delete(s.persisted, id)
	}
	s.persistedMappings = mfp
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
