package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/timeutil"
)

func initTempDB(t *testing.T) {
	t.Helper()
	if err := Init(filepath.Join(t.TempDir(), "test.db")); err != nil {
		t.Fatalf("database init failed: %v", err)
	}
	t.Cleanup(Close)
}

func indexExists(t *testing.T, name string) bool {
	t.Helper()
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("query index %s failed: %v", name, err)
	}
	return count > 0
}

func tableExists(t *testing.T, name string) bool {
	t.Helper()
	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("query table %s failed: %v", name, err)
	}
	return count > 0
}

// 查询索引必须随初始化幂等创建。
func TestInitCreatesQueryIndexes(t *testing.T) {
	initTempDB(t)

	for _, name := range []string{
		"idx_usage_events_user_recorded",
		"idx_usage_events_tenant_recorded",
		"idx_usage_records_user_model_recorded",
	} {
		if !indexExists(t, name) {
			t.Fatalf("expected index %s to exist", name)
		}
	}
}

// 社区版已移除 invoices 表：索引创建必须跳过而不是报错；
// 表存在时（例如历史库）仍应创建议价索引。
func TestEnsureQueryIndexesSkipsMissingInvoicesTable(t *testing.T) {
	initTempDB(t)

	if tableExists(t, "invoices") {
		t.Fatal("community schema should not contain invoices table")
	}
	if indexExists(t, "idx_invoices_user_period") {
		t.Fatal("invoices index must not exist without invoices table")
	}

	if _, err := DB.Exec(`CREATE TABLE invoices (
		invoice_id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		period_start DATETIME NOT NULL
	)`); err != nil {
		t.Fatalf("create invoices table failed: %v", err)
	}

	// 重复执行验证幂等。
	for i := 0; i < 2; i++ {
		if err := ensureQueryIndexes(DB); err != nil {
			t.Fatalf("ensureQueryIndexes failed: %v", err)
		}
	}
	if !indexExists(t, "idx_invoices_user_period") {
		t.Fatal("expected idx_invoices_user_period after invoices table exists")
	}
}

// 写池必须保持单连接；读池设置空闲/生命周期上限。
func TestConnectionPoolSettings(t *testing.T) {
	initTempDB(t)

	if got := DB.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("write pool max open conns = %d, want 1", got)
	}
	if ReadDB == nil {
		t.Fatal("expected read pool to be initialized")
	}
	if got := ReadDB.Stats().MaxOpenConnections; got < 4 {
		t.Fatalf("read pool max open conns = %d, want >= 4", got)
	}
}

// trace 清理只删除过期记录，保留近期记录。
func TestPurgeTraceRecordsBeforeDeletesOnlyOldRecords(t *testing.T) {
	initTempDB(t)

	oldAt := timeutil.FormatSQLite(time.Now().Add(-48 * time.Hour))
	recentAt := timeutil.FormatSQLite(time.Now())
	if _, err := DB.Exec(`INSERT INTO trace_records (trace_id, user_id, created_at) VALUES (?, ?, ?)`, "trace-old", "user-1", oldAt); err != nil {
		t.Fatalf("insert old trace failed: %v", err)
	}
	if _, err := DB.Exec(`INSERT INTO trace_records (trace_id, user_id, created_at) VALUES (?, ?, ?)`, "trace-recent", "user-1", recentAt); err != nil {
		t.Fatalf("insert recent trace failed: %v", err)
	}

	deleted, err := PurgeTraceRecordsBefore(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("purge failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	var count int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM trace_records WHERE trace_id = 'trace-old'`).Scan(&count); err != nil {
		t.Fatalf("count old failed: %v", err)
	}
	if count != 0 {
		t.Fatal("expected old trace record to be purged")
	}
	if err := DB.QueryRow(`SELECT COUNT(*) FROM trace_records WHERE trace_id = 'trace-recent'`).Scan(&count); err != nil {
		t.Fatalf("count recent failed: %v", err)
	}
	if count != 1 {
		t.Fatal("expected recent trace record to survive purge")
	}
}

// 环境变量解析：默认 / 禁用 / 非法回退。
func TestTraceRetentionConfigFromEnv(t *testing.T) {
	t.Setenv("TRACE_RETENTION_DAYS", "")
	t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "")

	cfg := TraceRetentionConfigFromEnv()
	if !cfg.Enabled || cfg.Retention != 30*24*time.Hour || cfg.Interval != 6*time.Hour {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	t.Setenv("TRACE_RETENTION_DAYS", "0")
	if cfg := TraceRetentionConfigFromEnv(); cfg.Enabled {
		t.Fatal("expected retention disabled for TRACE_RETENTION_DAYS=0")
	}

	t.Setenv("TRACE_RETENTION_DAYS", "abc")
	t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "0")
	cfg = TraceRetentionConfigFromEnv()
	if !cfg.Enabled || cfg.Retention != 30*24*time.Hour {
		t.Fatalf("expected invalid days to fall back to 30, got %+v", cfg)
	}
	if cfg.Interval != time.Hour {
		t.Fatalf("interval = %v, want 1h", cfg.Interval)
	}
}
