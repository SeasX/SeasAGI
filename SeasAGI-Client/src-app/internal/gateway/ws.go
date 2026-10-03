package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	wsReadTimeout = 120 * time.Second
	wsPingPeriod  = 30 * time.Second
	wsWriteWait   = 10 * time.Second
)

// WSBridge handles WebSocket connections for real-time chat.
// 所有经 WebSocket 进入的请求都会复用网关 HTTP 主链路（鉴权 / 限流 / DLP / 路由 /
// 转发 / 记账），确保 WebSocket 不是治理盲区。
type WSBridge struct {
	upgrader websocket.Upgrader
	svc      *Service
	mu       sync.RWMutex
	conns    map[*websocket.Conn]string // conn -> 已校验的访问令牌
	writeMu  sync.Mutex                 // gorilla 连接不支持并发写，统一串行化
}

// NewWSBridge creates a new WebSocket bridge bound to the gateway service.
func NewWSBridge(svc *Service) *WSBridge {
	return &WSBridge{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin for local proxy usage
				return true
			},
		},
		svc:   svc,
		conns: make(map[*websocket.Conn]string),
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
	token, ok := wb.authorize(r)
	if !ok {
		http.Error(w, "Invalid access token", http.StatusUnauthorized)
		return
	}

	conn, err := wb.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	wb.mu.Lock()
	wb.conns[conn] = token
	wb.mu.Unlock()

	defer func() {
		wb.mu.Lock()
		delete(wb.conns, conn)
		wb.mu.Unlock()
	}()

	// Set read deadline for idle connections
	conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// Start ping ticker to keep the connection alive and detect half-open peers.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				wb.writeMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				err := conn.WriteMessage(websocket.PingMessage, nil)
				wb.writeMu.Unlock()
				if err != nil {
					return
				}
			}
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
			wb.handleChatRequest(conn, msg)
		case "models":
			wb.handleModelsRequest(conn, msg)
		default:
			wb.sendError(conn, msg.RequestID, fmt.Sprintf("unknown message type: %s", msg.Type))
		}
	}
}

// authorize 校验 WebSocket 客户端的访问令牌：优先 Authorization: Bearer，
// 其次 ?token= 查询参数（浏览器 WebSocket API 无法自定义请求头）。
func (wb *WSBridge) authorize(r *http.Request) (string, bool) {
	if wb.svc == nil {
		return "", false
	}
	token := ""
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(auth[7:])
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return token, wb.svc.tokenMatches(token)
}

// handleChatRequest 将 WebSocket 上收到的 chat 请求复用 HTTP 主链路处理，
// 返回真实上游响应（鉴权 / 限流 / DLP / 路由 / 记账全部生效）。
func (wb *WSBridge) handleChatRequest(conn *websocket.Conn, msg WSMessage) {
	token := wb.tokenFor(conn)
	status, body := wb.svc.ServeGatewayRequest("/v1/chat/completions", msg.Request, token)
	wb.sendMessage(conn, WSResponse{
		Type:      "chat",
		RequestID: msg.RequestID,
		Status:    status,
		Data:      body,
	})
}

// handleModelsRequest processes a list models request over WebSocket.
func (wb *WSBridge) handleModelsRequest(conn *websocket.Conn, msg WSMessage) {
	status, body := wb.svc.ServeGatewayRequest("/v1/models", nil, wb.tokenFor(conn))
	wb.sendMessage(conn, WSResponse{
		Type:      "models",
		RequestID: msg.RequestID,
		Status:    status,
		Data:      body,
	})
}

// tokenFor 返回连接建立时校验通过的访问令牌。
func (wb *WSBridge) tokenFor(conn *websocket.Conn) string {
	wb.mu.RLock()
	defer wb.mu.RUnlock()
	return wb.conns[conn]
}

// sendMessage sends a WSResponse to a specific connection.
func (wb *WSBridge) sendMessage(conn *websocket.Conn, resp WSResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	wb.writeMu.Lock()
	defer wb.writeMu.Unlock()
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
