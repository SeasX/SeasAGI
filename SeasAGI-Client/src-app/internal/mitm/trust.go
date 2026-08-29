package mitm

// NewTrustInstaller 根据编译标签返回平台实现的 TrustInstaller。
// 三个平台文件（trust_darwin.go / trust_linux.go / trust_windows.go）各自定义 newPlatformTrustInstaller()。
// 此文件定义 NewTrustInstaller 公共入口 + 不支持平台的 noop 回退。

// noopTrustInstaller 是无操作的默认实现，用于不支持的平台或测试环境。
type noopTrustInstaller struct{}

func (n *noopTrustInstaller) Install(caPEM []byte) error { return nil }
func (n *noopTrustInstaller) Uninstall() error           { return nil }
func (n *noopTrustInstaller) IsInstalled() (bool, error) { return false, nil }
