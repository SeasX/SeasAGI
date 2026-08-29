package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSConnect(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	if bridge.ActiveConnections() != 1 {
		t.Errorf("Expected 1 active connection, got %d", bridge.ActiveConnections())
	}
}

func TestWSPingPong(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

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

func TestWSChatRequest(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

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
	if resp.Status != 200 {
		t.Errorf("Expected status 200, got %d", resp.Status)
	}

	// Verify response data
	var chatResp map[string]interface{}
	if err := json.Unmarshal(resp.Data, &chatResp); err != nil {
		t.Fatalf("Failed to unmarshal chat response: %v", err)
	}
	if chatResp["object"] != "chat.completion" {
		t.Errorf("Expected object 'chat.completion', got '%v'", chatResp["object"])
	}
}

func TestWSModelsRequest(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send models request
	msg := WSMessage{Type: "models", RequestID: "models-1"}
	conn.WriteJSON(msg)

	// Read response
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
}

func TestWSInvalidMessage(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

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
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

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
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Close connection
	conn.Close()

	// Give server time to process disconnect
	time.Sleep(100 * time.Millisecond)

	if bridge.ActiveConnections() != 0 {
		t.Errorf("Expected 0 active connections after disconnect, got %d", bridge.ActiveConnections())
	}
}

func TestWSCloseAll(t *testing.T) {
	bridge := NewWSBridge()
	server := httptest.NewServer(http.HandlerFunc(bridge.HandleWS))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Open 3 connections
	conns := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
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
