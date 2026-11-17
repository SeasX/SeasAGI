package trace

import (
	"database/sql"
	"sync"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
	"github.com/SeasAGI/SeasAGI-Server/shared/timeutil"
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

const (
	// traceBatchSize 达到后立即唤醒写入协程，避免缓冲区无限增长。
	traceBatchSize = 256
	// traceBatchWindow 是后台批量落盘的最大延迟。
	traceBatchWindow = 200 * time.Millisecond
)

type TraceStore struct {
	mu   sync.Mutex
	db   *sql.DB
	buf  []Record
	wake chan struct{}
	stop chan struct{}
	done chan struct{}
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
		s := &TraceStore{
			db:   db,
			wake: make(chan struct{}, 1),
			stop: make(chan struct{}),
			done: make(chan struct{}),
		}
		store = s
		go s.writeLoop()
	})
}

func GetStore() *TraceStore {
	return store
}

// Insert 只把记录放入内存缓冲区，由后台协程成批写入。
// 单请求不再创建 goroutine、也不再各自提交事务，避免逐行 fsync。
func (s *TraceStore) Insert(rec Record) {
	if s == nil || s.db == nil {
		return
	}
	s.mu.Lock()
	s.buf = append(s.buf, rec)
	full := len(s.buf) >= traceBatchSize
	s.mu.Unlock()
	if full {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

func (s *TraceStore) writeLoop() {
	defer close(s.done)
	ticker := time.NewTicker(traceBatchWindow)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			_ = s.Flush()
			return
		case <-ticker.C:
			if err := s.Flush(); err != nil {
				logging.Errorf("trace flush error: %v", err)
			}
		case <-s.wake:
			if err := s.Flush(); err != nil {
				logging.Errorf("trace flush error: %v", err)
			}
		}
	}
}

// Flush 把缓冲区在一个事务内批量写入；查询前会隐式调用以保证读己之写。
func (s *TraceStore) Flush() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

func (s *TraceStore) flushLocked() error {
	if len(s.buf) == 0 {
		return nil
	}
	batch := s.buf
	s.buf = nil

	tx, err := s.db.Begin()
	if err != nil {
		s.buf = batch
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO trace_records (trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		s.buf = batch
		return err
	}
	for _, rec := range batch {
		if _, err := stmt.Exec(rec.TraceID, rec.UserID, rec.DeviceID, rec.TenantID, rec.ChannelID, rec.Model, rec.Method, rec.Path, rec.StatusCode, rec.LatencyMs, rec.ClientIP, rec.CreatedAt); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			s.buf = append(batch, s.buf...)
			return err
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		s.buf = append(batch, s.buf...)
		return err
	}
	return nil
}

// Close 停止后台写入并落盘剩余记录。主要用于测试与优雅退出。
func (s *TraceStore) Close() {
	if s == nil {
		return
	}
	select {
	case <-s.stop:
		return
	default:
		close(s.stop)
	}
	<-s.done
}

func (s *TraceStore) QueryByTraceID(traceID string) (*Record, error) {
	if s == nil || s.db == nil {
		return nil, sql.ErrNoRows
	}
	_ = s.Flush()
	var rec Record
	err := s.db.QueryRow(`SELECT trace_id, user_id, device_id, tenant_id, channel_id, model, method, path, status_code, latency_ms, client_ip, created_at FROM trace_records WHERE trace_id = ?`, traceID).Scan(&rec.TraceID, &rec.UserID, &rec.DeviceID, &rec.TenantID, &rec.ChannelID, &rec.Model, &rec.Method, &rec.Path, &rec.StatusCode, &rec.LatencyMs, &rec.ClientIP, &rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	// 兼容历史 RFC3339 数据：读取时统一为 SQLite 时间格式。
	rec.CreatedAt = timeutil.Normalize(rec.CreatedAt)
	return &rec, nil
}

func (s *TraceStore) QueryByUserID(userID string, limit int) ([]Record, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	_ = s.Flush()
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
	_ = s.Flush()
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
	_ = s.Flush()
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

// PurgeBefore 删除早于指定时间的 trace。
// 使用 datetime() 比较，兼容历史 RFC3339 与 SQLite 空格格式混存的数据。
func (s *TraceStore) PurgeBefore(before time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	_ = s.Flush()
	result, err := s.db.Exec(`DELETE FROM trace_records WHERE datetime(created_at) < datetime(?)`, timeutil.FormatSQLite(before))
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
		// 兼容历史 RFC3339 数据：读取时统一为 SQLite 时间格式。
		rec.CreatedAt = timeutil.Normalize(rec.CreatedAt)
		items = append(items, rec)
	}
	return items, nil
}
