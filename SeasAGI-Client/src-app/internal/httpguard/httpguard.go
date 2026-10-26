package httpguard

import (
	"net"
	"net/http"
	"strings"
)

// maxBodyBytes 限制本地 HTTP 入口的请求体，避免单个请求打满内存。
const maxBodyBytes = 1 << 20

// LocalOnly 只允许回环地址访问，并校验 Host/Origin，防止任意网页借浏览器发起跨域调用。
// 仅用于绑定 127.0.0.1 的本地服务（WebUI / A2A）。
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalHost(r.Host) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			if !isLocalOrigin(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}
		if r.Body != nil {
			if r.ContentLength > maxBodyBytes {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalHost 判断 Host 是否为 localhost/127.0.0.1/[::1]（允许携带端口）。
func isLocalHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// IsLocalOrigin 判断 Origin 是否指向本机（允许 http/https 与端口）。
// 供 WebSocket 等需要复用同一判定逻辑的入口使用。
func IsLocalOrigin(origin string) bool {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return isLocalHost(trimmed)
}

// isLocalOrigin 判断 Origin 是否指向本机（允许 http/https 与端口）。
func isLocalOrigin(origin string) bool {
	return IsLocalOrigin(origin)
}
