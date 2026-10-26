package database

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/logging"
	"github.com/SeasAGI/SeasAGI-Server/platform-api/internal/timeutil"
)

const (
	defaultTraceRetentionDays   = 30
	defaultTracePurgeIntervalHr = 6
)

// TraceRetentionConfig 描述 trace_records 的保留策略。
type TraceRetentionConfig struct {
	Enabled   bool
	Retention time.Duration
	Interval  time.Duration
}

// TraceRetentionConfigFromEnv 读取与 relay-gateway 相同的环境变量语义：
//   - TRACE_RETENTION_DAYS：保留天数，默认 30；<=0 表示禁用清理
//   - TRACE_PURGE_INTERVAL_HOURS：清理间隔小时数，默认 6；<1 按 1 处理
//
// 非法值回退默认值。
func TraceRetentionConfigFromEnv() TraceRetentionConfig {
	days := defaultTraceRetentionDays
	if v := strings.TrimSpace(os.Getenv("TRACE_RETENTION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}

	hours := defaultTracePurgeIntervalHr
	if v := strings.TrimSpace(os.Getenv("TRACE_PURGE_INTERVAL_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			hours = n
		}
	}
	if hours < 1 {
		hours = 1
	}

	if days <= 0 {
		return TraceRetentionConfig{Enabled: false}
	}
	return TraceRetentionConfig{
		Enabled:   true,
		Retention: time.Duration(days) * 24 * time.Hour,
		Interval:  time.Duration(hours) * time.Hour,
	}
}

// PurgeTraceRecordsBefore 删除 created_at 早于 before 的 trace 记录。
// 使用 datetime() 比较，兼容历史 RFC3339 与 SQLite 空格格式混存的数据。
func PurgeTraceRecordsBefore(before time.Time) (int64, error) {
	if DB == nil {
		return 0, nil
	}
	result, err := DB.Exec(`DELETE FROM trace_records WHERE datetime(created_at) < datetime(?)`, timeutil.FormatSQLite(before))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// StartTraceRetention 启用后立即清理一次，之后按 Interval 周期清理；ctx 结束时退出。
func StartTraceRetention(ctx context.Context, cfg TraceRetentionConfig) {
	if !cfg.Enabled || cfg.Retention <= 0 {
		logging.Infof("trace retention disabled")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Duration(defaultTracePurgeIntervalHr) * time.Hour
	}

	purge := func() {
		cutoff := time.Now().Add(-cfg.Retention)
		n, err := PurgeTraceRecordsBefore(cutoff)
		if err != nil {
			logging.Errorf("trace retention purge failed (before %s): %v", cutoff.Format(time.RFC3339), err)
			return
		}
		logging.Infof("trace retention purged %d records older than %s", n, cutoff.Format(time.RFC3339))
	}

	logging.Infof("trace retention enabled: keep %s, purge every %s", cfg.Retention, cfg.Interval)
	purge()

	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logging.Infof("trace retention stopped")
				return
			case <-ticker.C:
				purge()
			}
		}
	}()
}
