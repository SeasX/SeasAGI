package timeutil

import (
	"testing"
	"time"
)

// 写入格式必须是 UTC 的 "2006-01-02 15:04:05"，与 SQLite CURRENT_TIMESTAMP 一致。
func TestFormatSQLiteNormalizesToUTC(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	got := FormatSQLite(time.Date(2026, 10, 25, 8, 0, 0, 0, zone))
	if want := "2026-10-25 00:00:00"; got != want {
		t.Fatalf("FormatSQLite = %q, want %q", got, want)
	}
}

// 读取时必须兼容 RFC3339 与空格格式，并统一输出为空格格式。
func TestNormalizeAcceptsRFC3339AndSQLite(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"2026-10-25T08:00:00Z", "2026-10-25 08:00:00"},
		{"2026-10-25T08:00:00+08:00", "2026-10-25 00:00:00"},
		{"2026-10-25 08:00:00", "2026-10-25 08:00:00"},
		{"", ""},
		{"not-a-timestamp", "not-a-timestamp"},
	}
	for _, tc := range cases {
		if got := Normalize(tc.in); got != tc.want {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// NowSQLite 必须可被 SQLiteLayout 解析，且不含 "T"。
func TestNowSQLiteMatchesLayout(t *testing.T) {
	now := NowSQLite()
	if _, err := time.Parse(SQLiteLayout, now); err != nil {
		t.Fatalf("NowSQLite() = %q, not parsable with SQLiteLayout: %v", now, err)
	}
	if len(now) != len(SQLiteLayout) {
		t.Fatalf("NowSQLite() = %q, want layout length %d", now, len(SQLiteLayout))
	}
}
