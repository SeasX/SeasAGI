package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	seaslog "github.com/SeasX/SeasLog4Go"
)

var (
	mu      sync.Mutex
	logger  *seaslog.SeasLog
	baseDir string
)

// Init initializes the global SeasLog4Go logger for platform-api.
// baseLogDir defaults to /var/log/seasagi if empty.
func Init(baseLogDir string) error {
	mu.Lock()
	defer mu.Unlock()

	if logger != nil {
		return nil
	}

	if baseLogDir == "" {
		baseLogDir = "/var/log/seasagi"
	}
	if err := os.MkdirAll(baseLogDir, 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}
	baseDir = baseLogDir

	logger = seaslog.NewSeasLog(
		seaslog.WithBasePath(baseLogDir),
		seaslog.WithLogger("platform-api"),
		seaslog.WithTemplate("%T | %L | %P | %Q | %t | %F | %M"),
		seaslog.WithLevel(seaslog.LevelAll),
		seaslog.WithTrimWrap(),
	)
	return nil
}

// Logger returns the underlying SeasLog instance (nil if not initialized).
func Logger() *seaslog.SeasLog {
	mu.Lock()
	defer mu.Unlock()
	return logger
}

// Close flushes buffers and releases all streams.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if logger != nil {
		return logger.Close()
	}
	return nil
}

// LogPath returns the base log directory.
func LogPath() string {
	mu.Lock()
	defer mu.Unlock()
	return baseDir
}

func ensureInit() {
	if logger == nil {
		_ = Init("")
	}
	if logger == nil {
		// Fallback: use temp dir if the primary path is not writable
		_ = Init(filepath.Join(os.TempDir(), "seasagi-logs"))
	}
}

// --- Convenience wrappers ---

func Info(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Info(msg, context...)
}

func Error(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Error(msg, context...)
}

func Warning(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Warning(msg, context...)
}

func Debug(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Debug(msg, context...)
}

func Notice(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Notice(msg, context...)
}

func Critical(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Critical(msg, context...)
}

func Alert(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Alert(msg, context...)
}

func Emergency(msg string, context ...map[string]string) error {
	ensureInit()
	return logger.Emergency(msg, context...)
}

// Fatal logs at EMERGENCY level and exits the process.
func Fatal(msg string) {
	ensureInit()
	_ = logger.Emergency(msg)
	_ = logger.Close()
	os.Exit(1)
}

// Fatalf logs a formatted message at EMERGENCY level and exits.
func Fatalf(format string, args ...any) {
	Fatal(fmt.Sprintf(format, args...))
}

// Infof logs a formatted message at INFO level.
func Infof(format string, args ...any) error {
	return Info(fmt.Sprintf(format, args...))
}

// Errorf logs a formatted message at ERROR level.
func Errorf(format string, args ...any) error {
	return Error(fmt.Sprintf(format, args...))
}

// Warningf logs a formatted message at WARNING level.
func Warningf(format string, args ...any) error {
	return Warning(fmt.Sprintf(format, args...))
}

// Debugf logs a formatted message at DEBUG level.
func Debugf(format string, args ...any) error {
	return Debug(fmt.Sprintf(format, args...))
}

// LogFilePath returns the expected log file path for the current date.
func LogFilePath() string {
	ensureInit()
	return filepath.Join(baseDir, "platform-api")
}
