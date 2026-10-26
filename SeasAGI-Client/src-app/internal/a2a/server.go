// Package a2a 提供 A2A（Agent-to-Agent）协议的服务端实现。
//
// 状态：experimental，未接入生产路径：main/app 未创建或挂载本包服务器，
// 全仓仅本包测试引用。复用时需先补齐鉴权、Host/Origin 校验与请求体上限
// （另行跟踪），否则不要对外暴露监听端口。
package a2a

import (
	"encoding/json"
	"net/http"

	"github.com/SeasAGI/SeasAGI-Client/internal/httpguard"
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

// ServeHTTP 处理 HTTP 请求；仅允许本机 Host/Origin 访问并限制请求体大小。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	httpguard.LocalOnly(http.HandlerFunc(s.serveHTTP)).ServeHTTP(w, r)
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
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
