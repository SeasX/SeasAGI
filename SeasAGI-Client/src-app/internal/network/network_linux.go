package network

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isSystemAwake() bool {
	out, err := exec.Command("systemctl", "is-active", "systemd-logind").Output()
	if err != nil {
		return true
	}
	return strings.TrimSpace(string(out)) == "active"
}

func captureNetworkID() string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return ""
	}
	ids := make([]string, 0)
	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		ids = append(ids, name)
	}
	return strings.Join(ids, ",")
}

func SetAutoLaunch(enabled bool) error {
	home, _ := os.UserHomeDir()
	autostartDir := filepath.Join(home, ".config", "autostart")
	desktopPath := filepath.Join(autostartDir, "seasagi.desktop")

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}

		if err := os.MkdirAll(autostartDir, 0755); err != nil {
			return err
		}

		desktop := `[Desktop Entry]
Type=Application
Name=SeasAGI
Exec=` + exePath + `
X-GNOME-Autostart-enabled=true
Terminal=false
`
		return os.WriteFile(desktopPath, []byte(desktop), 0644)
	}

	_ = os.Remove(desktopPath)
	return nil
}