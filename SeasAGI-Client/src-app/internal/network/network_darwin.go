package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func isSystemAwake() bool {
	out, err := exec.Command("pmset", "-g", "powerstate", "IODisplayWrangler").Output()
	if err != nil {
		return true
	}
	state := strings.TrimSpace(string(out))
	return state != "0"
}

func captureNetworkID() string {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", len(out))
}

func SetAutoLaunch(enabled bool) error {
	home, _ := os.UserHomeDir()
	plistPath := filepath.Join(home, "Library", "LaunchAgents", "com.seasagi.desktop.plist")

	if enabled {
		exePath, err := os.Executable()
		if err != nil {
			return err
		}
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key><string>com.seasagi.desktop</string>
    <key>ProgramArguments</key><array><string>%s</string></array>
    <key>RunAtLoad</key><true/>
</dict>
</plist>`, exePath)
		return os.WriteFile(plistPath, []byte(plist), 0644)
	}

	_ = os.Remove(plistPath)
	return nil
}
