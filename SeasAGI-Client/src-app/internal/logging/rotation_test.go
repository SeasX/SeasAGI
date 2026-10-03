package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 目录模式：SeasLog 按日滚动的 YYYYMMDD.log 文件应按保留天数清理。
func TestLogRotatorDirModeRetention(t *testing.T) {
	dir := t.TempDir()

	oldPath := filepath.Join(dir, "20260101.log")
	newPath := filepath.Join(dir, "20260929.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0644); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0644); err != nil {
		t.Fatalf("write new: %v", err)
	}
	old := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	r := NewLogRotator(dir, RotationConfig{
		MaxFileSize:   50 * 1024 * 1024,
		RetentionDays: 7,
		MaxFiles:      20,
		CheckInterval: time.Hour,
	})

	if err := r.CleanRetention(); err != nil {
		t.Fatalf("CleanRetention: %v", err)
	}
	if _, err := os.Stat(oldPath); err == nil {
		t.Error("old daily log should be removed")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Error("recent daily log should be retained")
	}
}

// 目录模式下 CheckAndRotate 不应重命名文件（写入方自行按日滚动）。
func TestLogRotatorDirModeCheckAndRotateKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "20260929.log")
	if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := NewLogRotator(dir, RotationConfig{
		MaxFileSize:   1, // 故意设得极小，单文件模式会触发轮转
		RetentionDays: 7,
		MaxFiles:      20,
		CheckInterval: time.Hour,
	})
	if err := r.CheckAndRotate(); err != nil {
		t.Fatalf("CheckAndRotate: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Error("dir mode should not rotate daily files")
	}
}

func TestLogRotatorTotalSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260929.log"), []byte("12345"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("should-not-count"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := NewLogRotator(dir, DefaultRotationConfig())
	total, err := r.TotalSize()
	if err != nil {
		t.Fatalf("TotalSize: %v", err)
	}
	if total != 5 {
		t.Errorf("TotalSize: got %d, want 5", total)
	}
}

func TestLogRotatorUpdateConfig(t *testing.T) {
	r := NewLogRotator("/tmp/x.log", DefaultRotationConfig())

	r.UpdateConfig(RotationConfig{MaxFiles: 3})
	cfg := r.Config()
	if cfg.MaxFiles != 3 {
		t.Errorf("MaxFiles: got %d, want 3", cfg.MaxFiles)
	}
	// 未提供的字段应保持原值
	if cfg.RetentionDays != DefaultRotationConfig().RetentionDays {
		t.Errorf("RetentionDays should be preserved, got %d", cfg.RetentionDays)
	}

	// 非法值（0/负数）不应覆盖
	r.UpdateConfig(RotationConfig{MaxFiles: -1, RetentionDays: 0})
	cfg = r.Config()
	if cfg.MaxFiles != 3 {
		t.Errorf("MaxFiles should not be overwritten by invalid value, got %d", cfg.MaxFiles)
	}
	if cfg.RetentionDays != DefaultRotationConfig().RetentionDays {
		t.Errorf("RetentionDays should not be overwritten by invalid value, got %d", cfg.RetentionDays)
	}
}

func TestLogRotatorStopIdempotent(t *testing.T) {
	r := NewLogRotator("/tmp/x.log", RotationConfig{CheckInterval: time.Hour})
	r.Start()
	r.Stop()
	r.Stop() // 重复 Stop 不应 panic
}
