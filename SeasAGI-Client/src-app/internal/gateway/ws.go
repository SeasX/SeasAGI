package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSBridge handles WebSocket connections for real-time chat.
// It translates WebSocket messages into the same format as the HTTP API,
// providing a persistent connection alternative to HTTP polling.
type WSBridge struct {
	upgrader websocket.Upgrader
	mu       sync.RWMutex
	conns    map[*websocket.Conn]bool
}

// NewWSBridge creates a new WebSocket bridge.
func NewWSBridge() *WSBridge {
	return &WSBridge{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin for local proxy usage
				return true
			},
		},
		conns: make(map[*websocket.Conn]bool),
	}
}

// WSMessage is the message format exchanged over WebSocket.
// It mirrors the HTTP API request format for consistency.
type WSMessage struct {
	Type      string          `json:"type"`       // "chat", "models", "ping"
	Request   json.RawMessage `json:"request"`    // The actual API request body
	RequestID string          `json:"request_id"` // Client-generated ID for response correlation
}

// WSResponse is the response sent back over WebSocket.
type WSResponse struct {
	Type      string          `json:"type"`       // "chat", "chat_chunk", "models", "pong", "error"
	RequestID string          `json:"request_id"` // Matches the request ID
	Status    int             `json:"status"`     // HTTP status code equivalent
	Data      json.RawMessage `json:"data"`       // Response data
	Error     string          `json:"error,omitempty"`
}

// HandleWS handles a WebSocket connection upgrade and message loop.
func (wb *WSBridge) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wb.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	wb.mu.Lock()
	wb.conns[conn] = true
	wb.mu.Unlock()

	defer func() {
		wb.mu.Lock()
		delete(wb.conns, conn)
		wb.mu.Unlock()
	}()

	// Set read deadline for idle connections
	conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})

	// Start ping ticker
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			ticker.Stop()
			return
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg WSMessage
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			wb.sendError(conn, "", fmt.Sprintf("invalid message format: %v", err))
			continue
		}

		switch msg.Type {
		case "ping":
			wb.sendMessage(conn, WSResponse{
				Type:      "pong",
				RequestID: msg.RequestID,
				Status:    200,
			})
		case "chat":
			// Forward to the gateway service's chat handler
			wb.handleChatRequest(conn, msg)
		case "models":
			wb.handleModelsRequest(conn, msg)
		default:
			wb.sendError(conn, msg.RequestID, fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}
}

// handleChatRequest processes a chat completion request received over WebSocket.
// In the actual integration, this would forward to the gateway's chat handler.
// For now, it echoes back a formatted response for testing.
func (wb *WSBridge) handleChatRequest(conn *websocket.Conn, msg WSMessage) {
	// Parse the request to validate format
	var req map[string]interface{}
	if err := json.Unmarshal(msg.Request, &req); err != nil {
		wb.sendError(conn, msg.RequestID, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	// Build a response — in production this would call the gateway's actual handler
	response := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-ws-%s", msg.RequestID),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "WebSocket response (bridge active)",
				},
				"finish_reason": "stop",
			},
		},
	}

	data, _ := json.Marshal(response)
	wb.sendMessage(conn, WSResponse{
		Type:      "chat",
		RequestID: msg.RequestID,
		Status:    200,
		Data:      data,
	})
}

// handleModelsRequest processes a list models request over WebSocket.
func (wb *WSBridge) handleModelsRequest(conn *websocket.Conn, msg WSMessage) {
	response := map[string]interface{}{
		"object": "list",
		"data":   []map[string]interface{}{},
	}

	data, _ := json.Marshal(response)
	wb.sendMessage(conn, WSResponse{
		Type:      "models",
		RequestID: msg.RequestID,
		Status:    200,
		Data:      data,
	})
}

// sendMessage sends a WSResponse to a specific connection.
func (wb *WSBridge) sendMessage(conn *websocket.Conn, resp WSResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	conn.WriteMessage(websocket.TextMessage, data)
}

// sendError sends an error response to a specific connection.
func (wb *WSBridge) sendError(conn *websocket.Conn, requestID, errMsg string) {
	wb.sendMessage(conn, WSResponse{
		Type:      "error",
		RequestID: requestID,
		Status:    400,
		Error:     errMsg,
	})
}

// ActiveConnections returns the number of active WebSocket connections.
func (wb *WSBridge) ActiveConnections() int {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	return len(wb.conns)
}

// CloseAll closes all active WebSocket connections.
func (wb *WSBridge) CloseAll() {
	wb.mu.Lock()
	defer wb.mu.Unlock()
	for conn := range wb.conns {
		conn.Close()
		delete(wb.conns, conn)
	}
}
