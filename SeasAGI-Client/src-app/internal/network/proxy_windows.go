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
	CurrentAddr() (string, error)
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

// CurrentAddr 返回当前生效的代理地址（host:port）。
// 未配置或无法解析时返回空字符串，便于调用方区分「本客户端的代理」与
// 「用户自有的代理」。
func (w *windowsProxySetter) CurrentAddr() (string, error) {
	cmd := exec.Command("reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`, "/v", "ProxyServer")
	output, err := cmd.Output()
	if err != nil {
		return "", nil
	}
	// 输出形如: "    ProxyServer    REG_SZ    127.0.0.1:8080"
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && strings.EqualFold(fields[0], "ProxyServer") {
			return fields[len(fields)-1], nil
		}
	}
	return "", nil
}
