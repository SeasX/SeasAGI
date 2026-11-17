package relay

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotFound 表示中继网关不存在。
var ErrNotFound = errors.New("relay gateway not found")

// Repository 封装 relay_gateways 表的全部 SQL（表结构不变）。
type Repository struct {
	// db 每次调用时取当前连接：备份还原会重新 Init 数据库并替换连接池。
	db func() *sql.DB
}

// NewRepository 基于连接提供函数创建仓储；生产传入 database.Writer。
func NewRepository(db func() *sql.DB) *Repository {
	return &Repository{db: db}
}

const gatewayColumns = `gateway_id, name, host, port, region, enabled, created_at`

func scanGateway(sc interface{ Scan(...any) error }) (RelayGateway, error) {
	var g RelayGateway
	err := sc.Scan(&g.GatewayID, &g.Name, &g.Host, &g.Port, &g.Region, &g.Enabled, &g.CreatedAt)
	return g, err
}

// List 返回中继网关；enabledOnly 时只返回已启用且地址有效（host 非空、port>0）的记录。
// 按 region、name 排序；无法解析的行被跳过（与重构前行为一致）。
func (r *Repository) List(ctx context.Context, enabledOnly bool) ([]RelayGateway, error) {
	query := `SELECT ` + gatewayColumns + ` FROM relay_gateways`
	if enabledOnly {
		query += ` WHERE enabled = 1 AND TRIM(host) <> '' AND port > 0`
	}
	query += ` ORDER BY region, name`

	rows, err := r.db().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelayGateway, 0)
	for rows.Next() {
		if g, err := scanGateway(rows); err == nil {
			items = append(items, g)
		}
	}
	return items, rows.Err()
}

// Get 返回单个网关；不存在时返回 ErrNotFound。
func (r *Repository) Get(ctx context.Context, gatewayID string) (*RelayGateway, error) {
	g, err := scanGateway(r.db().QueryRowContext(ctx, `SELECT `+gatewayColumns+` FROM relay_gateways WHERE gateway_id = ?`, gatewayID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get relay gateway %s: %w", gatewayID, err)
	}
	return &g, nil
}

// Create 新增一个启用状态的网关。
func (r *Repository) Create(ctx context.Context, g RelayGateway) error {
	_, err := r.db().ExecContext(ctx,
		`INSERT INTO relay_gateways (gateway_id, name, host, port, region, enabled) VALUES (?, ?, ?, ?, ?, 1)`,
		g.GatewayID, g.Name, g.Host, g.Port, g.Region,
	)
	return err
}

// Update 覆盖网关的可编辑字段。
func (r *Repository) Update(ctx context.Context, g RelayGateway) error {
	_, err := r.db().ExecContext(ctx,
		`UPDATE relay_gateways SET name=?, host=?, port=?, region=?, enabled=? WHERE gateway_id=?`,
		g.Name, g.Host, g.Port, g.Region, g.Enabled, g.GatewayID,
	)
	return err
}

// Delete 删除网关（不存在时不报错，与重构前行为一致）。
func (r *Repository) Delete(ctx context.Context, gatewayID string) error {
	_, err := r.db().ExecContext(ctx, `DELETE FROM relay_gateways WHERE gateway_id = ?`, gatewayID)
	return err
}
