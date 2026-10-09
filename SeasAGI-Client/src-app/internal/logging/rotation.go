package logging

import (
	"errors"
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
//
// 支持两种模式：
//   - 单文件模式：logPath 指向一个日志文件（如 app.log），超过 MaxFileSize 时重命名轮转。
//   - 目录模式：logPath 指向日志目录（如 ~/.seasagi/logs/client），
//     写入方按日自行滚动文件（YYYYMMDD.log），本器只负责保留策略。
type LogRotator struct {
	mu      sync.Mutex
	config  RotationConfig
	logPath string
	stopCh  chan struct{}
}

// NewLogRotator 创建日志轮转器。logPath 可以是日志文件或日志目录。
func NewLogRotator(logPath string, config RotationConfig) *LogRotator {
	return &LogRotator{
		config:  config,
		logPath: logPath,
		stopCh:  make(chan struct{}),
	}
}

// Config 返回当前配置副本。
func (r *LogRotator) Config() RotationConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.config
}

// UpdateConfig 更新配置。若正在 Start() 定时运行，新的 CheckInterval 将在下次循环生效。
func (r *LogRotator) UpdateConfig(config RotationConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if config.MaxFileSize <= 0 {
		config.MaxFileSize = r.config.MaxFileSize
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = r.config.RetentionDays
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = r.config.MaxFiles
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = r.config.CheckInterval
	}
	r.config = config
}

// LogPath 返回被管理的日志文件或目录路径。
func (r *LogRotator) LogPath() string {
	return r.logPath
}

// TotalSize 返回被管理日志的当前总字节数。目录模式下统计目录内所有 *.log。
func (r *LogRotator) TotalSize() (int64, error) {
	info, err := os.Stat(r.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return info.Size(), nil
	}
	var total int64
	entries, err := os.ReadDir(r.logPath)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		total += fi.Size()
	}
	return total, nil
}

// CheckAndRotate 检查当前日志文件大小，必要时轮转。
// 目录模式下日志由写入方按日滚动，本方法只执行保留清理。
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

	if info.IsDir() {
		return r.cleanRetentionLocked()
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
	return r.cleanRetentionLocked()
}

// cleanRetentionLocked 执行保留清理。调用方需持锁。
func (r *LogRotator) cleanRetentionLocked() error {
	dir := filepath.Dir(r.logPath)
	ext := filepath.Ext(r.logPath)
	prefix := strings.TrimSuffix(filepath.Base(r.logPath), ext) + "."

	// 目录模式：目录内所有 *.log 均为候选（日志按日滚动，无统一前缀）。
	if info, err := os.Stat(r.logPath); err == nil && info.IsDir() {
		dir = r.logPath
		ext = ".log"
		prefix = ""
	}

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
	var errs []error

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
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				errs = append(errs, fmt.Errorf("remove expired log %s: %w", name, err))
			}
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
			if err := os.Remove(files[i].Path); err != nil {
				errs = append(errs, fmt.Errorf("remove excess log %s: %w", files[i].Path, err))
			}
		}
	}

	return errors.Join(errs...)
}

// Start 启动定时检查。重复调用（在 Stop 之前）不会启动多个 goroutine。
func (r *LogRotator) Start() {
	r.mu.Lock()
	if r.stopCh != nil { // 已在运行
		r.mu.Unlock()
		return
	}
	// 每次 Start 都创建新的 stopCh，避免 Stop→Start 后 goroutine 因 nil channel
	// 永远阻塞而泄漏。
	r.stopCh = make(chan struct{})
	stopCh := r.stopCh
	r.mu.Unlock()

	go func() {
		for {
			// 每次循环重新读取间隔，使 UpdateConfig 修改的 CheckInterval 能在
			// 下一个周期生效。
			interval := r.Config().CheckInterval
			if interval <= 0 {
				interval = DefaultRotationConfig().CheckInterval
			}
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
				if err := r.CheckAndRotate(); err != nil {
					Errorf("log rotation failed: %v", err)
				}
				if err := r.CleanRetention(); err != nil {
					Errorf("log retention cleanup failed: %v", err)
				}
			case <-stopCh:
				timer.Stop()
				return
			}
		}
	}()
}

// Stop 停止定时检查。幂等。
func (r *LogRotator) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopCh == nil {
		return
	}
	close(r.stopCh)
	r.stopCh = nil
}
