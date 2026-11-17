package relay

import (
	"net/http"
	"strings"
)

// 上游响应头白名单。此前是 `for k, v := range resp.Header` 全量转发，
// 会把上游的 Set-Cookie、Authorization、WWW-Authenticate 以及 hop-by-hop 头
// 原样写到客户端域，造成会话头污染与内部信息泄露（P1-10）。
//
// 采用白名单而非黑名单：新增的上游头默认不转发，需要时显式加入。
// Content-Length 与 Transfer-Encoding 由 Go 的响应写入器自行处理，
// 复制它们会与流式/chunked 响应冲突，因此刻意排除。
func copyUpstreamHeaders(dst, src http.Header) {
	for name, values := range src {
		if !allowUpstreamHeader(name) {
			continue
		}
		for _, v := range values {
			dst.Add(name, v)
		}
	}
}

func allowUpstreamHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Content-Type", "Cache-Control", "ETag", "Last-Modified",
		"Expires", "Vary", "X-Request-Id", "Retry-After":
		return true
	}
	// OpenAI 兼容上游的限流可见性头（大小写经 CanonicalHeaderKey 归一后判断）。
	return strings.HasPrefix(strings.ToLower(http.CanonicalHeaderKey(name)), "x-ratelimit-")
}
