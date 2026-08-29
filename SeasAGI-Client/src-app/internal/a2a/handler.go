package a2a

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Handler A2A 请求处理器
type Handler struct {
	mu    sync.Mutex
	tasks map[string]*Task
}

// NewHandler 创建 A2A 处理器
func NewHandler() *Handler {
	return &Handler{tasks: make(map[string]*Task)}
}

// HandleJSONRPC 处理 JSON-RPC 请求
func (h *Handler) HandleJSONRPC(req *JSONRPCRequest) *JSONRPCResponse {
	switch req.Method {
	case MethodTaskSend:
		return h.handleTaskSend(req)
	case MethodTaskGet:
		return h.handleTaskGet(req)
	case MethodTaskCancel:
		return h.handleTaskCancel(req)
	default:
		return errorResponse(req.ID, CodeMethodNotFound, "method not found: "+req.Method, nil)
	}
}

// handleTaskSend 处理 tasks/send
func (h *Handler) handleTaskSend(req *JSONRPCRequest) *JSONRPCResponse {
	var params SendTaskParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, "invalid params: "+err.Error(), nil)
	}

	task := &Task{
		ID:     generateTaskID(),
		Status: "completed",
		Input:  params.Message,
		Output: map[string]any{
			"response": "Task processed successfully",
			"time":     time.Now().Format(time.RFC3339),
		},
	}

	h.mu.Lock()
	h.tasks[task.ID] = task
	h.mu.Unlock()

	result, _ := json.Marshal(task)
	return successResponse(req.ID, result)
}

// handleTaskGet 处理 tasks/get
func (h *Handler) handleTaskGet(req *JSONRPCRequest) *JSONRPCResponse {
	var params GetTaskParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, "invalid params: "+err.Error(), nil)
	}

	h.mu.Lock()
	task, ok := h.tasks[params.TaskID]
	h.mu.Unlock()

	if !ok {
		return errorResponse(req.ID, CodeInvalidParams, "task not found: "+params.TaskID, nil)
	}

	result, _ := json.Marshal(task)
	return successResponse(req.ID, result)
}

// handleTaskCancel 处理 tasks/cancel
func (h *Handler) handleTaskCancel(req *JSONRPCRequest) *JSONRPCResponse {
	var params CancelTaskParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, CodeInvalidParams, "invalid params: "+err.Error(), nil)
	}

	h.mu.Lock()
	task, ok := h.tasks[params.TaskID]
	if ok {
		task.Status = "canceled"
	}
	h.mu.Unlock()

	if !ok {
		return errorResponse(req.ID, CodeInvalidParams, "task not found: "+params.TaskID, nil)
	}

	result, _ := json.Marshal(task)
	return successResponse(req.ID, result)
}

// GetAgentMetadata 返回 Agent 元数据
func GetAgentMetadata(endpoint string) AgentMetadata {
	return AgentMetadata{
		Name:         "SeasAGI Agent",
		Description:  "SeasAGI A2A Agent — LLM API gateway with smart routing",
		Version:      "1.0.0",
		Capabilities: []string{"tasks/send", "tasks/get", "tasks/cancel", "streaming"},
		Endpoint:     endpoint,
	}
}

// successResponse 创建成功响应
func successResponse(id any, result json.RawMessage) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// errorResponse 创建错误响应
func errorResponse(id any, code int, message string, data any) *JSONRPCResponse {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message, Data: data},
	}
}

// generateTaskID 生成任务 ID
func generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}
