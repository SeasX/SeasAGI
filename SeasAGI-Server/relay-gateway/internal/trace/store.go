package trace

import (
	"database/sql"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

type Record struct {
	TraceID    string `json:"trace_id"`
	UserID     string `json:"user_id"`
	DeviceID   string `json:"device_id"`
	TenantID   string `json:"tenant_id"`
	ChannelID  string `json:"channel_id"`
	Model      string `json:"model"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int    `json:"latency_ms"`
	ClientIP   string `json:"client_ip"`
	CreatedAt  string `json:"created_at"`
}

var (
	storeOnce sync.Once
	store     *TraceStore
)

type TraceStore struct {
	mu sync.Mutex
	db *sql.DB
}

func InitStore(dbPath string) {
	storeOnce.Do(func() {
		db, err := openSQLite(dbPath)
		if err != nil {
			logging.Errorf("trace store open error: %v", err)
			return
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS trace_records (
			trace_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT '',
			device_id TEXT NOT NULL DEFAULT '',
			tenant_id TEXT NOT NULL DEFAULT 'default',
			channel_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			method TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			client_ip TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
			logging.Errorf("trace store init error: %v", err)
			return
		}
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_trace_user ON trace_records(user_id, created_at)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_trace_device ON trace_records(device_id, created_at)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_trace_tenant ON trace_records(tenant_id, created_at)`)
		db.Exec(`CREATE INDEX IF NOT EXISTS idx_trace_time ON trace_records(created_at)`)
		store = &TraceStore{db: db}
	})
}

func GetStore() *TraceStore {
	return store
}

func (s *TraceStore) Insert(rec Record) {
	if s == nil || s.db == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`INSERT OR IGNORE INTO trace_records (trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.TraceID, rec.UserID, rec.DeviceID, rec.TenantID, rec.ChannelID, rec.Model, rec.Method, rec.Path, rec.StatusCode, rec.LatencyMs, rec.ClientIP, rec.CreatedAt)
}

func (s *TraceStore) QueryByTraceID(traceID string) (*Record, error) {
	if s == nil || s.db == nil {
		return nil, sql.ErrNoRows
	}
	var rec Record
	err := s.db.QueryRow(`SELECT trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at FROM trace_records WHERE trace_id = ?`, traceID).Scan(&rec.TraceID, &rec.UserID, &rec.DeviceID, &rec.TenantID, &rec.ChannelID, &rec.Model, &rec.Method, &rec.Path, &rec.StatusCode, &rec.LatencyMs, &rec.ClientIP, &rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

func (s *TraceStore) QueryByUserID(userID string, limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at FROM trace_records WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *TraceStore) QueryByDeviceID(deviceID string, limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at FROM trace_records WHERE device_id = ? ORDER BY created_at DESC LIMIT ?`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *TraceStore) QueryRecent(limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at FROM trace_records ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *TraceStore) PurgeBefore(before time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	result, err := s.db.Exec(`DELETE FROM trace_records WHERE created_at < ?`, before.Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func scanRecords(rows *sql.Rows) ([]Record, error) {
	items := make([]Record, 0)
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.TraceID, &rec.UserID, &rec.DeviceID, &rec.TenantID, &rec.ChannelID, &rec.Model, &rec.Method, &rec.Path, &rec.StatusCode, &rec.LatencyMs, &rec.ClientIP, &rec.CreatedAt); err != nil {
			continue
		}
		items = append(items, rec)
	}
	return items, nil
}
