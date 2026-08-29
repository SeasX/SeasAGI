//go:build darwin

package mitm

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const darwinCertLabel = "SeasAGI MITM CA"

type darwinTrustInstaller struct {
	certLabel string
}

func newPlatformTrustInstaller() TrustInstaller {
	return &darwinTrustInstaller{certLabel: darwinCertLabel}
}

// NewTrustInstaller 是平台公共入口，返回当前平台的 TrustInstaller。
func NewTrustInstaller() TrustInstaller {
	return newPlatformTrustInstaller()
}

// Install 将 CA 证书安装到 macOS 系统钥匙串。
// 优先安装到当前用户钥匙串，避免因为系统钥匙串权限导致“已存在旧证书但当前 CA 未被信任”。
func (d *darwinTrustInstaller) Install(caPEM []byte) error {
	fingerprint, err := pemSHA256Fingerprint(caPEM)
	if err != nil {
		return fmt.Errorf("parse CA PEM: %w", err)
	}

	// 仅当同名证书且指纹一致时才认为已安装，避免“旧证书残留”导致误判。
	if installed, _ := d.isInstalledFingerprint(fingerprint); installed {
		return nil
	}

	tmpFile, err := writeTempPEM(caPEM)
	if err != nil {
		return fmt.Errorf("write temp PEM: %w", err)
	}
	defer removeTempFile(tmpFile)

	userKeychain := darwinUserKeychainPath()

	// 同名旧证书会让 StartMITM 误以为已信任，这里先尝试清理。
	_ = d.uninstallFromKeychain(userKeychain)
	_ = d.uninstallFromKeychain("/Library/Keychains/System.keychain")

	// 优先安装到用户钥匙串。macOS 的 native trust / reqwest 通常会读取这里，且不需要管理员权限。
	if err := d.installToKeychain(userKeychain, tmpFile); err != nil {
		return err
	}

	// 尝试同步到 System.keychain；如果没有管理员权限，不影响用户态客户端信任链。
	_ = d.installToKeychain("/Library/Keychains/System.keychain", tmpFile)

	if installed, _ := d.isInstalledFingerprint(fingerprint); !installed {
		return fmt.Errorf("CA certificate install verification failed")
	}
	return nil
}

// Uninstall 从用户/系统钥匙串卸载 CA 证书。
func (d *darwinTrustInstaller) Uninstall() error {
	userKeychain := darwinUserKeychainPath()
	if err := d.uninstallFromKeychain(userKeychain); err != nil {
		return err
	}
	if err := d.uninstallFromKeychain("/Library/Keychains/System.keychain"); err != nil {
		// System.keychain 没有权限时不阻断用户态清理。
		if !isDarwinPermissionErr(err) {
			return err
		}
	}
	return nil
}

// IsInstalled 检查 CA 证书是否已安装在用户或系统钥匙串中。
func (d *darwinTrustInstaller) IsInstalled() (bool, error) {
	userKeychain := darwinUserKeychainPath()
	for _, keychain := range []string{userKeychain, "/Library/Keychains/System.keychain"} {
		cmd := exec.Command("security", "find-certificate", "-a", "-c", d.certLabel, keychain)
		if err := cmd.Run(); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *darwinTrustInstaller) isInstalledFingerprint(fingerprint string) (bool, error) {
	userKeychain := darwinUserKeychainPath()
	for _, keychain := range []string{userKeychain, "/Library/Keychains/System.keychain"} {
		ok, err := darwinKeychainHasFingerprint(keychain, d.certLabel, fingerprint)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (d *darwinTrustInstaller) installToKeychain(keychain, certFile string) error {
	cmd := exec.Command("security", "add-trusted-cert", "-d", "-r", "trustRoot", "-p", "ssl", "-k", keychain, certFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("security add-trusted-cert (%s): %w: %s", keychain, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (d *darwinTrustInstaller) uninstallFromKeychain(keychain string) error {
	cmd := exec.Command("security", "delete-certificate", "-c", d.certLabel, keychain)
	if output, err := cmd.CombinedOutput(); err != nil {
		out := string(output)
		if strings.Contains(out, "could not be found") || strings.Contains(out, "The specified item could not be found") {
			return nil
		}
		if isDarwinPermissionErr(fmt.Errorf("%w: %s", err, out)) {
			return fmt.Errorf("security delete-certificate (%s): %w: %s", keychain, err, strings.TrimSpace(out))
		}
		return fmt.Errorf("security delete-certificate (%s): %w: %s", keychain, err, strings.TrimSpace(out))
	}
	return nil
}

func darwinKeychainHasFingerprint(keychain, certLabel, fingerprint string) (bool, error) {
	cmd := exec.Command("security", "find-certificate", "-a", "-c", certLabel, "-Z", keychain)
	output, err := cmd.CombinedOutput()
	if err != nil {
		out := string(output)
		if strings.Contains(out, "could not be found") || strings.TrimSpace(out) == "" {
			return false, nil
		}
		return false, fmt.Errorf("security find-certificate (%s): %w: %s", keychain, err, strings.TrimSpace(out))
	}
	return strings.Contains(strings.ToUpper(string(output)), strings.ToUpper(fingerprint)), nil
}

func pemSHA256Fingerprint(caPEM []byte) (string, error) {
	block, _ := pem.Decode(caPEM)
	if block == nil {
		return "", fmt.Errorf("decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	sum := sha256.Sum256(cert.Raw)
	return fmt.Sprintf("%X", sum[:]), nil
}

func darwinUserKeychainPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	}
	return "login.keychain-db"
}

func isDarwinPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "user interaction is not allowed") ||
		strings.Contains(text, "authorization") ||
		strings.Contains(text, "auth failed") ||
		strings.Contains(text, "permission denied")
}
