package trace

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/SeasAGI/SeasAGI-Server/relay-gateway/internal/logging"
)

const (
	defaultRetentionDays   = 30
	defaultPurgeIntervalHr = 6
)

// RetentionConfig 描述 trace 保留策略。
// Enabled 为 false 时不启动清理循环；Retention <= 0 同样视为禁用。
type RetentionConfig struct {
	Enabled   bool
	Retention time.Duration
	Interval  time.Duration
}

// RetentionConfigFromEnv 读取 trace 保留配置：
//   - TRACE_RETENTION_DAYS：保留天数，默认 30；<=0 表示禁用清理
//   - TRACE_PURGE_INTERVAL_HOURS：清理间隔小时数，默认 6；<1 按 1 处理
//
// 非法值回退默认值，避免配置错误导致清理被意外关闭或过于频繁。
func RetentionConfigFromEnv() RetentionConfig {
	days := defaultRetentionDays
	if v := strings.TrimSpace(os.Getenv("TRACE_RETENTION_DAYS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			days = n
		}
	}

	hours := defaultPurgeIntervalHr
	if v := strings.TrimSpace(os.Getenv("TRACE_PURGE_INTERVAL_HOURS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			hours = n
		}
	}
	if hours < 1 {
		hours = 1
	}

	if days <= 0 {
		return RetentionConfig{Enabled: false}
	}
	return RetentionConfig{
		Enabled:   true,
		Retention: time.Duration(days) * 24 * time.Hour,
		Interval:  time.Duration(hours) * time.Hour,
	}
}

// StartRetention 启用后在当前 goroutine 立即执行一次清理，之后按 Interval 周期清理；
// ctx 结束时退出循环。
func StartRetention(ctx context.Context, store *TraceStore, cfg RetentionConfig) {
	if !cfg.Enabled || cfg.Retention <= 0 {
		logging.Infof("trace retention disabled")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = time.Duration(defaultPurgeIntervalHr) * time.Hour
	}

	purge := func() {
		cutoff := time.Now().Add(-cfg.Retention)
		n, err := store.PurgeBefore(cutoff)
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
