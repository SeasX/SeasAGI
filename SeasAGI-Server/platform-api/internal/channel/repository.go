package channel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound 表示渠道不存在。
var ErrNotFound = errors.New("channel not found")

// Repository 封装渠道相关表（channels / channel_models / channel_strategies）的全部 SQL。
// 多表写入一律在单个事务中完成，任一步失败整体回滚。
type Repository struct {
	// db 每次调用时取当前连接：备份还原会重新 Init 数据库并替换连接池，
	// 仓储不能在构造时把旧句柄缓存下来。
	db func() *sql.DB
}

// NewRepository 基于连接提供函数创建仓储；生产传入 database.Writer，测试可传入独立临时库。
func NewRepository(db func() *sql.DB) *Repository {
	return &Repository{db: db}
}

const channelSelect = `SELECT c.channel_id, c.channel_type, c.provider_type, c.display_name, c.base_url, c.enabled, c.sort_order,
 COALESCE(s.weight, 100), COALESCE(s.priority, 0), COALESCE(s.gray_percent, 0), COALESCE(s.read_only, 0),
 COALESCE(s.encrypted_api_key, ''), COALESCE(s.enabled, 1)
 FROM channels c
 LEFT JOIN channel_strategies s ON c.channel_id = s.channel_id`

type rowScanner interface {
	Scan(dest ...any) error
}

// scanChannel 读取一行渠道；strategyEnabled 是 channel_strategies.enabled（默认 1）。
func scanChannel(sc rowScanner) (Channel, bool, error) {
	var ch Channel
	var enabledInt, readOnlyInt, strategyEnabled int
	if err := sc.Scan(&ch.ChannelID, &ch.ChannelType, &ch.ProviderType, &ch.DisplayName, &ch.BaseURL, &enabledInt, &ch.SortOrder,
		&ch.Weight, &ch.Priority, &ch.GrayPercent, &readOnlyInt, &ch.EncryptedAPIKey, &strategyEnabled); err != nil {
		return Channel{}, false, err
	}
	ch.Enabled = enabledInt == 1
	ch.ReadOnly = readOnlyInt == 1
	return ch, strategyEnabled == 1, nil
}

// List 返回全部渠道（含模型），按优先级 / 排序字段排序。
func (r *Repository) List(ctx context.Context) ([]Channel, error) {
	channels, _, err := r.listWithStrategy(ctx)
	return channels, err
}

// listWithStrategy 额外返回每个渠道的策略级 enabled 开关（与 channels 下标一一对应）。
func (r *Repository) listWithStrategy(ctx context.Context) ([]Channel, []bool, error) {
	rows, err := r.db().QueryContext(ctx, channelSelect+` ORDER BY COALESCE(s.priority, c.sort_order), c.channel_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("list channels: %w", err)
	}
	channels := []Channel{}
	var strategy []bool
	for rows.Next() {
		ch, se, err := scanChannel(rows)
		if err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("list channels: %w", err)
		}
		channels = append(channels, ch)
		strategy = append(strategy, se)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, nil, fmt.Errorf("list channels: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("list channels: %w", err)
	}

	// 外层游标完全关闭后再查询模型，避免单写连接上的游标嵌套。
	for i := range channels {
		models, err := r.models(ctx, channels[i].ChannelID)
		if err != nil {
			return nil, nil, err
		}
		channels[i].Models = models
	}
	return channels, strategy, nil
}

// Get 返回单个渠道；不存在时返回 ErrNotFound。
func (r *Repository) Get(ctx context.Context, channelID string) (*Channel, error) {
	ch, _, err := scanChannel(r.db().QueryRowContext(ctx, channelSelect+` WHERE c.channel_id = ?`, channelID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get channel %s: %w", channelID, err)
	}
	models, err := r.models(ctx, channelID)
	if err != nil {
		return nil, err
	}
	ch.Models = models
	return &ch, nil
}

func (r *Repository) models(ctx context.Context, channelID string) ([]ModelInfo, error) {
	rows, err := r.db().QueryContext(ctx, `SELECT model_id, model_name, capability FROM channel_models WHERE channel_id = ? ORDER BY model_id`, channelID)
	if err != nil {
		return nil, fmt.Errorf("list models for %s: %w", channelID, err)
	}
	defer rows.Close()
	models := []ModelInfo{}
	for rows.Next() {
		var m ModelInfo
		if err := rows.Scan(&m.ModelID, &m.ModelName, &m.Capability); err != nil {
			return nil, fmt.Errorf("list models for %s: %w", channelID, err)
		}
		models = append(models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list models for %s: %w", channelID, err)
	}
	return models, nil
}

// ChannelRecord 是写入渠道时的完整字段集合（凭证已加密）。
type ChannelRecord struct {
	ChannelID    string
	ChannelType  string
	ProviderType string
	DisplayName  string
	BaseURL      string
	Enabled      bool
	SortOrder    int
}

// ErrConflict 表示渠道 ID 已存在。
var ErrConflict = errors.New("channel already exists")

// Create 在一个事务中写入渠道、可选的加密凭证与模型列表。
func (r *Repository) Create(ctx context.Context, rec ChannelRecord, encryptedKey string, models []ModelInfo) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM channels WHERE channel_id = ?`, rec.ChannelID).Scan(&exists)
		if err == nil {
			return ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check channel %s: %w", rec.ChannelID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO channels (channel_id, channel_type, provider_type, display_name, base_url, enabled, sort_order)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			rec.ChannelID, rec.ChannelType, rec.ProviderType, rec.DisplayName, rec.BaseURL, boolInt(rec.Enabled), rec.SortOrder,
		); err != nil {
			return fmt.Errorf("insert channel %s: %w", rec.ChannelID, err)
		}
		if encryptedKey != "" {
			if err := upsertKey(ctx, tx, rec.ChannelID, encryptedKey); err != nil {
				return err
			}
		}
		return insertModels(ctx, tx, rec.ChannelID, models)
	})
}

