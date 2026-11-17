// Package logging 提供社区版内置日志器。
//
// 实现位于共享模块 github.com/SeasAGI/SeasAGI-Server/shared/logging；
// 这里仅固定本服务的 appName，并保持原有包级 API（Init / Logger / Close / 各级别方法）不变。
// 日志按天写入 <baseLogDir>/relay-gateway/YYYY-MM-DD.log。
package logging

import (
	"fmt"
	"os"

	sharedlog "github.com/SeasAGI/SeasAGI-Server/shared/logging"
)

const appName = "relay-gateway"

var svc = sharedlog.New(appName)

// Init 初始化全局日志器。baseLogDir 为空时默认 /var/log/seasagi。
func Init(baseLogDir string) error { return svc.Init(baseLogDir) }

// Logger 返回底层日志器（未初始化时为 nil）。
func Logger() *sharedlog.FileLogger { return svc.Logger() }

// Close 关闭当前日志文件句柄。
func Close() error { return svc.Close() }

// LogPath 返回日志根目录。
func LogPath() string { return svc.LogPath() }

// LogFilePath 返回当前服务的日志目录。
func LogFilePath() string { return svc.LogFilePath() }

// --- Convenience wrappers（签名与旧实现一致）---

func Info(msg string, context ...map[string]string) error  { return svc.Log("INFO", msg, context...) }
func Error(msg string, context ...map[string]string) error { return svc.Log("ERROR", msg, context...) }
func Warning(msg string, context ...map[string]string) error {
	return svc.Log("WARNING", msg, context...)
}
func Debug(msg string, context ...map[string]string) error { return svc.Log("DEBUG", msg, context...) }
func Notice(msg string, context ...map[string]string) error {
	return svc.Log("NOTICE", msg, context...)
}
func Critical(msg string, context ...map[string]string) error {
	return svc.Log("CRITICAL", msg, context...)
}
func Alert(msg string, context ...map[string]string) error { return svc.Log("ALERT", msg, context...) }
func Emergency(msg string, context ...map[string]string) error {
	return svc.Log("EMERGENCY", msg, context...)
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
