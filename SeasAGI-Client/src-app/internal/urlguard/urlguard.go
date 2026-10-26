package urlguard

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ValidateSecureURL 强制出站 URL 使用 HTTPS。
// 仅当 allowLoopbackHTTP 为真时，允许 http://localhost、http://127.0.0.1、http://[::1] 等本机地址，
// 以兼容本地模型服务（Ollama/vLLM）与本地企业部署。
func ValidateSecureURL(raw string, allowLoopbackHTTP bool) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.User != nil {
		return fmt.Errorf("url must not contain userinfo")
	}
	if u.Host == "" {
		return fmt.Errorf("url has no host")
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if allowLoopbackHTTP && IsLoopbackHost(u.Host) {
			return nil
		}
		return fmt.Errorf("refusing non-https url: %s", u.Host)
	default:
		return fmt.Errorf("unsupported url scheme %q", u.Scheme)
	}
}

// IsLoopbackHost 判断 host[:port] 是否指向本机回环地址。
func IsLoopbackHost(hostport string) bool {
	host := HostOnly(hostport)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// HostOnly 去掉端口与 IPv6 方括号，返回纯主机部分。
func HostOnly(hostport string) string {
	host := strings.TrimSpace(hostport)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.Trim(host, "[]")
}

// RejectLinkLocalHost 拒绝链路本地/未指定/组播目标（含云元数据 169.254.169.254），
// 允许回环与私网地址。用于「带凭证、目标由用户配置」的出站请求，防止被当作 SSRF 跳板。
func RejectLinkLocalHost(hostport string) error {
	host := HostOnly(hostport)
	if host == "" {
		return fmt.Errorf("target host is empty")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("refusing link-local or non-routable target: %s", host)
	}
	return nil
}

// SecureRedirect 是携带凭证的出站请求统一重定向策略：
// 最多 5 跳；禁止 https→http 降级；跨主机时剥离凭证头，避免 Key 被 30x 带到其它主机。
func SecureRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after 5 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	prev := via[len(via)-1].URL
	if strings.EqualFold(prev.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return fmt.Errorf("refusing redirect downgrade to %s", req.URL.Scheme)
	}
	if !strings.EqualFold(prev.Hostname(), req.URL.Hostname()) {
		req.Header.Del("Authorization")
		req.Header.Del("X-Api-Key")
		req.Header.Del("X-Goog-Api-Key")
	}
	return nil
}

// ValidateProxyTarget 校验 MITM 代理的 CONNECT 目标：禁止回环、链路本地、未指定与组播。
// 私网地址保留，以支持企业内网 API。
func ValidateProxyTarget(hostport string) error {
	host := HostOnly(hostport)
	if host == "" {
		return fmt.Errorf("proxy target host is empty")
	}
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("refusing proxy target %q", host)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return fmt.Errorf("refusing proxy target %s", host)
	}
	return nil
}
