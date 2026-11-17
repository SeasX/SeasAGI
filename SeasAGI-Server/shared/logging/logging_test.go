package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 日志写入 <base>/<appName>/YYYY-MM-DD.log，格式与拆分前一致；不同服务互不干扰。
func TestServiceWritesPerAppDailyFile(t *testing.T) {
	base := t.TempDir()
	a, b := New("platform-api"), New("relay-gateway")
	for _, s := range []*Service{a, b} {
		if err := s.Init(base); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
	}
	if err := a.Log("INFO", "hello", map[string]string{"b": "2", "a": "1"}); err != nil {
		t.Fatal(err)
	}
	if err := b.Logger().Warning("relay"); err != nil {
		t.Fatal(err)
	}

	day := time.Now().UTC().Format("2006-01-02")
	got, err := os.ReadFile(filepath.Join(base, "platform-api", day+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if line := strings.TrimSpace(string(got)); !strings.HasSuffix(line, " | INFO | hello | a=1 | b=2") {
		t.Fatalf("unexpected line %q", line)
	}
	if got, _ := os.ReadFile(filepath.Join(base, "relay-gateway", day+".log")); !strings.Contains(string(got), " | WARNING | relay") {
		t.Fatalf("relay log = %q", got)
	}
	if a.LogPath() != base || a.LogFilePath() != filepath.Join(base, "platform-api") {
		t.Fatalf("paths = %q %q", a.LogPath(), a.LogFilePath())
	}
}

// 重复 Init 不会切换目录（与原实现一致）。
func TestInitIsIdempotent(t *testing.T) {
	s := New("svc")
	first, second := t.TempDir(), t.TempDir()
	if err := s.Init(first); err != nil {
		t.Fatal(err)
	}
	if err := s.Init(second); err != nil {
		t.Fatal(err)
	}
	if s.LogPath() != first {
		t.Fatalf("LogPath = %q, want %q", s.LogPath(), first)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
