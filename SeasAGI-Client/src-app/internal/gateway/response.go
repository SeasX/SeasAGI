package gateway

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

func (s *Service) validateToken(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if len(auth) <= 7 || !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	return s.tokenMatches(strings.TrimSpace(auth[7:]))
}

// tokenMatches 以常量时间比较访问令牌，供 HTTP 与 WebSocket 入口共用。
func (s *Service) tokenMatches(token string) bool {
	if token == "" {
		return false
	}
	s.mu.RLock()
	currentToken := s.accessToken
	s.mu.RUnlock()
	return subtle.ConstantTimeCompare([]byte(token), []byte(currentToken)) == 1
}

// ServeGatewayRequest 在进程内复用 HTTP 主链路处理一次请求，返回（状态码, 响应体）。
// WebSocket 桥接通过它复用与 HTTP 完全一致的鉴权 / 限流 / DLP / 路由 / 转发 / 记账逻辑，
// 避免 WebSocket 成为绕过治理的旁路。
func (s *Service) ServeGatewayRequest(path string, body []byte, token string) (int, []byte) {
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return http.StatusBadRequest, []byte(`{"error":{"message":"invalid ws request"}}`)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	rec := &bodyRecorder{header: make(http.Header)}
	switch path {
	case "/v1/chat/completions":
		s.handleChatCompletions(rec, req)
	case "/v1/models":
		s.handleListModels(rec, req)
	default:
		return http.StatusNotFound, []byte(`{"error":{"message":"unsupported websocket message type"}}`)
	}
	return rec.snapshot()
}

// bodyRecorder 把 handler 的响应收集到内存，供进程内复用 HTTP 链路时取回结果。
type bodyRecorder struct {
	header http.Header
	status int
	buf    bytes.Buffer
}

func (r *bodyRecorder) Header() http.Header { return r.header }

func (r *bodyRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *bodyRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.buf.Write(p)
}

// Flush 让 bodyRecorder 满足 http.Flusher（流式 handler 会调用），此处仅累加不实时下发。
func (r *bodyRecorder) Flush() {}

func (r *bodyRecorder) snapshot() (int, []byte) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.status, r.buf.Bytes()
}

// releaseOnCloseBody 在响应体关闭（流结束）时归还渠道并发槽位。流式请求的 Body
// 会一直存活到客户端读完，槽位也随之一直占用，使 MaxConcurrent 对流式真正生效（CL-15）。
type releaseOnCloseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *releaseOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

// readLimitedBody 读取带大小上限的请求体；超限返回 413 且 ok=false（CL-13）。
func readLimitedBody(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "Request body too large")
			return nil, false
		}
		writeJSONError(w, http.StatusBadRequest, "Failed to read request body")
		return nil, false
	}
	return body, true
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    status,
		},
	})
}
