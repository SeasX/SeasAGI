package a2a

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentMetadata(t *testing.T) {
	meta := GetAgentMetadata("http://localhost:4318/a2a")
	if meta.Name != "SeasAGI Agent" {
		t.Errorf("expected name 'SeasAGI Agent', got %s", meta.Name)
	}
	if meta.Endpoint != "http://localhost:4318/a2a" {
		t.Errorf("expected endpoint, got %s", meta.Endpoint)
	}
	if len(meta.Capabilities) == 0 {
		t.Error("expected non-empty capabilities")
	}
}

func TestA2ATaskSend(t *testing.T) {
	h := NewHandler()
	params, _ := json.Marshal(SendTaskParams{
		Message: map[string]any{"content": "hello"},
	})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: MethodTaskSend, Params: params}
	resp := h.HandleJSONRPC(req)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatal("expected result")
	}

	var task Task
	json.Unmarshal(resp.Result, &task)
	if task.ID == "" {
		t.Error("expected non-empty task ID")
	}
	if task.Status != "completed" {
		t.Errorf("expected status completed, got %s", task.Status)
	}
}

func TestA2ATaskGet(t *testing.T) {
	h := NewHandler()

	// 先创建任务
	params, _ := json.Marshal(SendTaskParams{Message: map[string]any{"content": "test"}})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: MethodTaskSend, Params: params}
	resp := h.HandleJSONRPC(req)

	var task Task
	json.Unmarshal(resp.Result, &task)

	// 查询任务
	getParams, _ := json.Marshal(GetTaskParams{TaskID: task.ID})
	getReq := &JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: MethodTaskGet, Params: getParams}
	getResp := h.HandleJSONRPC(getReq)

	if getResp.Error != nil {
		t.Fatalf("unexpected error: %s", getResp.Error.Message)
	}
	var retrieved Task
	json.Unmarshal(getResp.Result, &retrieved)
	if retrieved.ID != task.ID {
		t.Errorf("expected task ID %s, got %s", task.ID, retrieved.ID)
	}
}

func TestA2ATaskCancel(t *testing.T) {
	h := NewHandler()

	// 创建任务
	params, _ := json.Marshal(SendTaskParams{Message: map[string]any{"content": "test"}})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: MethodTaskSend, Params: params}
	resp := h.HandleJSONRPC(req)

	var task Task
	json.Unmarshal(resp.Result, &task)

	// 取消任务
	cancelParams, _ := json.Marshal(CancelTaskParams{TaskID: task.ID})
	cancelReq := &JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: MethodTaskCancel, Params: cancelParams}
	cancelResp := h.HandleJSONRPC(cancelReq)

	if cancelResp.Error != nil {
		t.Fatalf("unexpected error: %s", cancelResp.Error.Message)
	}
	var canceled Task
	json.Unmarshal(cancelResp.Result, &canceled)
	if canceled.Status != "canceled" {
		t.Errorf("expected status canceled, got %s", canceled.Status)
	}
}

func TestA2AErrorHandling(t *testing.T) {
	h := NewHandler()
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "unknown/method"}
	resp := h.HandleJSONRPC(req)

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("expected code %d, got %d", CodeMethodNotFound, resp.Error.Code)
	}
}

func TestA2ATaskGetNotFound(t *testing.T) {
	h := NewHandler()
	params, _ := json.Marshal(GetTaskParams{TaskID: "nonexistent"})
	req := &JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: MethodTaskGet, Params: params}
	resp := h.HandleJSONRPC(req)

	if resp.Error == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestA2AHTTPMetadata(t *testing.T) {
	srv := NewServer("http://localhost:4318/a2a")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var meta AgentMetadata
	json.NewDecoder(rec.Body).Decode(&meta)
	if meta.Name != "SeasAGI Agent" {
		t.Errorf("expected agent name, got %s", meta.Name)
	}
}

func TestA2AHTTPSend(t *testing.T) {
	srv := NewServer("http://localhost:4318/a2a")
	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tasks/send","params":{"message":{"content":"hello"}}}`)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/a2a", body))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", resp.Error.Message)
	}
}

func TestA2AHTTPMethodNotAllowed(t *testing.T) {
	srv := NewServer("http://localhost:4318/a2a")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a2a", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestA2AHTTPNotFound(t *testing.T) {
	srv := NewServer("http://localhost:4318/a2a")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestA2AInvalidParams(t *testing.T) {
	h := NewHandler()
	req := &JSONRPCRequest{
		JSONRPC: "2.0", ID: 1, Method: MethodTaskSend,
		Params: json.RawMessage(`{invalid}`),
	}
	resp := h.HandleJSONRPC(req)
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
	if resp.Error.Code != CodeInvalidParams {
		t.Errorf("expected code %d, got %d", CodeInvalidParams, resp.Error.Code)
	}
}
