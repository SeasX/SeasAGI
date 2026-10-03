//go:build linux

package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type linuxProxySetter struct{}

// NewSystemProxySetter 返回 Linux 平台的 SystemProxySetter。
// 仅支持 GNOME 桌面环境，其他环境返回明确错误。
func NewSystemProxySetter() interface {
	Set(addr string) error
	Clear() error
	IsActive() (bool, error)
	CurrentAddr() (string, error)
} {
	return &linuxProxySetter{}
}

func (l *linuxProxySetter) Set(addr string) error {
	if !isGNOME() {
		return fmt.Errorf("unsupported desktop environment: only GNOME is supported")
	}

	host, port, err := splitHostPortLinux(addr)
	if err != nil {
		return err
	}

	// gsettings org.gnome.system.proxy mode 'manual'
	if output, err := exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "manual").CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set mode: %w: %s", err, string(output))
	}
	// 设置 host
	if output, err := exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "host", host).CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set http host: %w: %s", err, string(output))
	}
	// 设置 port
	if output, err := exec.Command("gsettings", "set", "org.gnome.system.proxy.http", "port", port).CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set http port: %w: %s", err, string(output))
	}
	// https host/port
	if output, err := exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "host", host).CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set https host: %w: %s", err, string(output))
	}
	if output, err := exec.Command("gsettings", "set", "org.gnome.system.proxy.https", "port", port).CombinedOutput(); err != nil {
		return fmt.Errorf("gsettings set https port: %w: %s", err, string(output))
	}
	return nil
}

func (l *linuxProxySetter) Clear() error {
	if !isGNOME() {
		return nil
	}
	_ = exec.Command("gsettings", "set", "org.gnome.system.proxy", "mode", "none").Run()
	return nil
}

func (l *linuxProxySetter) IsActive() (bool, error) {
	if !isGNOME() {
		return false, nil
	}
	cmd := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode")
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return string(output) == "'manual'\n", nil
}

func isGNOME() bool {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	return desktop == "GNOME" || desktop == "ubuntu:GNOME"
}

// CurrentAddr 返回当前生效的 HTTP 代理地址（host:port）。
// 未启用或无法解析时返回空字符串，便于调用方区分「本客户端的代理」与
// 「用户自有的代理」。
func (l *linuxProxySetter) CurrentAddr() (string, error) {
	if !isGNOME() {
		return "", nil
	}
	host, err := gsettingsGet("org.gnome.system.proxy.http", "host")
	if err != nil || host == "" {
		return "", nil
	}
	port, err := gsettingsGet("org.gnome.system.proxy.http", "port")
	if err != nil || port == "" {
		return "", nil
	}
	return host + ":" + port, nil
}

// gsettingsGet 读取 gsettings 键值并去掉输出外层引号与空白。
func gsettingsGet(schema, key string) (string, error) {
	output, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return "", err
	}
	return strings.Trim(strings.TrimSpace(string(output)), "'"), nil
}

func splitHostPortLinux(addr string) (string, string, error) {
	parts := filepath.SplitList(addr)
	_ = parts
	// 简单分割
	for i, c := range addr {
		if c == ':' {
			return addr[:i], addr[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("invalid addr format: %q", addr)
}
