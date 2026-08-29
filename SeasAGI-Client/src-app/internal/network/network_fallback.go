//go:build !darwin && !linux && !windows

package network

func isSystemAwake() bool {
	return true
}

func captureNetworkID() string {
	return ""
}

func SetAutoLaunch(enabled bool) error {
	return nil
}
