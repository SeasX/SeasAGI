package trace

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/timeutil"
)

// 未配置时使用默认值：保留 30 天、每 6 小时清理一次。
func TestRetentionConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("TRACE_RETENTION_DAYS", "")
	t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "")

	cfg := RetentionConfigFromEnv()
	if !cfg.Enabled {
		t.Fatal("expected retention enabled by default")
	}
	if cfg.Retention != 30*24*time.Hour {
		t.Fatalf("retention = %v, want 720h", cfg.Retention)
	}
	if cfg.Interval != 6*time.Hour {
		t.Fatalf("interval = %v, want 6h", cfg.Interval)
	}
}

// TRACE_RETENTION_DAYS <= 0 表示禁用。
func TestRetentionConfigFromEnvDisabled(t *testing.T) {
	for _, value := range []string{"0", "-3"} {
		t.Setenv("TRACE_RETENTION_DAYS", value)
		t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "12")

		if cfg := RetentionConfigFromEnv(); cfg.Enabled {
			t.Fatalf("expected retention disabled for TRACE_RETENTION_DAYS=%s", value)
		}
	}
}

// 非法值回退默认；清理间隔 <1 按 1 小时处理。
func TestRetentionConfigFromEnvInvalidFallsBack(t *testing.T) {
	t.Setenv("TRACE_RETENTION_DAYS", "not-a-number")
	t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "oops")

	cfg := RetentionConfigFromEnv()
	if !cfg.Enabled || cfg.Retention != 30*24*time.Hour || cfg.Interval != 6*time.Hour {
		t.Fatalf("unexpected fallback config: %+v", cfg)
	}

	t.Setenv("TRACE_RETENTION_DAYS", "7")
	t.Setenv("TRACE_PURGE_INTERVAL_HOURS", "0")

	cfg = RetentionConfigFromEnv()
	if cfg.Retention != 7*24*time.Hour {
		t.Fatalf("retention = %v, want 168h", cfg.Retention)
	}
	if cfg.Interval != time.Hour {
		t.Fatalf("interval = %v, want 1h", cfg.Interval)
	}
}

// StartRetention 启动后立即清理，并按短周期持续清理过期记录。
func TestStartRetentionPurgesPeriodically(t *testing.T) {
	if testStore == nil {
		t.Fatal("test store not initialized")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartRetention(ctx, testStore, RetentionConfig{
		Enabled:   true,
		Retention: time.Hour,
		Interval:  10 * time.Millisecond,
	})

	// 启动后的首次清理已执行；此时插入过期记录，验证周期清理确实生效。
	traceID := fmt.Sprintf("test-retention-periodic-%d", time.Now().UnixNano())
	testStore.Insert(Record{
		TraceID:   traceID,
		UserID:    "user-retention",
		CreatedAt: timeutil.FormatSQLite(time.Now().Add(-48 * time.Hour)),
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := testStore.QueryByTraceID(traceID); err == sql.ErrNoRows {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("record %s was not purged by periodic retention", traceID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// 禁用时不得删除任何记录。
func TestStartRetentionDisabledKeepsRecords(t *testing.T) {
	if testStore == nil {
		t.Fatal("test store not initialized")
	}

	traceID := fmt.Sprintf("test-retention-disabled-%d", time.Now().UnixNano())
	testStore.Insert(Record{
		TraceID:   traceID,
		CreatedAt: timeutil.FormatSQLite(time.Now().Add(-48 * time.Hour)),
	})

	StartRetention(context.Background(), testStore, RetentionConfig{Enabled: false})

	if _, err := testStore.QueryByTraceID(traceID); err != nil {
		t.Fatalf("expected record to survive when retention disabled, got %v", err)
	}
}
