//go:build !darwin

package tray

func runPlatformTray(icon []byte) {
}

func stopPlatformTray() {
}

func rebuildPlatformMenu(showFn, quitFn func(), channels []ChannelInfo, onSwitch func(string)) {
}

func setActiveChannel(id string) {
}

func setReopenHandler(fn func()) {
}
