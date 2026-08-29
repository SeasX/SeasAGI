package network

// NewSystemProxySetter 根据编译标签返回平台实现的 SystemProxySetter。
// 接口定义在 internal/mitm/types.go，network 包隐式实现（不导入 mitm）。

// noopProxySetter 是无操作的默认实现。
type noopProxySetter struct{}

func (n *noopProxySetter) Set(addr string) error    { return nil }
func (n *noopProxySetter) Clear() error             { return nil }
func (n *noopProxySetter) IsActive() (bool, error)  { return false, nil }
