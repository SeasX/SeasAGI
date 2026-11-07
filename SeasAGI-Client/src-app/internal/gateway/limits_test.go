package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReadLimitedBodyRejectsOversize 验证请求体超过上限时返回 413，
// 覆盖 embeddings / images / TTS / STT 等此前无上限的入口（CL-S7）。
func TestReadLimitedBodyRejectsOversize(t *testing.T) {
	const limit = 1024
	body := strings.Repeat("a", limit+1)

	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	w := httptest.NewRecorder()

	got, ok := readLimitedBody(w, r, limit)
	if ok {
		t.Fatal("expected oversize body to be rejected")
	}
	if got != nil {
		t.Fatalf("expected nil body, got %d bytes", len(got))
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestReadLimitedBodyAcceptsWithinLimit(t *testing.T) {
	const limit = 1024
	body := strings.Repeat("a", limit)

	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	w := httptest.NewRecorder()

	got, ok := readLimitedBody(w, r, limit)
	if !ok {
		t.Fatalf("expected body within limit to be accepted, status=%d", w.Code)
	}
	if len(got) != limit {
		t.Fatalf("expected %d bytes, got %d", limit, len(got))
	}
}
