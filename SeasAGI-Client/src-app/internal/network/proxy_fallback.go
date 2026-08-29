//go:build !darwin && !linux && !windows

package network

// NewSystemProxySetter 在不支持的平台返回 noop 实现。
func NewSystemProxySetter() interface {
	Set(addr string) error
	Clear() error
	IsActive() (bool, error)
} {
	return &noopProxySetter{}
}
