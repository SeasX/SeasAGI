// Package mitm 提供 MITM 透明代理能力，通过本地 CA 签发域名证书拦截 AI API 流量并转发到本地网关。
// 本文件定义全包共享的类型与接口骨架（Phase A0），具体实现在后续 Phase 中补充。
package mitm

import "time"

// State 表示 MITM 运行状态机。
type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateError    State = "error"
)

// Status 对外暴露的运行时状态快照，可被 JSON 序列化后传递给前端。
type Status struct {
	State State `json:"state"`
	// ProxyPort 本地 MITM 代理监听端口。
	ProxyPort int `json:"proxy_port"`
	// CAInstalled 表示本地 CA（ca.pem / ca-key.pem）已生成或加载成功。
	// 注意：它不代表 CA 已被操作系统信任，后者见 CATrusted。
	CAInstalled bool `json:"ca_installed"`
	// CATrusted 表示 CA 已成功安装到系统信任库并通过信任校验。
	CATrusted bool `json:"ca_trusted"`
	// RulesCount 当前生效的拦截域名数量。
	RulesCount int `json:"rules_count"`
	// SystemProxy 表示 SeasAGI 已成功设置系统代理指向本地 MITM。
	SystemProxy bool `json:"system_proxy"`
	// SystemProxyOwned 表示当前生效的系统代理是否指向本客户端（用于区分用户自有代理）。
	SystemProxyOwned bool   `json:"system_proxy_owned"`
	LastError        string `json:"last_error,omitempty"`
}

// InterceptEntry 拦截日志单条记录。
type InterceptEntry struct {
	Time        time.Time `json:"time"`
	Method      string    `json:"method"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	Intercepted bool      `json:"intercepted"`
	DurationMs  float64   `json:"duration_ms"`
}

// StartupStep 定义启动步骤顺序（正序启动，逆序回滚）。
var StartupStep = []string{"ca", "trust", "proxy", "system_proxy"}

// TrustInstaller CA 证书信任安装抽象。Phase A2 提供三平台实现。
type TrustInstaller interface {
	Install(caPEM []byte) error
	Uninstall() error
	IsInstalled() (bool, error)
}

// SystemProxySetter 系统代理设置抽象。
// 注意：此接口在 mitm 包定义，network 包的实现不需要导入 mitm 包（Go 隐式接口）。
type SystemProxySetter interface {
	Set(addr string) error
	Clear() error
	IsActive() (bool, error)
	// CurrentAddr 返回当前生效的系统代理地址（host:port）。
	// 未启用或不可读取时返回空字符串，便于调用方区分「本客户端的代理」与
	// 「用户自有的代理」，避免误清用户配置。
	CurrentAddr() (string, error)
}

// InterceptLogger 拦截日志抽象。Phase A4 提供环形缓冲实现。
type InterceptLogger interface {
	Log(entry InterceptEntry)
	Recent(n int) []InterceptEntry
}
