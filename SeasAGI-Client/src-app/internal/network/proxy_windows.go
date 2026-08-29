//go:build windows

package network

import (
	"fmt"
	"os/exec"
	"strings"
)

type windowsProxySetter struct{}

// NewSystemProxySetter 返回 Windows 平台的 SystemProxySetter。
func NewSystemProxySetter() interface {
	Set(addr string) error
	Clear() error
	IsActive() (bool, error)
} {
	return &windowsProxySetter{}
}

func (w *windowsProxySetter) Set(addr string) error {
	// 通过 reg 命令设置注册表
	// HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings
	regBase := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`

	// 设置 ProxyEnable=1
	if output, err := exec.Command("reg", "add", regBase, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("reg add ProxyEnable: %w: %s", err, string(output))
	}
	// 设置 ProxyServer
	if output, err := exec.Command("reg", "add", regBase, "/v", "ProxyServer", "/t", "REG_SZ", "/d", addr, "/f").CombinedOutput(); err != nil {
		return fmt.Errorf("reg add ProxyServer: %w: %s", err, string(output))
	}
	return nil
}

func (w *windowsProxySetter) Clear() error {
	regBase := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	_ = exec.Command("reg", "add", regBase, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f").Run()
	return nil
}

func (w *windowsProxySetter) IsActive() (bool, error) {
	cmd := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyEnable")
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(output), "0x1"), nil
}
