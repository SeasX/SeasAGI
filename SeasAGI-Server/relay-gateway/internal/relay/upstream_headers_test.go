package relay

import (
	"bytes"
	"net/http"
	"os"
	"testing"
)

// 上游敏感头不得进入客户端响应；白名单外的头默认丢弃。
func TestCopyUpstreamHeadersDropsSensitiveHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "text/event-stream")
	src.Add("Set-Cookie", "upstream_session=secret; HttpOnly")
	src.Set("Authorization", "Bearer upstream-secret")
	src.Set("WWW-Authenticate", "Bearer realm=upstream")
	src.Set("Connection", "keep-alive")
	src.Set("Transfer-Encoding", "chunked")
	src.Set("X-Internal-Debug", "must-not-leak")
	src.Set("X-Request-Id", "req-1")
	src.Set("X-Ratelimit-Remaining-Requests", "42")

	dst := http.Header{}
	copyUpstreamHeaders(dst, src)

	for _, name := range []string{
		"Set-Cookie", "Authorization", "WWW-Authenticate",
		"Connection", "Transfer-Encoding", "X-Internal-Debug",
	} {
		if got := dst.Get(name); got != "" {
			t.Errorf("sensitive header %q was forwarded: %q", name, got)
		}
	}

	for name, want := range map[string]string{
		"Content-Type":                   "text/event-stream",
		"X-Request-Id":                   "req-1",
		"X-Ratelimit-Remaining-Requests": "42",
	} {
		if got := dst.Get(name); got != want {
			t.Errorf("allowed header %q = %q, want %q", name, got, want)
		}
	}
}

// 防止后续改动把上游内部错误文本（可能含主机、路径、密钥）重新回显给客户端。
// 这里做源码级断言，因为触发真实 502 需要构造完整上游与通道上下文。
func TestHandlerDoesNotEchoRawErrors(t *testing.T) {
	data, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("read handler.go: %v", err)
	}
	if bytes.Contains(data, []byte(`"error": err.Error()`)) {
		t.Fatal("relay handler echoes raw upstream errors to clients again")
	}
}
