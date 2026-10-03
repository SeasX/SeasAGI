//go:build darwin

package network

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type darwinProxySetter struct {
	networkService string // "Wi-Fi" / "Ethernet" — 延迟初始化
}

// NewSystemProxySetter 返回 macOS 平台的 SystemProxySetter。
func NewSystemProxySetter() interface {
	Set(addr string) error
	Clear() error
	IsActive() (bool, error)
	CurrentAddr() (string, error)
} {
	return &darwinProxySetter{}
}

// detectNetworkService 枚举并缓存第一个活跃的网络服务名。
func (d *darwinProxySetter) detectNetworkService() (string, error) {
	if d.networkService != "" {
		return d.networkService, nil
	}

	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list network services: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	// 优先选择 Wi-Fi，其次 Ethernet
	priorities := []string{"Wi-Fi", "Ethernet", "USB 10/100/1000 LAN"}
	for _, prio := range priorities {
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, prio) {
				d.networkService = line
				return line, nil
			}
		}
	}
	// 如果没找到优先项，取第一个非星号行
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "An asterisk") {
			d.networkService = line
			return line, nil
		}
	}
	return "", fmt.Errorf("no network service found")
}

func (d *darwinProxySetter) Set(addr string) error {
	service, err := d.detectNetworkService()
	if err != nil {
		return err
	}

	host, port, err := splitHostPort(addr)
	if err != nil {
		return err
	}

	// 设置 HTTP 代理
	if output, err := exec.Command("networksetup", "-setwebproxy", service, host, port).CombinedOutput(); err != nil {
		return fmt.Errorf("setwebproxy: %w: %s", err, string(output))
	}
	// 设置 HTTPS 代理
	if output, err := exec.Command("networksetup", "-setsecurewebproxy", service, host, port).CombinedOutput(); err != nil {
		return fmt.Errorf("setsecurewebproxy: %w: %s", err, string(output))
	}
	return nil
}

func (d *darwinProxySetter) Clear() error {
	service, err := d.detectNetworkService()
	if err != nil {
		return err
	}

	_ = exec.Command("networksetup", "-setwebproxystate", service, "off").Run()
	_ = exec.Command("networksetup", "-setsecurewebproxystate", service, "off").Run()
	return nil
}

func (d *darwinProxySetter) IsActive() (bool, error) {
	service, err := d.detectNetworkService()
	if err != nil {
		return false, err
	}

	cmd := exec.Command("networksetup", "-getwebproxy", service)
	output, err := cmd.Output()
	if err != nil {
		return false, nil
	}
	// 检查输出中是否包含 "Enabled: Yes"
	re := regexp.MustCompile(`(?i)Enabled:\s*Yes`)
	return re.MatchString(string(output)), nil
}

// CurrentAddr 返回当前生效的 HTTP 代理地址（host:port）。
// 代理未启用或无法解析时返回空字符串，便于调用方区分「本客户端的代理」与
// 「用户自有的代理」。
func (d *darwinProxySetter) CurrentAddr() (string, error) {
	service, err := d.detectNetworkService()
	if err != nil {
		return "", err
	}

	output, err := exec.Command("networksetup", "-getwebproxy", service).Output()
	if err != nil {
		return "", nil
	}

	enabled := false
	host, port := "", ""
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Enabled":
			enabled = strings.EqualFold(value, "Yes")
		case "Server":
			host = value
		case "Port":
			port = value
		}
	}
	if !enabled || host == "" || port == "" {
		return "", nil
	}
	return host + ":" + port, nil
}

func splitHostPort(addr string) (string, string, error) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid addr format, expected host:port, got %q", addr)
	}
	return parts[0], parts[1], nil
}
