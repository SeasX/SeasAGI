package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RotationConfig 日志轮转配置。
type RotationConfig struct {
	MaxFileSize   int64         // 单文件最大字节数（默认 50MB）
	RetentionDays int           // 保留天数（默认 7）
	MaxFiles      int           // 最大文件数（默认 20）
	CheckInterval time.Duration // 检查间隔（默认 60s）
}

// DefaultRotationConfig 默认轮转配置。
func DefaultRotationConfig() RotationConfig {
	return RotationConfig{
		MaxFileSize:   50 * 1024 * 1024, // 50MB
		RetentionDays: 7,
		MaxFiles:      20,
		CheckInterval: 60 * time.Second,
	}
}

// LogRotator 日志轮转器。
type LogRotator struct {
	mu       sync.Mutex
	config   RotationConfig
	logPath  string
	stopCh   chan struct{}
}

// NewLogRotator 创建日志轮转器。
func NewLogRotator(logPath string, config RotationConfig) *LogRotator {
	return &LogRotator{
		config:  config,
		logPath: logPath,
		stopCh:  make(chan struct{}),
	}
}

// CheckAndRotate 检查当前日志文件大小，必要时轮转。
func (r *LogRotator) CheckAndRotate() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, err := os.Stat(r.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat log file: %w", err)
	}

	if info.Size() < r.config.MaxFileSize {
		return nil
	}

	// 轮转：重命名为 app.YYYY-MM-DD_HHmmss.log
	timestamp := time.Now().Format("2006-01-02_150405")
	dir := filepath.Dir(r.logPath)
	ext := filepath.Ext(r.logPath)
	base := strings.TrimSuffix(filepath.Base(r.logPath), ext)
	rotatedPath := filepath.Join(dir, fmt.Sprintf("%s.%s%s", base, timestamp, ext))

	if err := os.Rename(r.logPath, rotatedPath); err != nil {
		return fmt.Errorf("rotate log: %w", err)
	}

	return nil
}

// CleanRetention 按保留天数和最大文件数清理旧日志。
func (r *LogRotator) CleanRetention() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := filepath.Dir(r.logPath)
	ext := filepath.Ext(r.logPath)
	base := strings.TrimSuffix(filepath.Base(r.logPath), ext)
	prefix := base + "."

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read log dir: %w", err)
	}

	type fileInfo struct {
		Path    string
		ModTime time.Time
	}
	var files []fileInfo

	cutoff := time.Now().AddDate(0, 0, -r.config.RetentionDays)

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		// 按保留天数清理
		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(dir, name))
			continue
		}
		files = append(files, fileInfo{Path: filepath.Join(dir, name), ModTime: info.ModTime()})
	}

	// 按最大文件数清理（保留最新的 MaxFiles 个）
	if len(files) > r.config.MaxFiles {
		sort.Slice(files, func(i, j int) bool {
			return files[i].ModTime.Before(files[j].ModTime)
		})
		excess := len(files) - r.config.MaxFiles
		for i := 0; i < excess; i++ {
			os.Remove(files[i].Path)
		}
	}

	return nil
}

// Start 启动定时检查。
func (r *LogRotator) Start() {
	go func() {
		ticker := time.NewTicker(r.config.CheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = r.CheckAndRotate()
				_ = r.CleanRetention()
			case <-r.stopCh:
				return
			}
		}
	}()
}

// Stop 停止定时检查。
func (r *LogRotator) Stop() {
	close(r.stopCh)
}
