package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/SeasAGI/SeasAGI-Client/internal/config"
	"github.com/SeasAGI/SeasAGI-Client/internal/logs"
	"github.com/SeasAGI/SeasAGI-Client/internal/routing"
	"github.com/SeasAGI/SeasAGI-Client/internal/usage"
)

const wsTestToken = "ws-test-token"

// newWSTestBridge 构造一个最小可用、已绑定网关服务的 WebSocket 桥接。
func newWSTestBridge(t *testing.T) *WSBridge {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfgSvc := config.NewService(config.AppConfig{})
	return NewWSBridge(&Service{
		accessToken: wsTestToken,
		configSvc:   cfgSvc,
		logSvc:      logs.NewService(),
		usageSvc:    usage.NewService(),
		security:    newSecurityGuard(),
		resolver:    routing.NewResolver(cfgSvc),
		rateLimiter: newRateLimiter(),
	})
}

func wsDial(t *testing.T, bridge *WSBridge) (*websocket.Conn, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{"Authorization": []string{"Bearer " + wsTestToken}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		server.Close()
		t.Fatalf("Failed to connect: %v", err)
	}
	return conn, func() {
		conn.Close()
		server.Close()
	}
}

func TestWSConnect(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	if bridge.ActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", bridge.ActiveConnections())
	}
	_ = conn
}

// TestWSRequiresToken 验证 WebSocket 入口与 HTTP 入口一样需要访问令牌，
// 避免未经鉴权的连接绕过网关治理。
func TestWSRequiresToken(t *testing.T) {
	bridge := newWSTestBridge(t)
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	if _, resp, err := websocket.DefaultDialer.Dial(wsURL, nil); err == nil {
		t.Fatal("expected dial without token to fail")
	} else if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}

	// ?token= 查询参数同样可用（浏览器 WebSocket API 无法自定义请求头）
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?token="+wsTestToken, nil)
	if err != nil {
		t.Fatalf("expected token query param to authorize: %v", err)
	}
	conn.Close()
}

func TestWSPingPong(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	// Send ping
	ping := WSMessage{Type: "ping", RequestID: "test-1"}
	conn.WriteJSON(ping)

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp WSResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.Type != "pong" {
		t.Errorf("Expected type 'pong', got '%s'", resp.Type)
	}
	if resp.RequestID != "test-1" {
		t.Errorf("Expected request_id 'test-1', got '%s'", resp.RequestID)
	}
}

// TestWSChatRequestGoesThroughPipeline 验证 WS chat 走真实主链路：
// 未配置任何渠道时必须返回路由错误，而不是桩数据。
func TestWSChatRequestGoesThroughPipeline(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	// Send chat request
	chatReq := map[string]interface{}{
		"model":    "gpt-4",
		"messages": []map[string]interface{}{{"role": "user", "content": "Hello"}},
	}
	reqBytes, _ := json.Marshal(chatReq)
	msg := WSMessage{Type: "chat", RequestID: "chat-1", Request: reqBytes}
	conn.WriteJSON(msg)

	// Read response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp WSResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.Type != "chat" {
		t.Errorf("Expected type 'chat', got '%s'", resp.Type)
	}
	if resp.RequestID != "chat-1" {
		t.Errorf("Expected request_id 'chat-1', got '%s'", resp.RequestID)
	}
	// 无可用渠道 -> 主链路返回路由错误（400），而非之前的桩响应
	if resp.Status != http.StatusBadRequest {
		t.Errorf("Expected status 400 from routing error, got %d", resp.Status)
	}
	if strings.Contains(string(resp.Data), "bridge active") {
		t.Errorf("WS chat must not return stub data, got %s", string(resp.Data))
	}
	if !strings.Contains(string(resp.Data), "no enabled channel") {
		t.Errorf("Expected routing error body, got %s", string(resp.Data))
	}
}

// TestWSModelsRequest 验证 WS models 返回真实（可空的）模型列表。
func TestWSModelsRequest(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	msg := WSMessage{Type: "models", RequestID: "models-1"}
	conn.WriteJSON(msg)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp WSResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.Type != "models" {
		t.Errorf("Expected type 'models', got '%s'", resp.Type)
	}
	if resp.Status != 200 {
		t.Errorf("Expected status 200, got %d", resp.Status)
	}
	var list map[string]interface{}
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		t.Fatalf("Failed to unmarshal models response: %v", err)
	}
	if list["object"] != "list" {
		t.Errorf("Expected object 'list', got '%v'", list["object"])
	}
}

func TestWSInvalidMessage(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	// Send invalid JSON
	conn.WriteMessage(websocket.TextMessage, []byte("not json"))

	// Read error response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp WSResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.Type != "error" {
		t.Errorf("Expected type 'error', got '%s'", resp.Type)
	}
}

func TestWSUnknownType(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	// Send unknown type
	msg := WSMessage{Type: "unknown_type", RequestID: "unk-1"}
	conn.WriteJSON(msg)

	// Read error response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp WSResponse
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.Type != "error" {
		t.Errorf("Expected type 'error', got '%s'", resp.Type)
	}
	if resp.RequestID != "unk-1" {
		t.Errorf("Expected request_id 'unk-1', got '%s'", resp.RequestID)
	}
}

func TestWSDisconnect(t *testing.T) {
	bridge := newWSTestBridge(t)
	conn, cleanup := wsDial(t, bridge)
	defer cleanup()

	// Close connection
	conn.Close()

	// Give server time to process disconnect
	time.Sleep(100 * time.Millisecond)

	if bridge.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after disconnect, got %d", bridge.ActiveConnections())
	}
}

func TestWSCloseAll(t *testing.T) {
	bridge := newWSTestBridge(t)
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	header := http.Header{"Authorization": []string{"Bearer " + wsTestToken}}

	// Open 3 connections
	conns := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
		if err != nil {
			t.Fatalf("Failed to connect %d: %v", i, err)
		}
		conns[i] = conn
	}

	if bridge.ActiveConnections() != 3 {
		t.Errorf("Expected 3 active connections, got %d", bridge.ActiveConnections())
	}

	// Close all
	bridge.CloseAll()

	if bridge.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after CloseAll, got %d", bridge.ActiveConnections())
	}
}
