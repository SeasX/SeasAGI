package a2a

import (
	"encoding/json"
	"net/http"
)

// Server A2A HTTP 服务器
type Server struct {
	handler  *Handler
	metadata AgentMetadata
}

// NewServer 创建 A2A 服务器
func NewServer(endpoint string) *Server {
	return &Server{
		handler:  NewHandler(),
		metadata: GetAgentMetadata(endpoint),
	}
}

// ServeHTTP 处理 HTTP 请求
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/agent.json":
		s.handleMetadata(w, r)
	case "/a2a":
		s.handleA2A(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleMetadata 返回 Agent 元数据
func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.metadata)
}

// handleA2A 处理 JSON-RPC 请求
func (s *Server) handleA2A(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(errorResponse(nil, CodeParseError, "parse error: "+err.Error(), nil))
		return
	}

	resp := s.handler.HandleJSONRPC(&req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetHandler 获取处理器（用于测试）
func (s *Server) GetHandler() *Handler {
	return s.handler
}
