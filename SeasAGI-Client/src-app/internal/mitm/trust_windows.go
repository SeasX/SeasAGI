//go:build windows

package mitm

import (
	"fmt"
	"os/exec"
)

const windowsCertStore = "Root"

type windowsTrustInstaller struct{}

func newPlatformTrustInstaller() TrustInstaller {
	return &windowsTrustInstaller{}
}

// NewTrustInstaller 是平台公共入口，返回当前平台的 TrustInstaller。
func NewTrustInstaller() TrustInstaller {
	return newPlatformTrustInstaller()
}

// Install 使用 certutil 安装 CA 证书到系统根证书存储。
func (w *windowsTrustInstaller) Install(caPEM []byte) error {
	if installed, _ := w.IsInstalled(); installed {
		return nil
	}

	tmpFile, err := writeTempPEM(caPEM)
	if err != nil {
		return fmt.Errorf("write temp PEM: %w", err)
	}
	defer removeTempFile(tmpFile)

	// certutil -addstore -f Root <certfile>
	cmd := exec.Command("certutil", "-addstore", "-f", windowsCertStore, tmpFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("certutil -addstore: %w: %s", err, string(output))
	}
	return nil
}

// Uninstall 使用 certutil 从系统根证书存储卸载 CA 证书。
func (w *windowsTrustInstaller) Uninstall() error {
	// certutil -delstore Root "SeasAGI MITM CA"
	cmd := exec.Command("certutil", "-delstore", windowsCertStore, "SeasAGI MITM CA")
	if output, err := cmd.CombinedOutput(); err != nil {
		// 忽略"未找到"错误（幂等）
		_ = output
	}
	return nil
}

// IsInstalled 检查 CA 证书是否已安装。
func (w *windowsTrustInstaller) IsInstalled() (bool, error) {
	// certutil -store Root "SeasAGI MITM CA"
	cmd := exec.Command("certutil", "-store", windowsCertStore, "SeasAGI MITM CA")
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}
