package relay

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// JSON 入口超限时必须返回 413，不允许无上限读入内存。
func TestReadJSONBodyRejectsOversizedBody(t *testing.T) {
	oldLimit := maxJSONBodyBytes
	maxJSONBodyBytes = 16
	defer func() { maxJSONBodyBytes = oldLimit }()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/relay/chat/completions", strings.NewReader(strings.Repeat("a", 64)))

	if _, ok := readJSONBody(c); ok {
		t.Fatal("expected oversized body to be rejected")
	}
	if w.Code != 413 {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

// multipart 入口同样受大小上限约束。
func TestReadMultipartFormRejectsOversizedBody(t *testing.T) {
	oldLimit := maxMultipartBodyBytes
	maxMultipartBodyBytes = 64
	defer func() { maxMultipartBodyBytes = oldLimit }()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("model", "whisper-1")
	_ = writer.WriteField("padding", strings.Repeat("x", 256))
	_ = writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/relay/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	if _, ok := readMultipartForm(c); ok {
		t.Fatal("expected oversized multipart body to be rejected")
	}
	if w.Code != 413 {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

// 正常 multipart 请求应可解析。
func TestReadMultipartFormAcceptsValidBody(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("model", "whisper-1")
	_ = writer.Close()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("POST", "/relay/audio/transcriptions", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	c.Request = req

	form, ok := readMultipartForm(c)
	if !ok {
		t.Fatalf("expected multipart form to parse, status=%d", w.Code)
	}
	if got := form.Value["model"]; len(got) != 1 || got[0] != "whisper-1" {
		t.Fatalf("unexpected form values: %+v", form.Value)
	}
}

// 流式转发必须逐帧写入并 flush，避免首字延迟。
func TestCopyStreamWritesAndFlushes(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	payload := "data: {\"delta\":\"hi\"}\n\ndata: [DONE]\n\n"
	copyStream(c, strings.NewReader(payload))

	if w.Body.String() != payload {
		t.Fatalf("stream body = %q, want %q", w.Body.String(), payload)
	}
	if !w.Flushed {
		t.Fatal("expected stream to be flushed")
	}
}

// 流式响应不应设置覆盖整个响应体的总超时，只限制响应头等待时间。
func TestUpstreamClientHasNoOverallTimeout(t *testing.T) {
	if upstreamClient.Timeout != 0 {
		t.Fatalf("upstreamClient.Timeout = %v, want 0 (only header timeout)", upstreamClient.Timeout)
	}
	transport, ok := upstreamClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type %T", upstreamClient.Transport)
	}
	if transport.ResponseHeaderTimeout == 0 {
		t.Fatal("expected ResponseHeaderTimeout to be set")
	}
}
