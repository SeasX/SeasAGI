package mitm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleConnectRejectsLocalTargets 验证代理不会成为访问本机回环或云元数据的跳板（CL-S18）。
func TestHandleConnectRejectsLocalTargets(t *testing.T) {
	proxy := NewProxy("127.0.0.1:0", nil, nil, "http://127.0.0.1:4318", nil)

	for _, target := range []string{
		"127.0.0.1:4318",
		"localhost:4318",
		"[::1]:4318",
		"169.254.169.254:80",
		"[fe80::1]:80",
		"0.0.0.0:80",
	} {
		r := httptest.NewRequest(http.MethodConnect, "http://"+target, nil)
		r.Host = target
		w := httptest.NewRecorder()

		proxy.handleConnect(w, r)

		if w.Code != http.StatusForbidden {
			t.Errorf("target %q: expected 403, got %d", target, w.Code)
		}
	}
}
