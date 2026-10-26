package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// TestWSCheckOrigin 验证跨站网页无法借用用户浏览器连接本地网关（CL-S9）。
func TestWSCheckOrigin(t *testing.T) {
	bridge := newWSTestBridge(t)
	check := bridge.upgrader.CheckOrigin

	cases := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{"no origin (cli/sdk)", "127.0.0.1:4318", "", true},
		{"local origin", "127.0.0.1:4318", "http://localhost:3418", true},
		{"loopback origin", "127.0.0.1:4318", "http://127.0.0.1:5173", true},
		{"same host tunnel origin", "abc.trycloudflare.com", "https://abc.trycloudflare.com", true},
		{"cross-site origin", "127.0.0.1:4318", "https://evil.example.net", false},
		{"cross-site with same port", "127.0.0.1:4318", "https://evil.example.net:4318", false},
		{"null origin", "127.0.0.1:4318", "null", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ws", nil)
			r.Host = tc.host
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if got := check(r); got != tc.want {
				t.Fatalf("CheckOrigin(host=%q, origin=%q) = %v, want %v", tc.host, tc.origin, got, tc.want)
			}
		})
	}
}

// TestWSCrossSiteOriginRejected 验证真实握手阶段跨站 Origin 返回 403。
func TestWSCrossSiteOriginRejected(t *testing.T) {
	bridge := newWSTestBridge(t)
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{
		"Authorization": []string{"Bearer " + wsTestToken},
		"Origin":        []string{"https://evil.example.net"},
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		conn.Close()
		t.Fatal("expected cross-site origin handshake to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("expected 403, got %d", status)
	}
}

// TestWSLocalOriginAllowed 验证本机页面（开发服务器）的 Origin 可以完成握手。
func TestWSLocalOriginAllowed(t *testing.T) {
	bridge := newWSTestBridge(t)
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{
		"Authorization": []string{"Bearer " + wsTestToken},
		"Origin":        []string{"http://localhost:3418"},
	}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("expected local origin to be allowed: %v", err)
	}
	conn.Close()
}
