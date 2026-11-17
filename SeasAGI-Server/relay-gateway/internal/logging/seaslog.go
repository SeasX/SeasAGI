// Package logging 提供社区版内置日志器。
//
// 说明：原实现依赖未公开发布的私有模块 github.com/SeasX/SeasLog4Go，
// go.mod 中以开发者本机绝对路径 replace，导致外部贡献者无法复现构建。
// 现改为标准库实现，按天分文件写入 <baseLogDir>/<appName>/YYYY-MM-DD.log，
// 对外 API（Init / Logger / Close / 各级别方法）保持兼容。
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

const appName = "relay-gateway"

type fileLogger struct {
	mu   sync.Mutex
	dir  string
	day  string
	file *os.File
	out  *log.Logger
}

var (
	globalMu sync.Mutex
	logger   *fileLogger
	baseDir  string
)

// Init 初始化全局日志器。baseLogDir 为空时默认 /var/log/seasagi。
func Init(baseLogDir string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if logger != nil {
		return nil
	}
	if baseLogDir == "" {
		baseLogDir = "/var/log/seasagi"
	}
	dir := filepath.Join(baseLogDir, appName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	baseDir = baseLogDir
	logger = &fileLogger{dir: dir}
	return nil
}

// Logger 返回底层日志器（未初始化时为 nil）。
func Logger() *fileLogger {
	globalMu.Lock()
	defer globalMu.Unlock()
	return logger
}

// Close 关闭当前日志文件句柄。
func Close() error {
	l := Logger()
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
func LogPath() string {
	globalMu.Lock()
	defer globalMu.Unlock()
	return baseDir
}

func ensureInit() {
	globalMu.Lock()
	need := logger == nil
	globalMu.Unlock()
	if !need {
		return
	}
	if err := Init(""); err != nil {
		// 主路径不可写时退回临时目录，保证日志不丢。
		_ = Init(filepath.Join(os.TempDir(), "seasagi-logs"))
	}
}

func current() *fileLogger {
	ensureInit()
	return Logger()
}

func (l *fileLogger) rotate(day string) error {
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

func (l *fileLogger) write(level, msg string, context ...map[string]string) error {
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

func (l *fileLogger) Info(msg string, context ...map[string]string) error {
	return l.write("INFO", msg, context...)
}
func (l *fileLogger) Error(msg string, context ...map[string]string) error {
	return l.write("ERROR", msg, context...)
}
func (l *fileLogger) Warning(msg string, context ...map[string]string) error {
	return l.write("WARNING", msg, context...)
}
func (l *fileLogger) Debug(msg string, context ...map[string]string) error {
	return l.write("DEBUG", msg, context...)
}
func (l *fileLogger) Notice(msg string, context ...map[string]string) error {
	return l.write("NOTICE", msg, context...)
}
func (l *fileLogger) Critical(msg string, context ...map[string]string) error {
	return l.write("CRITICAL", msg, context...)
}
func (l *fileLogger) Alert(msg string, context ...map[string]string) error {
	return l.write("ALERT", msg, context...)
}
func (l *fileLogger) Emergency(msg string, context ...map[string]string) error {
	return l.write("EMERGENCY", msg, context...)
}

func logAt(level, msg string, context ...map[string]string) error {
	l := current()
	if l == nil {
		return fmt.Errorf("logger unavailable")
	}
	return l.write(level, msg, context...)
}

// --- Convenience wrappers（签名与旧实现一致）---

func Info(msg string, context ...map[string]string) error  { return logAt("INFO", msg, context...) }
func Error(msg string, context ...map[string]string) error { return logAt("ERROR", msg, context...) }
func Warning(msg string, context ...map[string]string) error {
	return logAt("WARNING", msg, context...)
}
func Debug(msg string, context ...map[string]string) error  { return logAt("DEBUG", msg, context...) }
func Notice(msg string, context ...map[string]string) error { return logAt("NOTICE", msg, context...) }
func Critical(msg string, context ...map[string]string) error {
	return logAt("CRITICAL", msg, context...)
}
func Alert(msg string, context ...map[string]string) error { return logAt("ALERT", msg, context...) }
func Emergency(msg string, context ...map[string]string) error {
	return logAt("EMERGENCY", msg, context...)
}

// Fatal 以 EMERGENCY 级别记录后退出进程。
func Fatal(msg string) {
	_ = Emergency(msg)
	_ = Close()
	os.Exit(1)
}

// Fatalf 格式化后以 EMERGENCY 级别记录并退出。
func Fatalf(format string, args ...any) {
	Fatal(fmt.Sprintf(format, args...))
}

func Infof(format string, args ...any) error    { return Info(fmt.Sprintf(format, args...)) }
func Errorf(format string, args ...any) error   { return Error(fmt.Sprintf(format, args...)) }
func Warningf(format string, args ...any) error { return Warning(fmt.Sprintf(format, args...)) }
func Debugf(format string, args ...any) error   { return Debug(fmt.Sprintf(format, args...)) }

// LogFilePath 返回当前服务的日志目录。
func LogFilePath() string {
	ensureInit()
	return filepath.Join(LogPath(), appName)
}
