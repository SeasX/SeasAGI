//go:build !darwin && !linux && !windows

package mitm

// NewTrustInstaller 在不支持的平台返回 noop 实现。
func NewTrustInstaller() TrustInstaller {
	return &noopTrustInstaller{}
}
