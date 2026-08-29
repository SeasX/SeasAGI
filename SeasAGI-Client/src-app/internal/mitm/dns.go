package mitm

// DNSHijacker 是路径 B（DNS 劫持）的扩展接口骨架。
// Phase A5 仅定义接口，不提供实现。后续如需路径 B 可在此接口上扩展。
type DNSHijacker interface {
	// Apply 将指定域名的 DNS 解析劫持到指定 IP。
	Apply(domains []string, ip string) error
	// Remove 移除 DNS 劫持配置。
	Remove() error
	// IsApplied 返回当前是否已生效。
	IsApplied() (bool, error)
}
