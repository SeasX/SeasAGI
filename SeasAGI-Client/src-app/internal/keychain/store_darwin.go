//go:build darwin

package keychain

import (
	"fmt"
	"os/exec"
	"strings"
)

// macOS：沿用系统 Keychain（security CLI）。系统 Keychain 已由操作系统加密，
// 无需额外落盘逻辑。

func platformGet(keyID string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-a", keyID, "-s", serviceName, "-w")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read keychain entry %s: %w", keyID, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func platformSave(keyID, secret string) error {
	_ = platformDelete(keyID)
	cmd := exec.Command("security", "add-generic-password", "-U", "-a", keyID, "-s", serviceName, "-w", secret)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("save keychain entry %s: %s (%w)", keyID, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func platformDelete(keyID string) error {
	cmd := exec.Command("security", "delete-generic-password", "-a", keyID, "-s", serviceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(output))
		if strings.Contains(msg, "could not be found") {
			return nil
		}
		return fmt.Errorf("delete keychain entry %s: %s (%w)", keyID, msg, err)
	}
	return nil
}