// Update 在一个事务中更新渠道字段；encryptedKey 为 nil 表示不改凭证，
// replaceModels 为 true 时用 models 整体替换模型列表。
func (r *Repository) Update(ctx context.Context, rec ChannelRecord, encryptedKey *string, replaceModels bool, models []ModelInfo) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE channels SET channel_type=?, provider_type=?, display_name=?, base_url=?, enabled=?, sort_order=?, updated_at=CURRENT_TIMESTAMP
			 WHERE channel_id=?`,
			rec.ChannelType, rec.ProviderType, rec.DisplayName, rec.BaseURL, boolInt(rec.Enabled), rec.SortOrder, rec.ChannelID,
		)
		if err != nil {
			return fmt.Errorf("update channel %s: %w", rec.ChannelID, err)
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return ErrNotFound
		}
		if encryptedKey != nil {
			if err := upsertKey(ctx, tx, rec.ChannelID, *encryptedKey); err != nil {
				return err
			}
		}
		if replaceModels {
			if _, err := tx.ExecContext(ctx, `DELETE FROM channel_models WHERE channel_id=?`, rec.ChannelID); err != nil {
				return fmt.Errorf("replace models for %s: %w", rec.ChannelID, err)
			}
			return insertModels(ctx, tx, rec.ChannelID, models)
		}
		return nil
	})
}

// Delete 在一个事务中删除渠道及其模型与策略（含加密凭证），返回是否存在。
// 删除策略行可避免同 ID 渠道重建时「复活」旧凭证。
func (r *Repository) Delete(ctx context.Context, channelID string) (bool, error) {
	found := false
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM channels WHERE channel_id=?`, channelID)
		if err != nil {
			return fmt.Errorf("delete channel %s: %w", channelID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete channel %s: %w", channelID, err)
		}
		if n == 0 {
			return nil
		}
		found = true
		for _, stmt := range []string{
			`DELETE FROM channel_models WHERE channel_id=?`,
			`DELETE FROM channel_strategies WHERE channel_id=?`,
		} {
			if _, err := tx.ExecContext(ctx, stmt, channelID); err != nil {
				return fmt.Errorf("delete channel %s: %w", channelID, err)
			}
		}
		return nil
	})
	return found, err
}

func (r *Repository) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := r.db().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin channel tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit channel tx: %w", err)
	}
	return nil
}

func upsertKey(ctx context.Context, tx *sql.Tx, channelID, encrypted string) error {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO channel_strategies (channel_id, encrypted_api_key, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(channel_id) DO UPDATE SET encrypted_api_key=excluded.encrypted_api_key, updated_at=CURRENT_TIMESTAMP`,
		channelID, encrypted,
	); err != nil {
		return fmt.Errorf("store credential for %s: %w", channelID, err)
	}
	return nil
}

func insertModels(ctx context.Context, tx *sql.Tx, channelID string, models []ModelInfo) error {
	for _, m := range models {
		capability := m.Capability
		if capability == "" {
			capability = "chat"
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO channel_models (channel_id, model_id, model_name, capability) VALUES (?, ?, ?, ?)`,
			channelID, m.ModelID, m.ModelName, capability,
		); err != nil {
			return fmt.Errorf("store model %s for %s: %w", m.ModelID, channelID, err)
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
