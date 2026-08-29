package mitm

import (
	"bufio"
	"io"
	"strings"
)

// newBufReader 创建带 8KB 缓冲的 bufio.Reader，用于从 TLS 连接读取 HTTP 请求。
func newBufReader(conn io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(conn, 8192)
}

// stringReader 从字符串创建 io.Reader，用于构造错误响应体。
func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}
