package a2a

import "encoding/json"

// JSONRPCRequest JSON-RPC 2.0 请求
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse JSON-RPC 2.0 响应
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError JSON-RPC 2.0 错误
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// AgentMetadata Agent 元数据（/.well-known/agent.json）
type AgentMetadata struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
	Endpoint     string   `json:"endpoint"`
}

// Task A2A 任务
type Task struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"` // pending / working / completed / failed / canceled
	Input     map[string]any         `json:"input,omitempty"`
	Output    map[string]any         `json:"output,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// SendTaskParams tasks/send 参数
type SendTaskParams struct {
	Message map[string]any `json:"message"`
}

// GetTaskParams tasks/get 参数
type GetTaskParams struct {
	TaskID string `json:"task_id"`
}

// CancelTaskParams tasks/cancel 参数
type CancelTaskParams struct {
	TaskID string `json:"task_id"`
}

// 错误码
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// A2A 方法名
const (
	MethodTaskSend   = "tasks/send"
	MethodTaskGet    = "tasks/get"
	MethodTaskCancel = "tasks/cancel"
)
