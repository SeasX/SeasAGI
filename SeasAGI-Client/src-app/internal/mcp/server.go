package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// TransportType MCP 传输类型。
type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
)

// GatewayServer SeasAGI MCP Server — 提供 MCP 协议接口。
type GatewayServer struct {
	mu       sync.RWMutex
	tools    map[string]MCPTool
	auditLog []AuditEntry
}

// AuditEntry 审计日志条目。
type AuditEntry struct {
	ToolName  string      `json:"tool_name"`
	Args      interface{} `json:"args"`
	Success   bool        `json:"success"`
	Error     string      `json:"error,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// NewGatewayServer 创建 MCP Server。
func NewGatewayServer() *GatewayServer {
	server := &GatewayServer{
		tools:    make(map[string]MCPTool),
		auditLog: make([]AuditEntry, 0),
	}
	server.registerBuiltinTools()
	return server
}

// RegisterTool 注册 tool。
func (s *GatewayServer) RegisterTool(tool MCPTool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
}

// GetTool 获取 tool。
func (s *GatewayServer) GetTool(name string) (MCPTool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tool, ok := s.tools[name]
	return tool, ok
}

// ListTools 列出所有 tool。
func (s *GatewayServer) ListTools() []MCPTool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]MCPTool, 0, len(s.tools))
	for _, tool := range s.tools {
		result = append(result, tool)
	}
	return result
}

// CallTool 调用 tool（含审计日志）。
func (s *GatewayServer) CallTool(name string, args map[string]interface{}) (interface{}, error) {
	s.mu.Lock()

	tool, ok := s.tools[name]
	if !ok {
		s.mu.Unlock()
		s.addAudit(name, args, false, fmt.Sprintf("tool not found: %s", name))
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	s.mu.Unlock()

	result, err := tool.Handler(args)

	s.mu.Lock()
	if err != nil {
		s.addAudit(name, args, false, err.Error())
	} else {
		s.addAudit(name, args, true, "")
	}
	s.mu.Unlock()

	return result, err
}

// addAudit 添加审计日志。
func (s *GatewayServer) addAudit(toolName string, args interface{}, success bool, errMsg string) {
	s.auditLog = append(s.auditLog, AuditEntry{
		ToolName:  toolName,
		Args:      args,
		Success:   success,
		Error:     errMsg,
		Timestamp: nowFormatted(),
	})
}

// GetAuditLog 获取审计日志。
func (s *GatewayServer) GetAuditLog() []AuditEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]AuditEntry, len(s.auditLog))
	copy(result, s.auditLog)
	return result
}

// HandleStdio 处理 stdio 传输的 JSON-RPC 请求。
func (s *GatewayServer) HandleStdio(input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(input)
	encoder := json.NewEncoder(output)

	for {
		var request map[string]interface{}
		if err := decoder.Decode(&request); err != nil {
			if err == io.EOF {
				return nil
			}
			continue
		}

		response := s.handleJSONRPC(request)
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
}

// handleJSONRPC 处理 JSON-RPC 请求。
func (s *GatewayServer) handleJSONRPC(request map[string]interface{}) map[string]interface{} {
	method, _ := request["method"].(string)
	id := request["id"]

	switch method {
	case "initialize":
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo": map[string]interface{}{
					"name":    "seasagi-mcp",
					"version": "1.0.0",
				},
			},
		}

	case "tools/list":
		tools := s.ListTools()
		toolList := make([]map[string]interface{}, 0, len(tools))
		for _, tool := range tools {
			toolList = append(toolList, map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"inputSchema": tool.InputSchema,
			})
		}
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"tools": toolList,
			},
		}

	case "tools/call":
		params, _ := request["params"].(map[string]interface{})
		toolName, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]interface{})

		result, err := s.CallTool(toolName, args)
		if err != nil {
			return map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]interface{}{
					"code":    -32603,
					"message": err.Error(),
				},
			}
		}

		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fmt.Sprintf("%v", result),
					},
				},
			},
		}

	default:
		return map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]interface{}{
				"code":    -32601,
				"message": "method not found: " + method,
			},
		}
	}
}

// nowFormatted 返回 RFC3339 格式的当前 UTC 时间戳。
func nowFormatted() string {
	return time.Now().UTC().Format(time.RFC3339)
}
