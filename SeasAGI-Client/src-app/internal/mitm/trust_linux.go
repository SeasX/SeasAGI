//go:build linux

package mitm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const linuxCertPath = "/usr/local/share/ca-certificates/seasagi-mitm.crt"

type linuxTrustInstaller struct{}

func newPlatformTrustInstaller() TrustInstaller {
	return &linuxTrustInstaller{}
}

// NewTrustInstaller 是平台公共入口，返回当前平台的 TrustInstaller。
func NewTrustInstaller() TrustInstaller {
	return newPlatformTrustInstaller()
}

// Install 将 CA 证书复制到系统 CA 目录并运行 update-ca-certificates。
func (l *linuxTrustInstaller) Install(caPEM []byte) error {
	if installed, _ := l.IsInstalled(); installed {
		return nil
	}

	// 写入到系统 CA 目录（需要 root）
	if err := os.WriteFile(linuxCertPath, caPEM, 0o644); err != nil {
		return fmt.Errorf("write CA to system dir: %w", err)
	}

	// 运行 update-ca-certificates
	cmd := exec.Command("update-ca-certificates")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("update-ca-certificates: %w: %s", err, string(output))
	}
	return nil
}

// Uninstall 删除 CA 证书并更新。
func (l *linuxTrustInstaller) Uninstall() error {
	if _, err := os.Stat(linuxCertPath); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(linuxCertPath); err != nil {
		return fmt.Errorf("remove CA file: %w", err)
	}
	cmd := exec.Command("update-ca-certificates", "--fresh")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("update-ca-certificates --fresh: %w: %s", err, string(output))
	}
	return nil
}

// IsInstalled 检查 CA 证书文件是否存在。
func (l *linuxTrustInstaller) IsInstalled() (bool, error) {
	_, err := os.Stat(filepath.Clean(linuxCertPath))
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, nil
}
