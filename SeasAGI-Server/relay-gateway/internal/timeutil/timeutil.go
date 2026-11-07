// Package timeutil 统一数据库时间字段的写入与读取格式（与 platform-api 保持一致）。
package timeutil

import (
	"strings"
	"time"
)

// SQLiteLayout 是与 SQLite CURRENT_TIMESTAMP 一致的 UTC 时间格式。
const SQLiteLayout = "2006-01-02 15:04:05"

// NowSQLite 返回当前 UTC 时间的 SQLite 格式字符串。
func NowSQLite() string {
	return FormatSQLite(time.Now())
}

// FormatSQLite 将时间统一转换为 UTC 的 SQLite 格式字符串。
func FormatSQLite(t time.Time) string {
	return t.UTC().Format(SQLiteLayout)
}

// Normalize 兼容读取历史数据：把 RFC3339（含 Z 或时区偏移）统一转换为 SQLite 格式；
// 已经是 SQLite 格式或无法解析时原样返回。
func Normalize(s string) string {
	if t, ok := parseFlexible(s); ok {
		return FormatSQLite(t)
	}
	return s
}

// parseFlexible 依次尝试常见的时间格式。
func parseFlexible(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		SQLiteLayout,
		"2006-01-02 15:04:05.999999999-07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
