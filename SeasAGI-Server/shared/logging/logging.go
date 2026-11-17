// Package logging 是 platform-api 与 relay-gateway 共用的标准库日志实现。
//
// 按天分文件写入 <baseLogDir>/<appName>/YYYY-MM-DD.log。每个服务通过 New(appName)
// 持有一个独立的 Service，并在自己的 internal/logging 中以包级函数暴露，
// 因此各服务的日志目录、调用方式与拆分前完全一致。
package logging

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultBaseDir 是未指定日志根目录时使用的位置。
const DefaultBaseDir = "/var/log/seasagi"

// Service 是单个服务的日志器状态（日志根目录 + 当前文件）。
type Service struct {
	appName string

	mu      sync.Mutex
	logger  *FileLogger
	baseDir string
}

// New 创建一个服务日志器；appName 决定日志子目录名。
func New(appName string) *Service {
	return &Service{appName: appName}
}

// FileLogger 按天滚动写入日志文件。
type FileLogger struct {
	mu   sync.Mutex
	dir  string
	day  string
	file *os.File
	out  *log.Logger
}

// Init 初始化日志器。baseLogDir 为空时默认 DefaultBaseDir；重复调用无副作用。
func (s *Service) Init(baseLogDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logger != nil {
		return nil
	}
	if baseLogDir == "" {
		baseLogDir = DefaultBaseDir
	}
	dir := filepath.Join(baseLogDir, s.appName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	s.baseDir = baseLogDir
	s.logger = &FileLogger{dir: dir}
	return nil
}

// Logger 返回底层日志器（未初始化时为 nil）。
func (s *Service) Logger() *FileLogger {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.logger
}

// Close 关闭当前日志文件句柄。
func (s *Service) Close() error {
	l := s.Logger()
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	l.out = nil
	l.day = ""
	return err
}

// LogPath 返回日志根目录。
func (s *Service) LogPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baseDir
}

// LogFilePath 返回当前服务的日志目录。
func (s *Service) LogFilePath() string {
	s.ensureInit()
	return filepath.Join(s.LogPath(), s.appName)
}

func (s *Service) ensureInit() {
	s.mu.Lock()
	need := s.logger == nil
	s.mu.Unlock()
	if !need {
		return
	}
	if err := s.Init(""); err != nil {
		// 主路径不可写时退回临时目录，保证日志不丢。
		_ = s.Init(filepath.Join(os.TempDir(), "seasagi-logs"))
	}
}

// Log 以指定级别写一条日志（未初始化时自动初始化）。
func (s *Service) Log(level, msg string, context ...map[string]string) error {
	s.ensureInit()
	l := s.Logger()
	if l == nil {
		return fmt.Errorf("logger unavailable")
	}
	return l.write(level, msg, context...)
}

func (l *FileLogger) rotate(day string) error {
	if l.file != nil && l.day == day {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
		l.out = nil
	}
	f, err := os.OpenFile(filepath.Join(l.dir, day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	l.file = f
	l.day = day
	l.out = log.New(f, "", 0)
	return nil
}

func (l *FileLogger) write(level, msg string, context ...map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UTC()
	if err := l.rotate(now.Format("2006-01-02")); err != nil {
		fmt.Fprintf(os.Stderr, "logging: %v\n", err)
		return err
	}

	var b strings.Builder
	b.WriteString(now.Format("2006-01-02 15:04:05"))
	b.WriteString(" | ")
	b.WriteString(level)
	b.WriteString(" | ")
	b.WriteString(msg)
	if len(context) > 0 && context[0] != nil {
		keys := make([]string, 0, len(context[0]))
		for k := range context[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(" | ")
			b.WriteString(k)
			b.WriteString("=")
			b.WriteString(context[0][k])
		}
	}
	return l.out.Output(1, b.String())
}

func (l *FileLogger) Info(msg string, context ...map[string]string) error {
	return l.write("INFO", msg, context...)
}
func (l *FileLogger) Error(msg string, context ...map[string]string) error {
	return l.write("ERROR", msg, context...)
}
func (l *FileLogger) Warning(msg string, context ...map[string]string) error {
	return l.write("WARNING", msg, context...)
}
func (l *FileLogger) Debug(msg string, context ...map[string]string) error {
	return l.write("DEBUG", msg, context...)
}
func (l *FileLogger) Notice(msg string, context ...map[string]string) error {
	return l.write("NOTICE", msg, context...)
}
func (l *FileLogger) Critical(msg string, context ...map[string]string) error {
	return l.write("CRITICAL", msg, context...)
}
func (l *FileLogger) Alert(msg string, context ...map[string]string) error {
	return l.write("ALERT", msg, context...)
}
func (l *FileLogger) Emergency(msg string, context ...map[string]string) error {
	return l.write("EMERGENCY", msg, context...)
}
